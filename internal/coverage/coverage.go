package coverage

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ElementReport is the outcome for one mustSupport element.
type ElementReport struct {
	ID        string `json:"id"`
	Populated bool   `json:"populated"`

	// Unresolved marks an element whose slice membership could not be decided.
	// It counts towards neither covered nor uncovered: the run did not measure
	// it, and a report that hid that would be reporting a guess.
	Unresolved bool   `json:"unresolved,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// ProfileReport is the coverage of one profile across the scanned resources.
type ProfileReport struct {
	URL     string `json:"url"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Package string `json:"package,omitempty"`

	// Resources is how many scanned resources were attributed to this profile.
	Resources int `json:"resources"`

	// ByType counts the resources attributed by resource type rather than by a
	// declared meta.profile. Coverage measured that way describes the dataset,
	// not conformance to this profile, so the count is surfaced.
	ByType int `json:"byType,omitempty"`

	Elements []ElementReport `json:"elements"`
	Warnings []string        `json:"warnings,omitempty"`
}

// Covered counts elements populated by at least one resource.
func (p ProfileReport) Covered() int {
	n := 0
	for _, e := range p.Elements {
		if e.Populated {
			n++
		}
	}
	return n
}

// Unresolved counts elements that could not be measured.
func (p ProfileReport) Unresolved() int {
	n := 0
	for _, e := range p.Elements {
		if e.Unresolved {
			n++
		}
	}
	return n
}

// Measurable is the number of elements the run could actually decide, and the
// denominator every percentage here uses. Unresolved elements are excluded so
// that a coverage figure never silently counts an unmeasured element as a miss.
func (p ProfileReport) Measurable() int {
	return len(p.Elements) - p.Unresolved()
}

// Percent is the share of measurable elements that are populated. A profile
// with nothing measurable returns 100: there is no evidence of a gap, and
// inventing a zero would read as a failure the data does not support.
func (p ProfileReport) Percent() float64 {
	m := p.Measurable()
	if m == 0 {
		return 100
	}
	return float64(p.Covered()) / float64(m) * 100
}

// Missing returns the IDs of measurable elements no resource populated.
func (p ProfileReport) Missing() []string {
	var out []string
	for _, e := range p.Elements {
		if !e.Populated && !e.Unresolved {
			out = append(out, e.ID)
		}
	}
	return out
}

// Report is the result of a coverage run.
type Report struct {
	Profiles []ProfileReport `json:"profiles"`

	// ResourcesScanned counts resources read, whether or not any profile
	// claimed them. Unattributed counts those no profile claimed — usually a
	// dataset whose resources declare no meta.profile.
	ResourcesScanned int `json:"resourcesScanned"`
	Unattributed     int `json:"unattributed,omitempty"`

	// ProfilesWithoutResources counts profiles no resource was attributed to.
	// They are left out of Profiles rather than listed at zero: nothing was
	// measured for them, and a row reading "0%" would be a verdict on data that
	// does not exist.
	ProfilesWithoutResources int `json:"profilesWithoutResources,omitempty"`

	// SkippedFiles lists inputs that could not contribute, with the reason.
	SkippedFiles []SkippedFile `json:"skippedFiles,omitempty"`
}

// SkippedFile records an input that was not measured.
type SkippedFile struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// Resource is one parsed FHIR resource.
type Resource struct {
	Path     string
	Type     string
	Profiles []string
	Body     map[string]any
}

// Options configures a coverage run.
type Options struct {
	// AttributeByType allows resources without a matching meta.profile to be
	// measured against a profile of the same resource type. Enabled when the
	// caller named specific profiles: they have said which profile the data is
	// meant to conform to. With a whole package loaded it stays off, because
	// attributing a bare Patient to all thirty Patient profiles in an IG would
	// be noise rather than information.
	AttributeByType bool
}

// Run measures the resources against the profiles and returns the report.
func Run(reg *Registry, profiles []*StructureDefinition, resources []Resource, opts Options) Report {
	var rep Report
	rep.ResourcesScanned = len(resources)

	claimed := make([]bool, len(resources))

	for _, sd := range profiles {
		resolved := reg.Resolve(sd)
		pr := ProfileReport{
			URL:      sd.URL,
			Name:     profileName(sd),
			Type:     sd.Type,
			Package:  sd.Package,
			Warnings: resolved.Warnings,
		}

		var matched []int
		for i, res := range resources {
			switch {
			case declaresProfile(res, sd.URL):
				matched = append(matched, i)
				claimed[i] = true
			case opts.AttributeByType && res.Type == sd.Type && len(res.Profiles) == 0:
				matched = append(matched, i)
				claimed[i] = true
				pr.ByType++
			}
		}
		pr.Resources = len(matched)

		if len(matched) == 0 {
			if len(resolved.MustSupport) > 0 {
				rep.ProfilesWithoutResources++
			}
			continue
		}

		for _, id := range resolved.MustSupport {
			er := ElementReport{ID: id}
			for _, i := range matched {
				populated, reason := reg.Populated(sd, resources[i].Body, id)
				if reason != "" {
					er.Unresolved = true
					er.Reason = reason
					break
				}
				if populated {
					er.Populated = true
					break
				}
			}
			pr.Elements = append(pr.Elements, er)
		}

		if len(pr.Elements) == 0 {
			// A profile that declares no mustSupport element has nothing to
			// report. Listing it would bury the profiles that do.
			continue
		}
		rep.Profiles = append(rep.Profiles, pr)
	}

	for i := range resources {
		if !claimed[i] {
			rep.Unattributed++
		}
	}

	sort.Slice(rep.Profiles, func(i, j int) bool {
		if rep.Profiles[i].Percent() != rep.Profiles[j].Percent() {
			return rep.Profiles[i].Percent() < rep.Profiles[j].Percent()
		}
		return rep.Profiles[i].Name < rep.Profiles[j].Name
	})
	return rep
}

// profileName prefers the human-readable name, falling back to the id and then
// the last path segment of the canonical URL.
func profileName(sd *StructureDefinition) string {
	if sd.Name != "" {
		return sd.Name
	}
	if sd.ID != "" {
		return sd.ID
	}
	if i := strings.LastIndex(sd.URL, "/"); i >= 0 {
		return sd.URL[i+1:]
	}
	return sd.URL
}

// declaresProfile reports whether the resource claims conformance to url. The
// |version suffix a meta.profile may carry is ignored for the comparison.
func declaresProfile(res Resource, url string) bool {
	for _, p := range res.Profiles {
		if p == url {
			return true
		}
		if i := strings.Index(p, "|"); i >= 0 && p[:i] == url {
			return true
		}
	}
	return false
}

// LoadResources reads FHIR resources from a path, which may be a file or a
// directory. Only JSON and NDJSON are read: coverage walks the decoded resource
// tree, and XML would have to be converted first. XML inputs are reported as
// skipped rather than passed over in silence.
func LoadResources(root string, exclude []string) ([]Resource, []SkippedFile, error) {
	var resources []Resource
	var skipped []SkippedFile

	info, err := os.Stat(root)
	if err != nil {
		return nil, nil, err
	}

	walk := func(path string) {
		if isExcluded(path, exclude) {
			return
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".json":
			res, err := loadJSON(path)
			if err != nil {
				skipped = append(skipped, SkippedFile{Path: path, Reason: err.Error()})
				return
			}
			if res != nil {
				resources = append(resources, *res)
			}
		case ".ndjson":
			rs, err := loadNDJSON(path)
			if err != nil {
				skipped = append(skipped, SkippedFile{Path: path, Reason: err.Error()})
				return
			}
			resources = append(resources, rs...)
		case ".xml":
			skipped = append(skipped, SkippedFile{Path: path, Reason: "XML input is not supported by coverage"})
		}
	}

	if !info.IsDir() {
		walk(root)
		return resources, skipped, nil
	}

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		walk(path)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	sort.Slice(resources, func(i, j int) bool { return resources[i].Path < resources[j].Path })
	return resources, skipped, nil
}

func isExcluded(path string, patterns []string) bool {
	for _, p := range patterns {
		if ok, _ := filepath.Match(p, filepath.Base(path)); ok {
			return true
		}
		if strings.Contains(filepath.ToSlash(path), strings.TrimSuffix(p, "/")+"/") {
			return true
		}
	}
	return false
}

func loadJSON(path string) (*Resource, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path from the user-named input tree
	if err != nil {
		return nil, err
	}
	return decodeResource(path, data)
}

func loadNDJSON(path string) ([]Resource, error) {
	f, err := os.Open(path) //nolint:gosec // path from the user-named input tree
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var out []Resource
	sc := bufio.NewScanner(f)
	// Bulk-export lines routinely exceed bufio's default 64 KiB ceiling.
	sc.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		res, err := decodeResource(path, []byte(line))
		if err != nil || res == nil {
			continue
		}
		out = append(out, *res)
	}
	return out, sc.Err()
}

func decodeResource(path string, data []byte) (*Resource, error) {
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, err
	}
	rt, _ := body["resourceType"].(string)
	if rt == "" {
		// Not a FHIR resource. Config files and fixtures live alongside
		// examples often enough that this is not worth reporting as a problem.
		return nil, nil
	}
	return &Resource{
		Path:     path,
		Type:     rt,
		Profiles: metaProfiles(body),
		Body:     body,
	}, nil
}

func metaProfiles(body map[string]any) []string {
	meta, ok := body["meta"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := meta["profile"].([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, p := range raw {
		if s, ok := p.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
