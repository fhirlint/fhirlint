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
	store    *Store
	upstream string
	client   *http.Client

	mu     sync.Mutex
	misses []Miss

	listener net.Listener
	srv      *http.Server
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
	return "http://" + ln.Addr().String(), nil
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

	if s.upstream == "" {
		s.replay(w, r, body)
		return
	}
	s.record(w, r, body)
}

func (s *Server) replay(w http.ResponseWriter, r *http.Request, body []byte) {
	in, ok := s.store.Lookup(r.Method, r.URL.Path, r.URL.RawQuery, body)
	if !ok {
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
		return
	}
	writeInteraction(w, in)
}

func (s *Server) record(w http.ResponseWriter, r *http.Request, body []byte) {
	target := s.upstream + r.URL.Path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	// The destination is the terminology server the user chose for recording,
	// not anything the incoming request controls: only the path and query come
	// from the validator, and they are appended to that fixed base.
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, strings.NewReader(string(body))) //nolint:gosec // G704: target base is operator-supplied, not request-controlled
	if err != nil {
		writeOutcome(w, http.StatusBadGateway, "building upstream request: "+err.Error())
		return
	}
	// Carry the negotiation headers through: the terminology server keys its
	// response format and FHIR version off them.
	for _, h := range []string{"Accept", "Content-Type", "Accept-Charset", "Accept-Language"} {
		if v := r.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}

	resp, err := s.client.Do(req) //nolint:gosec // G704: see the request construction above
	if err != nil {
		writeOutcome(w, http.StatusBadGateway, "terminology server unreachable: "+err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		writeOutcome(w, http.StatusBadGateway, "reading terminology response: "+err.Error())
		return
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

	// Pass infrastructure failures through without storing them. A 404 from a
	// misconfigured endpoint would otherwise be replayed forever as if it were
	// the terminology server's real answer.
	if !recordable(resp.StatusCode) {
		writeInteraction(w, in)
		return
	}

	s.mu.Lock()
	err = s.store.Put(in, body)
	s.mu.Unlock()
	if err != nil {
		writeOutcome(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeInteraction(w, in)
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
		_, _ = w.Write(in.Response)
		return
	}
	_, _ = io.WriteString(w, in.ResponseText)
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
