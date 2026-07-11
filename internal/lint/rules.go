package lint

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
)

// rawFinding is a single convention violation before a severity (from config)
// and message ID are attached.
type rawFinding struct {
	message  string
	location string // resource-relative element path, e.g. "id" or "url"
}

// checkFunc runs a built-in rule against a resource. params holds the rule's
// configured parameters (e.g. "base" for canonical-url-pattern).
type checkFunc func(res gjson.Result, params map[string]string) []rawFinding

// builtinRule defines one convention check.
type builtinRule struct {
	name           string
	description    string
	defaultSev     string
	requiredParams []string
	check          checkFunc
}

var (
	kebabRe  = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	pascalRe = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)
)

// registry holds every built-in rule, keyed by name.
var registry = map[string]builtinRule{
	"id-kebab-case": {
		name:        "id-kebab-case",
		description: "Resource id should be lowercase, hyphen-separated (kebab-case).",
		defaultSev:  "warning",
		check: func(res gjson.Result, _ map[string]string) []rawFinding {
			id := res.Get("id")
			if !id.Exists() {
				return nil
			}
			if s := id.String(); !kebabRe.MatchString(s) {
				return []rawFinding{{
					message:  fmt.Sprintf("resource id %q is not lowercase kebab-case (letters, digits, hyphen-separated)", s),
					location: "id",
				}}
			}
			return nil
		},
	},
	"canonical-url-pattern": {
		name:           "canonical-url-pattern",
		description:    "A resource's canonical url must start with a configured base URL.",
		defaultSev:     "error",
		requiredParams: []string{"base"},
		check: func(res gjson.Result, params map[string]string) []rawFinding {
			base := params["base"]
			url := res.Get("url")
			if !url.Exists() {
				return nil
			}
			if s := url.String(); !strings.HasPrefix(s, base) {
				return []rawFinding{{
					message:  fmt.Sprintf("canonical url %q does not start with the configured base %q", s, base),
					location: "url",
				}}
			}
			return nil
		},
	},
	"profile-name-pascalcase": {
		name:        "profile-name-pascalcase",
		description: "StructureDefinition.name should be PascalCase.",
		defaultSev:  "warning",
		check: func(res gjson.Result, _ map[string]string) []rawFinding {
			if res.Get("resourceType").String() != "StructureDefinition" {
				return nil
			}
			name := res.Get("name")
			if !name.Exists() {
				return nil
			}
			if s := name.String(); !pascalRe.MatchString(s) {
				return []rawFinding{{
					message:  fmt.Sprintf("StructureDefinition.name %q is not PascalCase (start uppercase, letters and digits only)", s),
					location: "name",
				}}
			}
			return nil
		},
	},
}
