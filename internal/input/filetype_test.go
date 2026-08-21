package input_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/fhirlint/fhirlint/internal/input"
)

func TestLookupFileType(t *testing.T) {
	cases := []struct {
		path     string
		known    bool
		kind     input.Kind
		parsable bool
		lines    bool
	}{
		{"patient.json", true, input.KindResource, true, false},
		{"patient.xml", true, input.KindResource, true, false},
		{"export.ndjson", true, input.KindLineDelimited, true, true},
		{"export.jsonl", true, input.KindLineDelimited, true, true},
		{"map.fml", true, input.KindValidatorOnly, false, false},
		{"map.map", true, input.KindValidatorOnly, false, false},
		{"notes.txt", false, 0, false, false},
		{"noextension", false, 0, false, false},

		// Case and directories must not change the answer.
		{"EXPORT.JSONL", true, input.KindLineDelimited, true, true},
		{"/tmp/some dir/Patient.Json", true, input.KindResource, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			ft, ok := input.LookupFileType(tc.path)
			if ok != tc.known {
				t.Fatalf("LookupFileType(%q) known = %v, want %v", tc.path, ok, tc.known)
			}
			if ok && ft.Kind != tc.kind {
				t.Errorf("kind = %v, want %v", ft.Kind, tc.kind)
			}
			if got := input.IsFHIRFile(tc.path); got != tc.known {
				t.Errorf("IsFHIRFile = %v, want %v", got, tc.known)
			}
			if got := input.IsParsable(tc.path); got != tc.parsable {
				t.Errorf("IsParsable = %v, want %v", got, tc.parsable)
			}
			if got := input.IsLineDelimited(tc.path); got != tc.lines {
				t.Errorf("IsLineDelimited = %v, want %v", got, tc.lines)
			}
		})
	}
}

// .jsonl and .ndjson are the same format under two names, so nothing may treat
// them differently — that difference was the bug (#340).
func TestFileTypes_JSONLMatchesNDJSON(t *testing.T) {
	jsonl, _ := input.LookupFileType("a.jsonl")
	ndjson, _ := input.LookupFileType("a.ndjson")

	if jsonl.Kind != ndjson.Kind {
		t.Errorf(".jsonl kind = %v, .ndjson kind = %v — they must be read the same way", jsonl.Kind, ndjson.Kind)
	}
	if jsonl.Parser != ndjson.Parser {
		t.Errorf(".jsonl parser = %q, .ndjson parser = %q", jsonl.Parser, ndjson.Parser)
	}
}

func TestFileTypes_WellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, ft := range input.FileTypes {
		if !strings.HasPrefix(ft.Ext, ".") {
			t.Errorf("extension %q must start with a dot", ft.Ext)
		}
		if ft.Ext != strings.ToLower(ft.Ext) {
			t.Errorf("extension %q must be lowercase — lookups normalise to lowercase", ft.Ext)
		}
		if ft.Parser == "" {
			t.Errorf("extension %q has no parser name, so messages about it cannot say what it is", ft.Ext)
		}
		if seen[ft.Ext] {
			t.Errorf("extension %q listed twice", ft.Ext)
		}
		seen[ft.Ext] = true
	}
}

func TestExtensionList(t *testing.T) {
	got := input.Extensions()
	if !slices.IsSorted(got) {
		t.Errorf("Extensions() = %q, want sorted so messages are stable", got)
	}
	for _, want := range []string{".json", ".jsonl", ".ndjson", ".xml", ".fml"} {
		if !slices.Contains(got, want) {
			t.Errorf("Extensions() = %q, missing %q", got, want)
		}
	}
	if !strings.Contains(input.ExtensionList(), ", ") {
		t.Errorf("ExtensionList() = %q, want a comma-separated list", input.ExtensionList())
	}
}
