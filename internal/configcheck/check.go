// Package configcheck validates a fhirlint.yml config file for unknown keys,
// invalid enum values, and type errors. It reads the raw YAML so that it can
// report line numbers and catch keys that viper silently ignores.
package configcheck

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/fhirlint/fhirlint/internal/lint"
	"github.com/fhirlint/fhirlint/internal/suppress"
	"github.com/fhirlint/fhirlint/internal/validator"
	"go.yaml.in/yaml/v3"
)

// Issue is a single validation problem found in the config file.
type Issue struct {
	Line    int
	Key     string // may be empty for file-level problems
	Message string
}

func (i Issue) String() string {
	if i.Line > 0 {
		return fmt.Sprintf("line %d: %s", i.Line, i.Message)
	}
	return i.Message
}

type valueKind int

const (
	kindString valueKind = iota
	kindBool
	kindInt
	kindEnum
	kindStringList
	kindEnumList
	kindSuppressList
	kindSeverityOverrideList
	kindOverrideList
	kindRuleList
	kindLintMap
	kindMap
)

type keySpec struct {
	kind   valueKind
	values []string // valid enum members for kindEnum and kindEnumList
}

var topLevelKeys = map[string]keySpec{
	"severity":                    {kind: kindEnum, values: []string{"information", "warning", "error"}},
	"fail-on":                     {kind: kindEnum, values: []string{"error", "warning", "information", "never"}},
	"max-warnings":                {kind: kindInt},
	"fhir-version":                {kind: kindEnum, values: validator.FHIRVersionIDs()},
	"profile":                     {kind: kindStringList},
	"profile-map":                 {kind: kindMap},
	"ig":                          {kind: kindStringList},
	"codesystem":                  {kind: kindStringList},
	"valueset":                    {kind: kindStringList},
	"format":                      {kind: kindEnumList, values: []string{"terminal", "json", "html", "junit", "sarif", "markdown", "md", "codeclimate", "github"}},
	"output":                      {kind: kindString},
	"ignore":                      {kind: kindStringList},
	"exclude":                     {kind: kindStringList},
	"no-terminology-server":       {kind: kindBool},
	"terminology-server":          {kind: kindString},
	"allow-insecure-tx":           {kind: kindBool},
	"best-practice":               {kind: kindEnum, values: []string{"ignore", "hint", "warning", "error"}},
	"tx-cache":                    {kind: kindString},
	"tx-log":                      {kind: kindString},
	"locale":                      {kind: kindString},
	"allow-example-urls":          {kind: kindBool},
	"jurisdiction":                {kind: kindString},
	"display-issues-are-warnings": {kind: kindBool},
	"po":                          {kind: kindStringList},
	"watch":                       {kind: kindEnum, values: []string{"single", "all"}},
	"watch-interval":              {kind: kindInt},
	"suppress":                    {kind: kindSuppressList},
	"severity-override":           {kind: kindSeverityOverrideList},
	"show-suppressed":             {kind: kindBool},
	"group":                       {kind: kindBool},
	"cache":                       {kind: kindBool},
	"cache-dir":                   {kind: kindString},
	"timeout":                     {kind: kindString},
	"url-timeout":                 {kind: kindString},
	"baseline":                    {kind: kindString},
	"url":                         {kind: kindStringList},
	"extract":                     {kind: kindString},
	"extract-each":                {kind: kindString},
	"bundle-entries":              {kind: kindBool},
	"skip-non-fhir":               {kind: kindBool},
	"validator-arg":               {kind: kindStringList},
	"validation-timeout":          {kind: kindString},
	"max-messages":                {kind: kindInt},
	"proxy":                       {kind: kindString},
	"https-proxy":                 {kind: kindString},
	"validator-version":           {kind: kindString},
	"require-suppress-reason":     {kind: kindBool},
	"show-source":                 {kind: kindBool},
	"check-references":            {kind: kindBool},
	"since":                       {kind: kindString},
	"tx-offline":                  {kind: kindBool},
	"tx-dir":                      {kind: kindString},
	"server":                      {kind: kindString},
	"quiet":                       {kind: kindBool},
	"no-color":                    {kind: kindBool},
	"overrides":                   {kind: kindOverrideList},
	"rules":                       {kind: kindRuleList},
	"lint":                        {kind: kindLintMap},
}

// KnownKeys returns the set of recognized top-level fhirlint.yml keys.
func KnownKeys() map[string]struct{} {
	out := make(map[string]struct{}, len(topLevelKeys))
	for k := range topLevelKeys {
		out[k] = struct{}{}
	}
	return out
}

var overrideKeys = map[string]keySpec{
	"files":         {kind: kindStringList},
	"ig":            {kind: kindStringList},
	"profile":       {kind: kindStringList},
	"best-practice": {kind: kindEnum, values: []string{"ignore", "hint", "warning", "error"}},
	"severity":      {kind: kindEnum, values: []string{"information", "warning", "error", "fatal"}},
	"fail-on":       {kind: kindEnum, values: []string{"error", "warning", "information", "never"}},
	"suppress":      {kind: kindSuppressList},
}

var suppressKeys = map[string]struct{}{
	"messageId": {}, "messageid": {},
	"constraint": {}, "expression": {}, "pattern": {},
	"severity": {}, "reason": {}, "expires": {},
}

var ruleKeys = map[string]struct{}{
	"id": {}, "resource": {}, "assert": {}, "message": {}, "severity": {},
}

// Check reads the YAML config file at path and returns all validation issues.
// Returns nil, nil if the file does not exist (not an error — config is optional).
func Check(path string) ([]Issue, error) {
	data, err := os.ReadFile(path) //nolint:gosec // caller-supplied path
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return []Issue{{Message: fmt.Sprintf("YAML parse error: %v", err)}}, nil
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return nil, nil // empty file
	}

	root := doc.Content[0]
	return checkMapping(root, topLevelKeys), nil
}

// checkMapping validates every key/value pair in a YAML mapping node.
func checkMapping(node *yaml.Node, specs map[string]keySpec) []Issue {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	var issues []Issue
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]
		key := keyNode.Value
		line := keyNode.Line

		spec, ok := specs[key]
		if !ok {
			msg := fmt.Sprintf("unknown key %q", key)
			if sug := suggest(key, keysOf(specs)); sug != "" {
				msg += fmt.Sprintf(" (did you mean %q?)", sug)
			}
			issues = append(issues, Issue{Line: line, Key: key, Message: msg})
			continue
		}
		issues = append(issues, validateValue(key, line, valNode, spec)...)
	}
	return issues
}

// validateValue checks that a YAML value node matches the expected keySpec.
func validateValue(key string, line int, node *yaml.Node, spec keySpec) []Issue {
	switch spec.kind {
	case kindString:
		if node.Kind != yaml.ScalarNode {
			return []Issue{{Line: line, Key: key, Message: fmt.Sprintf("%q must be a string", key)}}
		}

	case kindBool:
		if node.Kind != yaml.ScalarNode || (node.Value != "true" && node.Value != "false") {
			return []Issue{{Line: line, Key: key, Message: fmt.Sprintf("%q must be a boolean (true or false), got %q", key, node.Value)}}
		}

	case kindInt:
		if node.Kind != yaml.ScalarNode {
			return []Issue{{Line: line, Key: key, Message: fmt.Sprintf("%q must be an integer", key)}}
		}
		if _, err := strconv.Atoi(node.Value); err != nil {
			return []Issue{{Line: line, Key: key, Message: fmt.Sprintf("%q must be an integer, got %q", key, node.Value)}}
		}

	case kindEnum:
		if node.Kind != yaml.ScalarNode {
			return []Issue{{Line: line, Key: key, Message: fmt.Sprintf("%q must be a string", key)}}
		}
		if !contains(spec.values, node.Value) {
			return []Issue{{Line: line, Key: key, Message: fmt.Sprintf("invalid value %q for %q (allowed: %s)", node.Value, key, strings.Join(spec.values, ", "))}}
		}

	case kindStringList:
		return checkStringList(key, line, node)

	case kindEnumList:
		return checkEnumList(key, line, node, spec.values)

	case kindSuppressList:
		return checkSuppressList(key, line, node)

	case kindSeverityOverrideList:
		return checkSeverityOverrideList(key, line, node)

	case kindOverrideList:
		return checkOverrideList(key, line, node)

	case kindRuleList:
		return checkRuleList(key, line, node)

	case kindLintMap:
		return checkLintMap(key, line, node)

	case kindMap:
		// No structural validation for maps
	}
	return nil
}

func checkStringList(key string, line int, node *yaml.Node) []Issue {
	// Accept a single scalar as a one-element list.
	if node.Kind == yaml.ScalarNode {
		return nil
	}
	if node.Kind != yaml.SequenceNode {
		return []Issue{{Line: line, Key: key, Message: fmt.Sprintf("%q must be a list of strings", key)}}
	}
	var issues []Issue
	for _, item := range node.Content {
		if item.Kind != yaml.ScalarNode {
			issues = append(issues, Issue{Line: item.Line, Key: key, Message: fmt.Sprintf("%q items must be strings", key)})
		}
	}
	return issues
}

func checkEnumList(key string, line int, node *yaml.Node, allowed []string) []Issue {
	issues := checkStringList(key, line, node)
	if len(issues) > 0 {
		return issues
	}
	if node.Kind == yaml.ScalarNode {
		if !contains(allowed, node.Value) {
			return []Issue{{Line: line, Key: key, Message: fmt.Sprintf("invalid value %q for %q (allowed: %s)", node.Value, key, strings.Join(allowed, ", "))}}
		}
		return nil
	}
	for _, item := range node.Content {
		if item.Kind == yaml.ScalarNode && !contains(allowed, item.Value) {
			issues = append(issues, Issue{Line: item.Line, Key: key, Message: fmt.Sprintf("invalid value %q for %q (allowed: %s)", item.Value, key, strings.Join(allowed, ", "))})
		}
	}
	return issues
}

func checkSuppressList(key string, line int, node *yaml.Node) []Issue {
	if node.Kind != yaml.SequenceNode {
		return []Issue{{Line: line, Key: key, Message: fmt.Sprintf("%q must be a list", key)}}
	}
	var issues []Issue
	for _, item := range node.Content {
		if item.Kind == yaml.ScalarNode {
			continue // string rule like "messageId:dom-6"
		}
		if item.Kind != yaml.MappingNode {
			issues = append(issues, Issue{Line: item.Line, Key: key, Message: "suppress rule must be a string or map"})
			continue
		}
		issues = append(issues, checkSuppressMap(item)...)
	}
	return issues
}

func checkSuppressMap(node *yaml.Node) []Issue {
	var issues []Issue
	hasType := false
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i].Value
		line := node.Content[i].Line
		if _, ok := suppressKeys[k]; !ok {
			msg := fmt.Sprintf("unknown suppress key %q", k)
			issues = append(issues, Issue{Line: line, Key: k, Message: msg})
			continue
		}
		if k == "messageId" || k == "messageid" || k == "constraint" || k == "expression" || k == "pattern" {
			hasType = true
		}
		if k == "severity" {
			valNode := node.Content[i+1]
			allowed := []string{"information", "warning", "error", "fatal"}
			if !contains(allowed, valNode.Value) {
				issues = append(issues, Issue{Line: line, Key: k, Message: fmt.Sprintf("invalid value %q for suppress severity (allowed: %s)", valNode.Value, strings.Join(allowed, ", "))})
			}
		}
		// Validated with the same function that parsing uses, so `config check`
		// cannot disagree with `validate` about what a valid date looks like.
		if k == "expires" {
			valNode := node.Content[i+1]
			if _, err := suppress.ParseExpiry(valNode.Value); err != nil {
				issues = append(issues, Issue{Line: line, Key: k, Message: fmt.Sprintf("invalid suppress expires %q: use YYYY-MM-DD", valNode.Value)})
			}
		}
	}
	if !hasType {
		issues = append(issues, Issue{Line: node.Line, Message: "suppress rule must have one of: messageId, constraint, expression, pattern"})
	}
	return issues
}

// checkSeverityOverrideList validates `severity-override:`. It shares the
// suppression selector keys, but `severity` means something different here —
// the level to apply, not the level to match — so it is required, and the
// string shorthand a suppression allows cannot express a rule at all.
func checkSeverityOverrideList(key string, line int, node *yaml.Node) []Issue {
	if node.Kind != yaml.SequenceNode {
		return []Issue{{Line: line, Key: key, Message: fmt.Sprintf("%q must be a list", key)}}
	}
	var issues []Issue
	for _, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			issues = append(issues, Issue{Line: item.Line, Key: key,
				Message: "severity-override rule must be a map with a selector and a severity"})
			continue
		}
		issues = append(issues, checkSeverityOverrideMap(item)...)
	}
	return issues
}

func checkSeverityOverrideMap(node *yaml.Node) []Issue {
	var issues []Issue
	hasType, hasSeverity := false, false
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i].Value
		line := node.Content[i].Line
		if _, ok := suppressKeys[k]; !ok {
			issues = append(issues, Issue{Line: line, Key: k,
				Message: fmt.Sprintf("unknown severity-override key %q", k)})
			continue
		}
		switch k {
		case "messageId", "messageid", "constraint", "expression", "pattern":
			hasType = true
		case "severity":
			hasSeverity = true
			allowed := suppress.SeverityLevels()
			if !contains(allowed, node.Content[i+1].Value) {
				issues = append(issues, Issue{Line: line, Key: k,
					Message: fmt.Sprintf("invalid value %q for severity-override severity (allowed: %s)",
						node.Content[i+1].Value, strings.Join(allowed, ", "))})
			}
		case "expires":
			if _, err := suppress.ParseExpiry(node.Content[i+1].Value); err != nil {
				issues = append(issues, Issue{Line: line, Key: k,
					Message: fmt.Sprintf("invalid severity-override expires %q: use YYYY-MM-DD", node.Content[i+1].Value)})
			}
		}
	}
	if !hasType {
		issues = append(issues, Issue{Line: node.Line,
			Message: "severity-override rule must have one of: messageId, constraint, expression, pattern"})
	}
	if !hasSeverity {
		issues = append(issues, Issue{Line: node.Line,
			Message: "severity-override rule must have a severity: the level to apply"})
	}
	return issues
}

func checkRuleList(key string, line int, node *yaml.Node) []Issue {
	if node.Kind != yaml.SequenceNode {
		return []Issue{{Line: line, Key: key, Message: fmt.Sprintf("%q must be a list", key)}}
	}
	var issues []Issue
	for _, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			issues = append(issues, Issue{Line: item.Line, Key: key, Message: "rule must be a map with id and assert"})
			continue
		}
		issues = append(issues, checkRuleMap(item)...)
	}
	return issues
}

func checkRuleMap(node *yaml.Node) []Issue {
	var issues []Issue
	hasID, hasAssert := false, false
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i].Value
		line := node.Content[i].Line
		if _, ok := ruleKeys[k]; !ok {
			issues = append(issues, Issue{Line: line, Key: k, Message: fmt.Sprintf("unknown rule key %q", k)})
			continue
		}
		switch k {
		case "id":
			hasID = true
		case "assert":
			hasAssert = true
		case "severity":
			valNode := node.Content[i+1]
			allowed := []string{"information", "warning", "error"}
			if !contains(allowed, valNode.Value) {
				issues = append(issues, Issue{Line: line, Key: k, Message: fmt.Sprintf("invalid value %q for rule severity (allowed: %s)", valNode.Value, strings.Join(allowed, ", "))})
			}
		}
	}
	if !hasID {
		issues = append(issues, Issue{Line: node.Line, Message: "rule must have an id"})
	}
	if !hasAssert {
		issues = append(issues, Issue{Line: node.Line, Message: "rule must have an assert expression"})
	}
	return issues
}

var lintSeverities = []string{"information", "warning", "error"}

func checkLintMap(key string, line int, node *yaml.Node) []Issue {
	if node.Kind != yaml.MappingNode {
		return []Issue{{Line: line, Key: key, Message: fmt.Sprintf("%q must be a mapping of rule -> severity", key)}}
	}
	names := lintRuleNames()
	var issues []Issue
	for i := 0; i+1 < len(node.Content); i += 2 {
		ruleNode := node.Content[i]
		valNode := node.Content[i+1]
		name := ruleNode.Value

		meta, known := lint.Known(name)
		if !known {
			msg := fmt.Sprintf("unknown lint rule %q", name)
			if sug := suggest(name, names); sug != "" {
				msg += fmt.Sprintf(" (did you mean %q?)", sug)
			}
			issues = append(issues, Issue{Line: ruleNode.Line, Key: name, Message: msg})
			continue
		}
		issues = append(issues, checkLintRuleValue(name, meta, valNode)...)
	}
	return issues
}

func checkLintRuleValue(name string, meta lint.RuleMeta, node *yaml.Node) []Issue {
	switch node.Kind {
	case yaml.ScalarNode:
		if !contains(lintSeverities, node.Value) {
			return []Issue{{Line: node.Line, Key: name, Message: fmt.Sprintf("invalid severity %q for lint rule %q (allowed: %s)", node.Value, name, strings.Join(lintSeverities, ", "))}}
		}
		var issues []Issue
		for _, rp := range meta.RequiredParams {
			issues = append(issues, Issue{Line: node.Line, Key: name, Message: fmt.Sprintf("lint rule %q requires parameter %q — use the map form with severity and %s", name, rp, rp)})
		}
		return issues
	case yaml.MappingNode:
		var issues []Issue
		present := map[string]bool{}
		for i := 0; i+1 < len(node.Content); i += 2 {
			k := node.Content[i].Value
			v := node.Content[i+1]
			if k == "severity" {
				if !contains(lintSeverities, v.Value) {
					issues = append(issues, Issue{Line: node.Content[i].Line, Key: name, Message: fmt.Sprintf("invalid severity %q for lint rule %q (allowed: %s)", v.Value, name, strings.Join(lintSeverities, ", "))})
				}
				continue
			}
			present[k] = true
		}
		for _, rp := range meta.RequiredParams {
			if !present[rp] {
				issues = append(issues, Issue{Line: node.Line, Key: name, Message: fmt.Sprintf("lint rule %q requires parameter %q", name, rp)})
			}
		}
		return issues
	default:
		return []Issue{{Line: node.Line, Key: name, Message: fmt.Sprintf("lint rule %q must be a severity string or a map", name)}}
	}
}

func lintRuleNames() []string {
	metas := lint.Rules()
	out := make([]string, 0, len(metas))
	for _, m := range metas {
		out = append(out, m.Name)
	}
	return out
}

func checkOverrideList(key string, line int, node *yaml.Node) []Issue {
	if node.Kind != yaml.SequenceNode {
		return []Issue{{Line: line, Key: key, Message: fmt.Sprintf("%q must be a list", key)}}
	}
	var issues []Issue
	for _, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			issues = append(issues, Issue{Line: item.Line, Key: key, Message: "override entry must be a map"})
			continue
		}
		issues = append(issues, checkOverrideMap(item)...)
	}
	return issues
}

func checkOverrideMap(node *yaml.Node) []Issue {
	issues := checkMapping(node, overrideKeys)
	// Warn if files: is missing (required for matching to work).
	hasFiles := false
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == "files" {
			hasFiles = true
			break
		}
	}
	if !hasFiles {
		issues = append(issues, Issue{Line: node.Line, Message: "override entry is missing required key \"files\""})
	}
	return issues
}

// suggest returns the closest key from candidates if the edit distance is ≤ 3.
func suggest(typo string, candidates []string) string {
	best, bestDist := "", len(typo)+1
	for _, c := range candidates {
		d := levenshtein(typo, c)
		if d < bestDist {
			bestDist, best = d, c
		}
	}
	if bestDist <= 3 {
		return best
	}
	return ""
}

func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	dp := make([]int, lb+1)
	for j := range dp {
		dp[j] = j
	}
	for i := 1; i <= la; i++ {
		prev := dp[0]
		dp[0] = i
		for j := 1; j <= lb; j++ {
			tmp := dp[j]
			if a[i-1] == b[j-1] {
				dp[j] = prev
			} else {
				m := dp[j]
				if dp[j-1] < m {
					m = dp[j-1]
				}
				if prev < m {
					m = prev
				}
				dp[j] = 1 + m
			}
			prev = tmp
		}
	}
	return dp[lb]
}

func keysOf(m map[string]keySpec) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
