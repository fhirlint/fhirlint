# Custom lint rules

Custom lint rules let you assert project-specific expectations that profile
validation cannot express — for example "every `Patient` must carry an MRN
identifier" or "no reference may point at `example.org`". Each rule is a
[FHIRPath](https://hl7.org/fhirpath/) expression evaluated against every
validated resource; when the assertion does not hold, fhirlint emits a finding.

Rule findings behave like any other issue: they appear in every reporter
(terminal, JSON, HTML, JUnit, SARIF, Markdown, CodeClimate), respect
`--severity` and `--fail-on`, and can be suppressed or baselined.

## Defining rules

Rules live under a `rules:` key in `fhirlint.yml`, or in a standalone file
loaded with `--rules-file` (which takes precedence over the config key).

```yaml
rules:
  - id: patient-needs-mrn
    resource: Patient
    assert: "identifier.where(system='http://hospital.example/mrn').exists()"
    message: "Patient is missing an MRN identifier"
    severity: error
```

| Field | Required | Description |
|-------|----------|-------------|
| `id` | yes | Stable identifier. Forms the `rule:<id>` message ID and the suppression key. Letters, digits, `.`, `_`, `-`. Must be unique. |
| `assert` | yes | A FHIRPath expression that should evaluate to `true`. An empty or `false` result is a finding. |
| `resource` | no | A resourceType (e.g. `Patient`). When set, the rule only applies to resources of that type. |
| `message` | no | The message shown when the rule fails. Defaults to a description built from the assertion. |
| `severity` | no | `error`, `warning`, or `information`. Defaults to `error`. Only `error` fails the build (subject to `--fail-on`). |

A standalone rules file may use the same `rules:` list, or a bare list:

```yaml
# lint-rules.yml
- id: has-name
  assert: "name.exists()"
- id: active-flag
  resource: Patient
  assert: "active.exists()"
  severity: warning
```

## How assertions are evaluated

- The expression is evaluated with the resource as the context, exactly like a
  FHIR invariant. A leading resourceType acts as a type filter, so both
  `Patient.identifier.exists()` and `identifier.exists()` work.
- The top-level expression must yield a single boolean. An empty result (for
  example comparing an absent element) counts as **not satisfied** (`false`).
- Rules run in-process against FHIR **JSON** for R4, R4B and R5 — there is no
  JVM round-trip, so they stay fast across large directories.
- **XML resources are skipped** with a notice; convert to JSON to lint them.
- A malformed or unsupported expression is reported when rules are loaded, and
  by `fhirlint config check` — never silently ignored.

## Suppressing a rule

Because each finding carries the message ID `rule:<id>`, rules suppress like any
other issue:

```bash
fhirlint validate patient.json --suppress messageId:rule:patient-needs-mrn
```

```yaml
suppress:
  - messageId: rule:patient-needs-mrn
    reason: "MRN not yet assigned in staging data"
```

## Supported FHIRPath subset

fhirlint ships a focused, in-process FHIRPath engine. It covers the constructs
lint rules realistically need; anything outside the subset makes the rule fail
to compile (rather than evaluate incorrectly).

**Navigation**
- Path steps with an optional leading resourceType filter: `Patient.name.given`
- Indexers: `name[0].given`

**Functions**
- Collection: `exists([criteria])`, `empty()`, `where(criteria)`, `all(criteria)`,
  `count()`, `first()`, `last()`
- Boolean: `not()`, `hasValue()`
- String: `startsWith(s)`, `endsWith(s)`, `contains(s)`, `matches(regex)`,
  `length()`, `toString()`

**Operators**
- Equality: `=`, `!=`
- Comparison: `<`, `>`, `<=`, `>=`
- Logic: `and`, `or`, `xor`, `implies`
- Membership: `in`

**Literals**
- Boolean (`true`, `false`), single-quoted strings (`'value'`), numbers, and `$this`

### Not supported

Arithmetic (`+`, `-`, `*`, `/`, `div`, `mod`, `&`), union (`|`), the `~`/`!~`
operators, date/quantity literals, and any function not listed above. A rule
using these is rejected at load time with a clear error, so you learn about it
before validation runs — not through a silently wrong result.

## Examples

```yaml
rules:
  # Require a specific identifier system
  - id: patient-needs-mrn
    resource: Patient
    assert: "identifier.where(system='http://hospital.example/mrn').exists()"
    message: "Patient is missing an MRN identifier"

  # Ban placeholder references
  - id: no-example-refs
    assert: "descendants().reference.exists() implies reference.startsWith('http://example.org').not()"
    severity: warning

  # Enforce a coding system on Observation.code
  - id: obs-loinc
    resource: Observation
    assert: "code.coding.where(system='http://loinc.org').exists()"

  # Require at least one official name
  - id: official-name
    resource: Patient
    assert: "name.where(use='official').exists()"
    severity: warning

  # Constrain a value with a comparison
  - id: mrn-length
    resource: Patient
    assert: "identifier.where(system='http://hospital.example/mrn').value.length() >= 5"
```
