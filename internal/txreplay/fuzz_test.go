package txreplay

import (
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"testing"
)

// FuzzKey keys a fuzzed request. Recordings are committed to the repository and
// can arrive through a pull request, and every replayed request is looked up by
// this key. Beyond not panicking, layout must not matter: indenting a JSON body
// and reversing the order of the query parameters give the same key.
func FuzzKey(f *testing.F) {
	f.Add("POST", "/CodeSystem/$validate-code", "", []byte(`{"resourceType":"Parameters","parameter":[{"name":"code","valueCode":"x"}]}`))
	f.Add("GET", "/ValueSet", "url=http://a%7C1&_format=json&url=http://b", []byte(nil))
	f.Add("get", "/metadata", "%zz&a=1;b=2", []byte("not json"))
	f.Add("POST", "/", "a=1&a=1", []byte(`{"n":12345678901234567890,"f":1e400}`))

	f.Fuzz(func(t *testing.T, method, path, rawQuery string, body []byte) {
		k := Key(method, path, rawQuery, body)
		if len(k) != 32 {
			t.Fatalf("key %q is not 32 hex characters", k)
		}
		if again := canonicalJSON(canonicalJSON(body)); string(again) != string(canonicalJSON(body)) {
			t.Errorf("canonicalJSON is not idempotent: %q then %q", canonicalJSON(body), again)
		}

		var v interface{}
		if json.Valid(body) && json.Unmarshal(body, &v) == nil {
			if indented, err := json.MarshalIndent(v, "", "  "); err == nil {
				if Key(method, path, rawQuery, indented) != Key(method, path, rawQuery, canonicalJSON(body)) {
					t.Errorf("indenting the body changed the key")
				}
			}
		}

		parts := strings.Split(rawQuery, "&")
		for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
			parts[i], parts[j] = parts[j], parts[i]
		}
		reversed := strings.Join(parts, "&")
		_, err1 := url.ParseQuery(rawQuery)
		_, err2 := url.ParseQuery(reversed)
		if err1 == nil && err2 == nil && canonicalQuery(rawQuery) != canonicalQuery(reversed) {
			t.Errorf("parameter order changed the key: %q vs %q", rawQuery, reversed)
		}
	})
}

// FuzzCanonicalQuery checks that canonicalQuery only merges queries that mean
// the same thing: two queries share a canonical form only if both parse to the
// same parameters, or neither parses and they are identical (#468).
func FuzzCanonicalQuery(f *testing.F) {
	f.Add("a=x%26b%3Dy", "a=x&b=y")
	f.Add("a=x%3Bb", "a=x;b")
	f.Add("b=2&a=1", "a=1&b=2")
	f.Add("url=http://a%7C1", "url=http://a|1")
	f.Add("a", "a=")

	f.Fuzz(func(t *testing.T, q1, q2 string) {
		if canonicalQuery(q1) != canonicalQuery(q2) {
			return
		}
		v1, err1 := url.ParseQuery(q1)
		v2, err2 := url.ParseQuery(q2)
		switch {
		case err1 != nil || err2 != nil:
			if q1 != q2 {
				t.Errorf("%q and %q share the canonical form %q", q1, q2, canonicalQuery(q1))
			}
		case !sameParams(v1, v2):
			t.Errorf("%q and %q parse differently but share the canonical form %q", q1, q2, canonicalQuery(q1))
		}
	})
}

// sameParams compares parsed queries regardless of the order of repeated values.
func sameParams(a, b url.Values) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok || len(va) != len(vb) {
			return false
		}
		sa, sb := slices.Clone(va), slices.Clone(vb)
		slices.Sort(sa)
		slices.Sort(sb)
		if !slices.Equal(sa, sb) {
			return false
		}
	}
	return true
}
