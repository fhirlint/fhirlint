package configcheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// committedSchemaPath is the generated schema checked into the repo root, which
// is what the published $schema URL serves.
const committedSchemaPath = "../../fhirlint.schema.json"

// TestCommittedSchemaIsUpToDate is the drift guard: adding a key or changing an
// enum in topLevelKeys without regenerating the file fails here rather than
// silently publishing a stale schema to every editor pointing at the URL.
func TestCommittedSchemaIsUpToDate(t *testing.T) {
	generated, err := Schema()
	if err != nil {
		t.Fatalf("Schema() error: %v", err)
	}
	committed, err := os.ReadFile(committedSchemaPath)
	if err != nil {
		t.Fatalf("reading %s: %v", committedSchemaPath, err)
	}
	// Compare parsed documents so trailing-newline differences do not matter.
	var g, c any
	if err := json.Unmarshal(generated, &g); err != nil {
		t.Fatalf("generated schema is not valid JSON: %v", err)
	}
	if err := json.Unmarshal(committed, &c); err != nil {
		t.Fatalf("committed schema is not valid JSON: %v", err)
	}
	gs, _ := json.Marshal(g)
	cs, _ := json.Marshal(c)
	if string(gs) != string(cs) {
		t.Errorf("fhirlint.schema.json is out of date — regenerate it with:\n\n\tgo run . config schema > fhirlint.schema.json\n")
	}
}

// TestSchemaCoversEveryKey asserts the schema describes exactly the keys
// `config check` knows about, in both directions.
func TestSchemaCoversEveryKey(t *testing.T) {
	data, err := Schema()
	if err != nil {
		t.Fatalf("Schema() error: %v", err)
	}
	var doc struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for key := range topLevelKeys {
		if _, ok := doc.Properties[key]; !ok {
			t.Errorf("key %q is known to config check but missing from the schema", key)
		}
	}
	for key := range doc.Properties {
		if _, ok := topLevelKeys[key]; !ok {
			t.Errorf("key %q is in the schema but unknown to config check", key)
		}
	}
}

// TestSchemaCoversEveryKind fails when a new valueKind is added without a case
// in schemaForKind, which would otherwise silently emit an unconstrained value.
func TestSchemaCoversEveryKind(t *testing.T) {
	kinds := []valueKind{
		kindString, kindBool, kindInt, kindEnum, kindStringList, kindEnumList,
		kindSuppressList, kindOverrideList, kindRuleList, kindLintMap, kindMap,
	}
	for _, k := range kinds {
		got := schemaForKind(keySpec{kind: k, values: []string{"a", "b"}})
		if len(got) == 0 {
			t.Errorf("valueKind %d has no case in schemaForKind", k)
		}
	}
	// Guard the guard: the list above must cover every kind that exists. kindMap
	// is the highest constant, so anything added after it is caught here.
	if unhandled := schemaForKind(keySpec{kind: kindMap + 1}); len(unhandled) != 0 {
		t.Error("a new valueKind was added past kindMap — add it to schemaForKind and to this test")
	}
}

// TestSchemaRejectsUnknownKeys documents that the schema mirrors config check's
// treatment of typos rather than silently allowing them.
func TestSchemaRejectsUnknownKeys(t *testing.T) {
	data, err := Schema()
	if err != nil {
		t.Fatalf("Schema() error: %v", err)
	}
	var doc struct {
		AdditionalProperties *bool `json:"additionalProperties"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc.AdditionalProperties == nil || *doc.AdditionalProperties {
		t.Error("schema must set additionalProperties:false so editors flag unknown keys")
	}
}

// TestExampleConfigMatchesSchemaKeys checks that every key used in the shipped
// example is one the schema describes. Full JSON Schema validation of the
// example is covered in CI, which has a schema validator available.
func TestExampleConfigMatchesSchemaKeys(t *testing.T) {
	path := filepath.Join("..", "..", "fhirlint.yml.example")
	issues, err := Check(path)
	if err != nil {
		t.Fatalf("Check(%s): %v", path, err)
	}
	if len(issues) != 0 {
		t.Errorf("fhirlint.yml.example does not pass config check: %v", issues)
	}
}
