package validator

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/fhirlint/fhirlint/internal/cache"
)

// Validator 7.0.x emits many terminology issues without the
// operationoutcome-message-id extension (hapifhir/org.hl7.fhir.core#2708),
// while 6.10.x gave the same findings an id. Baselines and messageId
// suppressions key on that id, so an unpinned upgrade silently stops matching
// them (#449).
//
// The text is still the validator's own: it is formatted from a template in
// the Messages.properties bundled with the JAR. Matching the text back against
// those templates recovers the id. The templates are read from the JAR that
// ran, never from a copy, because they change between releases.

// messagesEntry is the bundled English message bundle, at the JAR's root.
const messagesEntry = "Messages.properties"

// minTemplateLiteral is the least fixed text a template must have to be used
// for inference. A template that is mostly placeholders ("{0}", "{0}: {1}")
// matches nearly anything and would attach an arbitrary id.
const minTemplateLiteral = 12

// pluralSuffix strips the CLDR plural form from a key. The validator reports a
// plural message under its base constant (BaseValidator.rulePlural and
// friends), so FOO_one and FOO_other both stand for FOO.
var pluralSuffix = regexp.MustCompile(`_(zero|one|two|few|many|other)$`)

type messageTemplate struct {
	id     string
	prefix string // fixed text before the first placeholder, for a cheap pre-check
	suffix string // fixed text after the last placeholder
	re     *regexp.Regexp
}

// messageCatalog maps validator message text back to its message id.
type messageCatalog struct {
	templates []messageTemplate

	mu   sync.Mutex
	memo map[string]string // text → id ("" when no unique match)
}

// infer returns the id of the template that text was formatted from, or ""
// when no template matches or templates with different ids do.
func (c *messageCatalog) infer(text string) string {
	if c == nil {
		return ""
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	c.mu.Lock()
	id, seen := c.memo[text]
	c.mu.Unlock()
	if seen {
		return id
	}

	// Exactly one id, or none. Templates can overlap: the validator's
	// INACTIVE_CONCEPT_FOUND text "has a status of retired and inactive and its
	// use should be reviewed" also fits INACTIVE_CONCEPT_FOUND_ADD ("has a
	// status of {0} and {2} and its use…"), and preferring the more specific
	// template picks the wrong one. Every id-less issue in the 78 kbv.basis
	// examples under 7.0.1 matches a single template, so the strict rule costs
	// nothing there.
	best := ""
	for i := range c.templates {
		t := &c.templates[i]
		if t.id == best || !strings.HasPrefix(text, t.prefix) || !strings.HasSuffix(text, t.suffix) || !t.re.MatchString(text) {
			continue
		}
		if best != "" {
			best = ""
			break
		}
		best = t.id
	}

	c.mu.Lock()
	c.memo[text] = best
	c.mu.Unlock()
	return best
}

// parseMessageCatalog builds a catalog from a Java .properties message bundle.
func parseMessageCatalog(r io.Reader) (*messageCatalog, error) {
	props, err := readProperties(r)
	if err != nil {
		return nil, err
	}
	c := &messageCatalog{memo: map[string]string{}}
	for key, value := range props {
		t, ok := compileTemplate(pluralSuffix.ReplaceAllString(key, ""), value)
		if ok {
			c.templates = append(c.templates, t)
		}
	}
	return c, nil
}

// compileTemplate turns a java.text.MessageFormat pattern into an anchored
// regular expression. MessageFormat quoting: a doubled single quote is a
// literal quote, a lone one starts or ends a quoted run in which braces are
// literal, and {…} is an argument, whatever its format type.
func compileTemplate(id, pattern string) (messageTemplate, bool) {
	pattern = strings.TrimSpace(pattern)
	var (
		re       strings.Builder
		literals []string // fixed text between placeholders, in order
		cur      strings.Builder
		quoted   bool
	)
	flush := func() {
		literals = append(literals, cur.String())
		re.WriteString(regexp.QuoteMeta(cur.String()))
		cur.Reset()
	}
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch {
		case ch == '\'' && i+1 < len(pattern) && pattern[i+1] == '\'':
			cur.WriteByte('\'')
			i++
		case ch == '\'':
			quoted = !quoted
		case ch == '{' && !quoted:
			depth := 1
			j := i + 1
			for ; j < len(pattern) && depth > 0; j++ {
				switch pattern[j] {
				case '{':
					depth++
				case '}':
					depth--
				}
			}
			if depth != 0 {
				return messageTemplate{}, false // unbalanced: not a usable pattern
			}
			flush()
			re.WriteString("(.*?)")
			i = j - 1
		default:
			cur.WriteByte(ch)
		}
	}
	flush()

	literal := 0
	for _, l := range literals {
		literal += len(l)
	}
	if literal < minTemplateLiteral {
		return messageTemplate{}, false
	}
	compiled, err := regexp.Compile("(?s)^" + re.String() + "$")
	if err != nil {
		return messageTemplate{}, false
	}
	return messageTemplate{
		id:     id,
		prefix: literals[0],
		suffix: literals[len(literals)-1],
		re:     compiled,
	}, true
}

// readProperties parses the subset of the .properties format the validator's
// bundles use: key = value lines, # and ! comments, backslash line
// continuations and backslash escapes including \uXXXX.
func readProperties(r io.Reader) (map[string]string, error) {
	props := map[string]string{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var logical strings.Builder
	for sc.Scan() {
		line := sc.Text()
		if logical.Len() == 0 {
			trimmed := strings.TrimLeft(line, " \t\f")
			if trimmed == "" || trimmed[0] == '#' || trimmed[0] == '!' {
				continue
			}
			line = trimmed
		} else {
			line = strings.TrimLeft(line, " \t\f")
		}
		if continues(line) {
			logical.WriteString(line[:len(line)-1])
			continue
		}
		logical.WriteString(line)
		key, value := splitProperty(logical.String())
		logical.Reset()
		if key != "" {
			props[unescapeProperty(key)] = unescapeProperty(value)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return props, nil
}

// continues reports whether a line ends in an odd number of backslashes.
func continues(line string) bool {
	n := 0
	for i := len(line) - 1; i >= 0 && line[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

// splitProperty splits a logical line the way java.util.Properties does: the
// key ends at the first unescaped '=', ':' or whitespace, and the separator is
// optional whitespace with at most one '=' or ':' in it.
func splitProperty(line string) (string, string) {
	end := len(line)
	for i := 0; i < len(line); i++ {
		if line[i] == '\\' {
			i++
			continue
		}
		if strings.IndexByte("=: \t\f", line[i]) >= 0 {
			end = i
			break
		}
	}
	rest := strings.TrimLeft(line[end:], " \t\f")
	if rest != "" && (rest[0] == '=' || rest[0] == ':') {
		rest = strings.TrimLeft(rest[1:], " \t\f")
	}
	return line[:end], rest
}

func unescapeProperty(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 't':
			b.WriteByte('\t')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 'f':
			b.WriteByte('\f')
		case 'u':
			if i+4 < len(s) {
				if v, err := strconv.ParseUint(s[i+1:i+5], 16, 16); err == nil {
					b.WriteRune(rune(v))
					i += 4
					continue
				}
			}
			b.WriteByte('u')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// loadMessageCatalog reads the message bundle out of a validator JAR.
func loadMessageCatalog(jarPath string) (*messageCatalog, error) {
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }()
	for _, f := range zr.File {
		if f.Name != messagesEntry {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer func() { _ = rc.Close() }()
		return parseMessageCatalog(rc)
	}
	return nil, fmt.Errorf("%s has no %s", jarPath, messagesEntry)
}

type catalogKey struct {
	path  string
	size  int64
	mtime int64
}

var catalogs sync.Map // catalogKey → *messageCatalog (nil when the JAR has none)

// messageCatalogFor returns the catalog of the JAR at jarPath, loading it once
// per JAR file. It returns nil, and inference is skipped, when the JAR cannot
// be read: a missing id is the validator's normal behaviour for some issues and
// not worth failing a run over.
func messageCatalogFor(jarPath string) *messageCatalog {
	if jarPath == "" {
		p, err := cache.JARPath()
		if err != nil {
			return nil
		}
		jarPath = p
	}
	st, err := os.Stat(jarPath)
	if err != nil {
		return nil
	}
	key := catalogKey{jarPath, st.Size(), st.ModTime().UnixNano()}
	if c, ok := catalogs.Load(key); ok {
		return c.(*messageCatalog)
	}
	c, err := loadMessageCatalog(jarPath)
	if err != nil {
		c = nil
	}
	catalogs.Store(key, c)
	return c
}
