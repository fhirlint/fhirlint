package stats

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"

	"github.com/tidwall/gjson"

	"github.com/fhirlint/fhirlint/internal/input"
)

// ParseFile extracts the FHIR resources from a file. A .json or .xml file
// yields one resource (its root); a .ndjson file yields one resource per
// non-empty line. Unparseable input yields a resource with an empty Type,
// which Compute buckets under "(unknown)".
func ParseFile(path string) ([]Resource, error) {
	data, err := os.ReadFile(path) //nolint:gosec // user-supplied dataset path
	if err != nil {
		return nil, err
	}

	if input.IsLineDelimited(path) {
		return parseNDJSON(data), nil
	}
	if strings.EqualFold(filepath.Ext(path), ".xml") {
		return []Resource{parseXML(data)}, nil
	}
	// A format fhirlint cannot read is not a resource it can count. Saying so
	// beats the old default branch, which handed a FHIR Mapping Language file
	// to the JSON parser and counted the failure as one resource.
	if !input.IsParsable(path) {
		return nil, nil
	}
	return []Resource{parseJSON(data)}, nil
}

func parseJSON(data []byte) Resource {
	return Resource{
		Type:     gjson.GetBytes(data, "resourceType").String(),
		Profiles: profilesFromJSON(data),
	}
}

func parseNDJSON(data []byte) []Resource {
	var out []Resource
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		out = append(out, parseJSON([]byte(line)))
	}
	return out
}

func profilesFromJSON(data []byte) []string {
	res := gjson.GetBytes(data, "meta.profile")
	if !res.Exists() {
		return nil
	}
	var profiles []string
	res.ForEach(func(_, value gjson.Result) bool {
		if p := strings.TrimSpace(value.String()); p != "" {
			profiles = append(profiles, p)
		}
		return true
	})
	return profiles
}

// xmlResource extracts the root element name (the resourceType) and any
// meta.profile values from a FHIR XML resource.
type xmlResource struct {
	XMLName xml.Name
	Meta    struct {
		Profile []struct {
			Value string `xml:"value,attr"`
		} `xml:"profile"`
	} `xml:"meta"`
}

func parseXML(data []byte) Resource {
	var r xmlResource
	if err := xml.Unmarshal(data, &r); err != nil {
		return Resource{}
	}
	var profiles []string
	for _, p := range r.Meta.Profile {
		if v := strings.TrimSpace(p.Value); v != "" {
			profiles = append(profiles, v)
		}
	}
	return Resource{Type: r.XMLName.Local, Profiles: profiles}
}
