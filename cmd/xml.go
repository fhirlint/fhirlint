package cmd

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// xmlPathSegments converts a JSONPath-style path to a slice of XML element
// name segments, stripping array indices.
// Example: "$.entry[0].resource.Patient" → ["entry", "resource", "Patient"]
func xmlPathSegments(path string) []string {
	p := strings.TrimPrefix(path, "$.")
	p = strings.TrimPrefix(p, "$")
	var buf strings.Builder
	inBracket := false
	for _, r := range p {
		switch {
		case r == '[':
			inBracket = true
		case r == ']':
			inBracket = false
		case !inBracket:
			buf.WriteRune(r)
		}
	}
	p = strings.Trim(buf.String(), ".")
	parts := strings.Split(p, ".")
	out := make([]string, 0, len(parts))
	for _, s := range parts {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// xmlPathMatches reports whether stack[1:] (the path relative to the document
// root element) equals path. stack[0] is the root element name (e.g. "Patient"),
// which is not part of user-supplied paths.
func xmlPathMatches(stack []string, path []string) bool {
	if len(stack) == 0 || len(path) == 0 {
		return false
	}
	rel := stack[1:]
	if len(rel) != len(path) {
		return false
	}
	for i, s := range rel {
		if s != path[i] {
			return false
		}
	}
	return true
}

// xmlDeletePaths removes all elements matching any of paths from the XML document.
// Paths are relative to the document root element: ["meta", "tag"] removes every
// <tag> that is a direct child of <meta> (which is a direct child of the root).
// Unmatched paths are silently ignored.
func xmlDeletePaths(data []byte, paths [][]string) ([]byte, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)

	var stack []string
	skipDepth := 0

	for {
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
			if skipDepth > 0 {
				skipDepth++
				continue
			}
			for _, path := range paths {
				if xmlPathMatches(stack, path) {
					skipDepth = 1
					break
				}
			}
			if skipDepth == 0 {
				if err := enc.EncodeToken(t); err != nil {
					return nil, err
				}
			}
		case xml.EndElement:
			if skipDepth > 0 {
				skipDepth--
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
				continue
			}
			if err := enc.EncodeToken(t); err != nil {
				return nil, err
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		default:
			if skipDepth == 0 {
				if err := enc.EncodeToken(tok); err != nil {
					return nil, err
				}
			}
		}
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// xmlExtract extracts the first element at path from data and returns it as a
// standalone XML document. If the document root declares a default namespace
// and the extracted element does not, the namespace is injected so the result
// can be validated as a standalone FHIR resource.
//
// Paths follow JSONPath-style syntax relative to the document root:
//
//	"$.entry.resource.Patient"  →  segments ["entry", "resource", "Patient"]
//
// Array indices in the path are stripped — the first matching element is returned.
func xmlExtract(data []byte, path string) ([]byte, error) {
	segments := xmlPathSegments(path)
	if len(segments) == 0 {
		return nil, fmt.Errorf("--extract: empty path %q", path)
	}

	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false

	var stack []string
	var rootNS string
	collecting := false
	collectDepth := 0
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)

	for {
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if len(stack) == 0 {
				for _, a := range t.Attr {
					if a.Name.Local == "xmlns" && a.Name.Space == "" {
						rootNS = a.Value
					}
				}
			}
			stack = append(stack, t.Name.Local)

			if collecting {
				collectDepth++
				if err := enc.EncodeToken(t); err != nil {
					return nil, err
				}
			} else if xmlPathMatches(stack, segments) {
				collecting = true
				collectDepth = 1
				if rootNS != "" && !xmlHasDefaultNS(t.Attr) {
					t.Attr = append([]xml.Attr{{Name: xml.Name{Local: "xmlns"}, Value: rootNS}}, t.Attr...)
				}
				if err := enc.EncodeToken(t); err != nil {
					return nil, err
				}
			}

		case xml.EndElement:
			if collecting {
				if err := enc.EncodeToken(t); err != nil {
					return nil, err
				}
				collectDepth--
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
				if collectDepth == 0 {
					if err := enc.Flush(); err != nil {
						return nil, err
					}
					return buf.Bytes(), nil
				}
			} else {
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
			}

		default:
			if collecting {
				if err := enc.EncodeToken(tok); err != nil {
					return nil, err
				}
			}
		}
	}

	return nil, fmt.Errorf("--extract: path %q not found in XML input", path)
}

// xmlHasDefaultNS reports whether the attribute list contains a default
// namespace declaration (xmlns="...").
func xmlHasDefaultNS(attrs []xml.Attr) bool {
	for _, a := range attrs {
		if a.Name.Local == "xmlns" && a.Name.Space == "" {
			return true
		}
	}
	return false
}
