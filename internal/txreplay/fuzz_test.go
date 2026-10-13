package txreplay

import (
	"encoding/json"
	"net/url"
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
