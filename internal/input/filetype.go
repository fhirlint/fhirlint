package input

import (
	"path/filepath"
	"sort"
	"strings"
)

// Kind says how a file is read, which is a different question from whether the
// validator accepts it.
//
// fhirlint does not only hand paths to the JAR: stats, coverage, referential
// integrity, --extract and the JSON preprocessing all parse the file
// themselves, and they understand JSON and XML only. An extension table with a
// single "is this a FHIR file" bit would let a FHIR Mapping Language file reach
// a JSON parser and be counted as one malformed resource (#341).
type Kind int

const (
	// KindResource is one resource per file, in a format fhirlint can parse.
	KindResource Kind = iota
	// KindLineDelimited is one resource per line, likewise parsable.
	KindLineDelimited
	// KindValidatorOnly is a format the JAR understands and fhirlint does not.
	// It can be validated; it cannot be counted, sliced or rewritten.
	KindValidatorOnly
)

// FileType is one recognised input extension.
type FileType struct {
	Ext  string
	Kind Kind
	// Parser names the format for messages, e.g. "XML input is not supported
	// by coverage".
	Parser string
}

// FileTypes is the single source of truth for the input formats fhirlint
// accepts. The directory walk, the line-splitting decision, stats parsing,
// coverage loading, shell completions, help texts and the "no FHIR files found"
// message are all derived from it.
//
// Extensions are lowercase and include the dot. Adding one here is the whole
// change; adding one anywhere else is how the six copies of this list drifted
// apart before (#340).
var FileTypes = []FileType{
	{Ext: ".json", Kind: KindResource, Parser: "JSON"},
	{Ext: ".xml", Kind: KindResource, Parser: "XML"},

	// Bulk Data exports use .ndjson. .jsonl is the same format under the name
	// the surrounding data tooling produces — DuckDB, Spark, jq, pandas — which
	// is what a dataset assembled outside a FHIR server tends to be called.
	{Ext: ".ndjson", Kind: KindLineDelimited, Parser: "NDJSON"},
	{Ext: ".jsonl", Kind: KindLineDelimited, Parser: "NDJSON"},

	// FHIR Mapping Language. The JAR parses it, builds the StructureMap and
	// validates that, reporting positions back in the .fml source. fhirlint
	// cannot read it, so everything that parses files skips it and says so.
	{Ext: ".fml", Kind: KindValidatorOnly, Parser: "FHIR Mapping Language"},
	{Ext: ".map", Kind: KindValidatorOnly, Parser: "FHIR Mapping Language"},
}

// LookupFileType finds the entry for a path's extension.
func LookupFileType(path string) (FileType, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	for _, ft := range FileTypes {
		if ft.Ext == ext {
			return ft, true
		}
	}
	return FileType{}, false
}

// IsFHIRFile reports whether the path is an input fhirlint hands to the
// validator at all. This is what a directory walk filters on.
func IsFHIRFile(path string) bool {
	_, ok := LookupFileType(path)
	return ok
}

// IsLineDelimited reports whether the file holds one resource per line and must
// be split before validating.
func IsLineDelimited(path string) bool {
	ft, ok := LookupFileType(path)
	return ok && ft.Kind == KindLineDelimited
}

// IsParsable reports whether fhirlint can read the file itself. Callers that do
// more than pass the path along — stats, coverage, refcheck, extract,
// preprocessing — gate on this, and report what they skipped rather than
// dropping it in silence.
func IsParsable(path string) bool {
	ft, ok := LookupFileType(path)
	return ok && ft.Kind != KindValidatorOnly
}

// Extensions lists every recognised extension, sorted, for help texts and error
// messages: ".json, .jsonl, .map, .ndjson, .xml".
func Extensions() []string {
	out := make([]string, 0, len(FileTypes))
	for _, ft := range FileTypes {
		out = append(out, ft.Ext)
	}
	sort.Strings(out)
	return out
}

// ExtensionList renders Extensions() for a message.
func ExtensionList() string {
	return strings.Join(Extensions(), ", ")
}
