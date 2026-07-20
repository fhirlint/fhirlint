package configcheck

import (
	"encoding/json"
	"sort"
)

// SchemaID is where the generated schema is published, and what a fhirlint.yml
// modeline points at.
const SchemaID = "https://raw.githubusercontent.com/fhirlint/fhirlint/main/fhirlint.schema.json"

// Schema returns a JSON Schema describing fhirlint.yml, generated from the same
// topLevelKeys map that `fhirlint config check` validates against. Deriving it
// rather than maintaining a second copy is the point: the two cannot disagree
// about which keys exist or which enum values are valid.
//
// Structured values (suppress, overrides, rules, lint, profile-map) are
// described loosely — the schema states their shape, while `config check`
// remains the authority on their contents.
func Schema() ([]byte, error) {
	props := make(map[string]any, len(topLevelKeys))
	for name, spec := range topLevelKeys {
		props[name] = schemaForKind(spec)
	}
	doc := map[string]any{
		"$schema":     "http://json-schema.org/draft-07/schema#",
		"$id":         SchemaID,
		"title":       "fhirlint configuration",
		"description": "Project-level configuration for fhirlint (fhirlint.yml). CLI flags always take precedence over values in this file.",
		"type":        "object",
		// Unknown keys are an error in `config check`, so the schema mirrors that
		// and editors flag typos the same way the CLI would.
		"additionalProperties": false,
		"properties":           props,
	}
	return json.MarshalIndent(doc, "", "  ")
}

func schemaForKind(spec keySpec) map[string]any {
	// `config check` tolerates a null value for some kinds — a key written with
	// its list entirely commented out parses as null, which fhirlint.yml.example
	// itself does. The schema mirrors that exactly, kind by kind, so an editor
	// never flags a file the CLI accepts (verified against `config check`).
	switch spec.kind {
	case kindString:
		return map[string]any{"type": []string{"string", "null"}}
	case kindBool:
		return map[string]any{"type": "boolean"}
	case kindInt:
		return map[string]any{"type": "integer"}
	case kindEnum:
		return map[string]any{"type": "string", "enum": sortedCopy(spec.values)}
	case kindStringList:
		return map[string]any{
			"type":  []string{"array", "null"},
			"items": map[string]any{"type": "string"},
		}
	case kindEnumList:
		return map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string", "enum": sortedCopy(spec.values)},
		}
	case kindSuppressList:
		return objectListSchema(sortedKeys(suppressKeys))
	case kindRuleList:
		return objectListSchema(sortedKeys(ruleKeys))
	case kindOverrideList:
		props := make(map[string]any, len(overrideKeys))
		for name, s := range overrideKeys {
			props[name] = schemaForKind(s)
		}
		return map[string]any{
			"type": "array",
			"items": map[string]any{
				"type":                 "object",
				"properties":           props,
				"additionalProperties": false,
			},
		}
	case kindLintMap:
		return map[string]any{"type": "object"}
	case kindMap:
		return map[string]any{"type": []string{"object", "null"}}
	default:
		// New kinds must be handled explicitly; an unconstrained value is the
		// safe fallback, and TestSchemaCoversEveryKind fails so it gets one.
		return map[string]any{}
	}
}

// objectListSchema describes an array of objects restricted to the given keys.
func objectListSchema(keys []string) map[string]any {
	props := make(map[string]any, len(keys))
	for _, k := range keys {
		props[k] = map[string]any{}
	}
	return map[string]any{
		"type": "array",
		"items": map[string]any{
			"type":                 "object",
			"properties":           props,
			"additionalProperties": false,
		},
	}
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
