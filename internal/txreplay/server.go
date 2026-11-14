package txreplay

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// upstreamTimeout bounds a single proxied terminology call while recording.
const upstreamTimeout = 60 * time.Second

// maxResponseBytes bounds one upstream response while recording. The
// terminology server is untrusted (SECURITY.md) and the body is held in memory
// and then written into the recording, so it is cut off here (#588). A large
// ValueSet expansion stays far below it. A variable so tests can lower it.
var maxResponseBytes int64 = 128 << 20

// Miss is a request the replay server could not answer.
type Miss struct {
	Method string
	Path   string
	Query  string
	// Detail summarises what was asked about (system, code, value set URL) so
	// the user can tell which resource needs re-recording.
	Detail string
}

func (m Miss) String() string {
	s := m.Method + " " + m.Path
	if m.Query != "" {
		s += "?" + m.Query
	}
	if m.Detail != "" {
		s += " (" + m.Detail + ")"
	}
	return s
}

// Server stands in for a terminology server. With an upstream it records what
// it proxies; without one it replays from the store and records what it could
// not answer.
type Server struct {
	store           *Store
	upstream        string
	upstreamHeaders map[string]string
	client          *http.Client

	// Registry routing (#465): the tx registry the recorder forwards to, and
	// the hosts its answers named. ecosystem is empty without routing.
	ecosystem string
	routed    map[string]bool
	contacted map[string]bool
	// base is this server's own URL, for rewriting registry answers.
	base string

	mu     sync.Mutex
	misses []Miss

	listener net.Listener
	srv      *http.Server
}

// WithUpstreamHeaders sets headers added to every proxied request — the
// credential for an authenticated terminology server, in practice.
//
// Recording is the one path where fhirlint makes the terminology call itself,
// so the JAR's settings file cannot help here. The headers are not stored: the
// point of a recording is that replaying it needs no credential at all.
func (s *Server) WithUpstreamHeaders(h map[string]string) *Server {
	s.upstreamHeaders = h
	return s
}

// NewRecorder proxies to upstream and stores every interaction.
func NewRecorder(store *Store, upstream string, client *http.Client) *Server {
	if client == nil {
		client = &http.Client{Timeout: upstreamTimeout}
	}
	return &Server{store: store, upstream: strings.TrimSuffix(upstream, "/"), client: client}
}

// NewPlayer answers only from the store. Anything not recorded is a miss.
func NewPlayer(store *Store) *Server {
	return &Server{store: store}
}

// Start binds a loopback port and serves until Stop. It returns the base URL to
// hand to the validator's -tx option.
func (s *Server) Start() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("starting local terminology server: %w", err)
	}
	s.listener = ln
	s.srv = &http.Server{
		Handler:           http.HandlerFunc(s.handle),
		ReadHeaderTimeout: 30 * time.Second,
	}
	go func() { _ = s.srv.Serve(ln) }()
	s.base = "http://" + ln.Addr().String()
	return s.base, nil
}

// Stop shuts the server down. It is safe to call on a server that never started.
func (s *Server) Stop() error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Close()
}

// Misses returns the requests replay could not answer, in arrival order.
func (s *Server) Misses() []Miss {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Miss(nil), s.misses...)
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "cannot read request body", http.StatusBadRequest)
		return
	}
	defer func() { _ = r.Body.Close() }()

	if s.ecosystem != "" {
		switch {
		case isRegistryPath(r.URL.Path):
			s.serveRegistry(w, r, body)
			return
		case strings.HasPrefix(r.URL.Path, routePrefix):
			s.serveRoute(w, r, body)
			return
		}
	}
	if s.upstream == "" {
		s.replay(w, r, body)
		return
	}
	s.record(w, r, body)
}

func (s *Server) replay(w http.ResponseWriter, r *http.Request, body []byte) {
	in, ok := s.store.Lookup(r.Method, r.URL.Path, r.URL.RawQuery, body)
	if !ok {
		s.miss(w, r, body)
		return
	}
	writeInteraction(w, in)
}

// miss records a request replay could not answer and says so to the JAR.
func (s *Server) miss(w http.ResponseWriter, r *http.Request, body []byte) {
	s.mu.Lock()
	s.misses = append(s.misses, Miss{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.RawQuery,
		Detail: describe(body),
	})
	s.mu.Unlock()
	// The JAR treats some terminology failures as warnings rather than
	// aborting, so the run must not be trusted to fail on its own — the
	// caller checks Misses() afterwards. This response only keeps the
	// validator from hanging.
	writeOutcome(w, http.StatusNotFound, "not recorded: run 'fhirlint tx warm' to record this terminology request")
}

func (s *Server) record(w http.ResponseWriter, r *http.Request, body []byte) {
	target := s.upstream + r.URL.Path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	in, ok := s.forward(w, r, body, target, nil)
	if !ok {
		return
	}
	if !s.keep(w, in, body) {
		return
	}
	writeInteraction(w, in)
}

// forward sends the validator's request to target and returns the answer as
// an Interaction. headers nil means the upstream credentials; routed servers
// pass an empty map so that they never receive them. On failure it has
// already answered the validator and reports false.
func (s *Server) forward(w http.ResponseWriter, r *http.Request, body []byte, target string, headers map[string]string) (*Interaction, bool) {
	// The destination is the terminology server the user chose for recording,
	// the registry, or a server a registry answer named in this run — never
	// anything the incoming request controls alone: only the path and query
	// come from the validator.
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, strings.NewReader(string(body))) //nolint:gosec // G704: target base is operator-supplied or allowlisted, not request-controlled
	if err != nil {
		writeOutcome(w, http.StatusBadGateway, "building upstream request: "+err.Error())
		return nil, false
	}
	// Carry the negotiation headers through: the terminology server keys its
	// response format and FHIR version off them.
	for _, h := range []string{"Accept", "Content-Type", "Accept-Charset", "Accept-Language"} {
		if v := r.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	if headers == nil {
		headers = s.upstreamHeaders
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	resp, err := s.client.Do(req) //nolint:gosec // G704: see the request construction above
	if err != nil {
		writeOutcome(w, http.StatusBadGateway, "terminology server unreachable: "+err.Error())
		return nil, false
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		writeOutcome(w, http.StatusBadGateway, "reading terminology response: "+err.Error())
		return nil, false
	}
	if int64(len(respBody)) > maxResponseBytes {
		writeOutcome(w, http.StatusBadGateway, fmt.Sprintf("terminology response over the limit of %d bytes; not recorded", maxResponseBytes))
		return nil, false
	}

	in := &Interaction{
		Method:      r.Method,
		Path:        r.URL.Path,
		Query:       r.URL.RawQuery,
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
	}
	if json.Valid(body) {
		in.Request = canonicalJSON(body)
	}
	if json.Valid(respBody) {
		in.Response = respBody
	} else {
		in.ResponseText = string(respBody)
	}
	return in, true
}

// keep stores an answer worth replaying. An infrastructure failure is passed
// to the validator unstored: a 404 from a misconfigured endpoint would
// otherwise be replayed forever as if it were the server's real answer. It
// reports false when it has already answered the validator.
func (s *Server) keep(w http.ResponseWriter, in *Interaction, body []byte) bool {
	if !recordable(in.Status) {
		writeInteraction(w, in)
		return false
	}
	s.mu.Lock()
	err := s.store.Put(in, body)
	s.mu.Unlock()
	if err != nil {
		writeOutcome(w, http.StatusInternalServerError, err.Error())
		return false
	}
	return true
}

// recordable reports whether a response is an answer worth keeping. Terminology
// servers answer "this code is not valid" with 200 and a Parameters body, or
// with 422; anything else is a transport or configuration problem.
func recordable(status int) bool {
	return (status >= 200 && status < 300) || status == http.StatusUnprocessableEntity
}

func writeInteraction(w http.ResponseWriter, in *Interaction) {
	ct := in.ContentType
	if ct == "" {
		ct = "application/fhir+json"
	}
	w.Header().Set("Content-Type", ct)
	status := in.Status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	if len(in.Response) > 0 {
		_, _ = w.Write(in.Response) //nolint:gosec // G705: a terminology answer to the validator JAR, not a page a browser renders
		return
	}
	_, _ = io.WriteString(w, in.ResponseText) //nolint:gosec // G705: as above, the client is the validator JAR
}

func writeOutcome(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/fhir+json")
	w.WriteHeader(status)
	outcome := map[string]interface{}{
		"resourceType": "OperationOutcome",
		"issue": []map[string]interface{}{{
			"severity":    "error",
			"code":        "not-found",
			"diagnostics": msg,
		}},
	}
	_ = json.NewEncoder(w).Encode(outcome)
}

// describe pulls the identifying values out of a terminology Parameters body so
// a miss can name what was asked about instead of just the endpoint.
func describe(body []byte) string {
	if !json.Valid(body) {
		return ""
	}
	type coding struct {
		System  string `json:"system"`
		Code    string `json:"code"`
		Version string `json:"version"`
	}
	var params struct {
		Parameter []struct {
			Name                 string  `json:"name"`
			ValueURI             string  `json:"valueUri"`
			ValueCode            string  `json:"valueCode"`
			ValueString          string  `json:"valueString"`
			ValueCoding          *coding `json:"valueCoding"`
			ValueCodeableConcept *struct {
				Coding []coding `json:"coding"`
			} `json:"valueCodeableConcept"`
		} `json:"parameter"`
	}
	if err := json.Unmarshal(body, &params); err != nil {
		return ""
	}

	var parts []string
	add := func(label, v string) {
		if v != "" {
			parts = append(parts, label+" "+v)
		}
	}
	addCoding := func(c coding) {
		add("system", c.System)
		add("code", c.Code)
	}
	for _, p := range params.Parameter {
		switch {
		// The validator asks about a whole CodeableConcept far more often than
		// about a bare system/code pair, so this is the case that decides
		// whether a miss message is useful at all.
		case p.ValueCodeableConcept != nil:
			for _, c := range p.ValueCodeableConcept.Coding {
				addCoding(c)
			}
		case p.ValueCoding != nil:
			addCoding(*p.ValueCoding)
		case p.Name == "url" || p.Name == "system" || p.Name == "code" || p.Name == "version":
			v := p.ValueURI
			if v == "" {
				v = p.ValueCode
			}
			if v == "" {
				v = p.ValueString
			}
			add(p.Name, v)
		}
	}
	return strings.Join(parts, ", ")
}
