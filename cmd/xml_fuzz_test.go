package cmd

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"testing"
)

// FuzzXMLPreprocess runs a fuzzed XML document through --extract and then
// --ignore with a fuzzed path, in the order validate applies them. The
// document comes from a repository the user did not write, and both walk it
// with their own element stack over a non-strict decoder. Malformed input is
// an error, never a panic, and what --extract returns is a single element
// that parses on its own, since it goes to the validator as a resource.
func FuzzXMLPreprocess(f *testing.F) {
	f.Add([]byte(`<Bundle xmlns="http://hl7.org/fhir"><entry><resource><Patient><id value="1"/></Patient></resource></entry></Bundle>`), "$.entry[0].resource.Patient")
	f.Add([]byte(`<Bundle xmlns="http://hl7.org/fhir"><entry><resource><Patient xmlns="http://hl7.org/fhir"><meta><tag/></meta></Patient></resource></entry></Bundle>`), "entry.resource.Patient")
	f.Add([]byte(`<?xml version="1.0"?><!-- c --><Bundle><entry><resource><Patient><text><div xmlns="http://www.w3.org/1999/xhtml"><p>&amp;</p></div></text></Patient></resource></entry></Bundle>`), "entry.resource")
	f.Add([]byte(`<a><b><c></b></c></a>`), "b.c")
	f.Add([]byte(`<a x:y="1"><![CDATA[<b>]]></a>`), "$")
	f.Add([]byte(`</a>`), "a")

	f.Fuzz(func(t *testing.T, data []byte, path string) {
		doc := data
		if segs := xmlPathSegments(path); len(segs) > 0 {
			extracted, err := xmlExtract(data, path)
			if err == nil {
				if err := wellFormed(extracted); err != nil {
					t.Errorf("--extract %q returned XML that does not parse: %v\n%s", path, err, extracted)
				}
				doc = extracted
			}
		}
		_, _ = xmlDeletePaths(doc, [][]string{xmlPathSegments(path), {"meta"}})
	})
}

// wellFormed reports whether data is exactly one element, balanced.
func wellFormed(data []byte) error {
	dec := xml.NewDecoder(bytes.NewReader(data))
	depth, roots := 0, 0
	for {
		tok, err := dec.RawToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		switch tok.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
	if depth != 0 || roots != 1 {
		return errors.New("not exactly one balanced element")
	}
	return nil
}
