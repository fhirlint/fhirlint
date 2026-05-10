# Validating partial JSON with --extract and --extract-each

Not every API response is a pure FHIR resource. Internal systems often wrap FHIR resources in proprietary envelopes, add metadata layers, or return arrays of resources outside a FHIR Bundle. This guide explains how to validate only the FHIR-conformant parts of a JSON document without modifying the source.

## Table of contents

- [The problem: non-FHIR wrappers](#the-problem-non-fhir-wrappers)
- [--extract: single resource from a wrapper](#--extract-single-resource-from-a-wrapper)
- [--extract-each: multiple resources from an array](#--extract-each-multiple-resources-from-an-array)
- [Combining with --ignore](#combining-with---ignore)
- [Combining with --url](#combining-with---url)
- [JSONPath syntax](#jsonpath-syntax)
- [Typical patterns](#typical-patterns)

---

## The problem: non-FHIR wrappers

APIs often look like this — a FHIR resource embedded inside a custom envelope:

```json
{
  "status": "ok",
  "requestId": "abc-123",
  "generatedAt": "2026-05-10T12:00:00Z",
  "data": {
    "fhir": {
      "resourceType": "Patient",
      "id": "pat-1",
      "name": [{ "family": "Müller", "given": ["Hans"] }]
    }
  }
}
```

If you pass this directly to fhirlint, the validator rejects it immediately — `resourceType` is missing at the root, and `status`/`requestId` are not valid FHIR fields. The actual Patient resource is buried at `$.data.fhir`.

---

## `--extract`: single resource from a wrapper

`--extract` takes a JSONPath expression, extracts the value at that path, and validates it as the FHIR resource. The outer wrapper is discarded.

```bash
fhirlint validate api-response.json --extract "$.data.fhir"
```

This is equivalent to running:

```bash
cat api-response.json | jq '.data.fhir' | fhirlint validate
```

But it keeps the label in the output tied to the original filename, and it works with all output formats and flags.

### Nested paths

JSONPath supports any level of nesting:

```bash
# $.entry[0].resource — FHIR Bundle-style: extract the first entry's resource
fhirlint validate bundle-response.json --extract "$.entry[0].resource"

# $.payload.records[2].resource — deeply nested
fhirlint validate payload.json --extract "$.payload.records[2].resource"
```

### With `--url`

`--extract` also works when fetching from an HTTP endpoint:

```bash
fhirlint validate \
  --url https://my-api.internal/patient/1 \
  --extract "$.data.fhir"
```

fhirlint fetches the URL, extracts the value at the given path, and validates it.

---

## `--extract-each`: multiple resources from an array

When an API returns an array of FHIR resources (not a FHIR Bundle), `--extract-each` extracts every element and validates them as separate resources — all in a single JVM invocation:

```json
{
  "status": "ok",
  "medications": [
    { "resourceType": "Medication", "id": "med-001", "code": { ... } },
    { "resourceType": "Medication", "id": "med-002", "code": { ... } }
  ]
}
```

```bash
fhirlint validate api-response.json --extract-each "$.medications"
```

Output:

```
▶ api-response.json[0] (Medication/med-001)
  ✓ Valid

▶ api-response.json[1] (Medication/med-002)
  ✗ ERROR  Medication.code: minimum required = 1, but only found 0
           @ Medication.code (line 8, col 5)

────────────────────────────────────────
Files: 2  Valid: 1  Errors: 1  Warnings: 0
```

Each result is labelled `filename[index]`. When the element has `resourceType` and `id`, the label includes them for easier identification: `api-response.json[0] (Medication/med-001)`.

The exit code reflects the combined result across all elements, following the `--fail-on` setting.

### With `--url`

```bash
fhirlint validate \
  --url https://my-api.internal/medications \
  --extract-each "$.medications"
```

### Note: `--extract` and `--extract-each` are mutually exclusive

Use `--extract` when the path points to a single object, and `--extract-each` when it points to an array. Using both in the same invocation returns an error.

---

## Combining with `--ignore`

Sometimes the extracted resource itself contains non-conformant fields — for example, proprietary extensions or fields your IG doesn't define. Use `--ignore` to strip them before validation.

`--ignore` is applied to the **outer document before extraction**, so it can strip fields from the wrapper too:

```bash
# Strip a non-conformant meta.tag from the extracted Patient
fhirlint validate api-response.json \
  --extract "$.data.fhir" \
  --ignore "$.data.fhir.meta.tag"
```

With `--extract-each`, `--ignore` paths are applied to each extracted element individually — use a path relative to the element root:

```bash
# Each Medication element has a non-conformant "internalId" field
fhirlint validate api-response.json \
  --extract-each "$.medications" \
  --ignore "$.internalId"
```

---

## Combining with `--url`

Both flags work with all input sources:

```bash
# Extract from a live API response
fhirlint validate --url https://my-api/patient/1 --extract "$.fhir"

# Validate each resource from an array endpoint
fhirlint validate --url https://my-api/medications --extract-each "$.items"

# Multiple URLs, each with extraction
fhirlint validate \
  --url https://my-api/patient/1 \
  --url https://my-api/patient/2 \
  --extract "$.fhir"
```

Note: `--extract-each` can only be combined with a **single** `--url`. For multiple URLs, use `--extract` or validate without extraction.

---

## JSONPath syntax

fhirlint uses a simplified JSONPath syntax:

| Syntax | Meaning |
|--------|---------|
| `$.field` | Top-level field |
| `$.parent.child` | Nested field |
| `$.array[0]` | First element of an array |
| `$.array[2].field` | Field on the third array element |

The leading `$.` is optional — `data.fhir` and `$.data.fhir` are equivalent.

---

## Typical patterns

### Internal API with envelope

```json
{ "meta": { ... }, "payload": { "resourceType": "MedicationRequest", ... } }
```

```bash
fhirlint validate response.json --extract "$.payload"
```

### FHIR server response with extra metadata

Some FHIR servers add non-standard top-level fields to their responses:

```json
{ "resourceType": "Patient", "id": "1", "_serverMeta": { ... }, "name": [...] }
```

```bash
fhirlint validate response.json --ignore "$._serverMeta"
```

No extraction needed here — `resourceType` is already at the root. Just strip the offending field.

### Bulk export array

```json
{ "exported": "2026-05-10", "resources": [ {...}, {...}, {...} ] }
```

```bash
fhirlint validate export.json --extract-each "$.resources"
```

### ETL pipeline fixture

A test fixture for an ETL pipeline stores the input, the expected output, and the FHIR resource together:

```json
{
  "input": { "hl7v2": "..." },
  "expected": { "fhir": { "resourceType": "Patient", ... } }
}
```

```bash
fhirlint validate fixture.json --extract "$.expected.fhir"
```

This lets you validate FHIR correctness without restructuring your test fixtures.
