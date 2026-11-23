# Style & naming rules

Built-in lint rules check **authoring conventions** that profile validation does
not enforce — naming style and canonical-URL patterns. They complement the FHIR
validator: the validator tells you a resource is *conformant*, these rules tell
you it follows your project's *conventions*.

Rules are **opt-in** and configured under the `lint:` key in `fhirlint.yml`. Each
rule carries its own severity. Findings use the message ID `lint:<rule>` and
flow through every reporter, the `--severity`/`--fail-on` gates, suppression, and
baseline — exactly like validator issues.

```yaml
lint:
  id-kebab-case: warning
  profile-name-pascalcase: warning
  canonical-url-pattern:
    severity: error
    base: "https://example.org/fhir/"
```

- A rule mapped to a bare severity (`id-kebab-case: warning`) is enabled at that
  severity.
- A rule that takes parameters uses the map form with a `severity` and the
  parameter keys.
- Allowed severities: `error`, `warning`, `information`. Only `error` fails the
  build (subject to `--fail-on`).
- Rules run in-process against FHIR **JSON** (R4/R4B/R5). XML resources are
  skipped with a notice.
- `fhirlint config check` validates the `lint:` section: unknown rule names,
  invalid severities, and missing required parameters are reported with line
  numbers.

## Built-in rules

| Rule | Default severity | Parameters | Checks |
|------|------------------|------------|--------|
| `id-kebab-case` | `warning` | — | `Resource.id` is lowercase, hyphen-separated (`[a-z0-9]` words joined by `-`). |
| `profile-name-pascalcase` | `warning` | — | `StructureDefinition.name` starts uppercase, then has letters, digits and underscores only: HL7's own name invariant (`sdf-0` in R4, `cnl-0` in R5). See [Underscores in profile names](#underscores-in-profile-names). |
| `canonical-url-pattern` | `error` | `base` (required) | A resource's canonical `url` starts with the configured `base`. `Device` and `DeviceDefinition` are skipped: their `url` is a network address, not a canonical. |

Rules only fire when the relevant element is present: a resource without an
`id`, a non-`StructureDefinition` resource, or a resource without a `url` is not
flagged by the respective rule.

### Why not just rely on the validator?

The FHIR validator rejects an *invalid* id (e.g. one containing an underscore or
exceeding 64 characters). It does **not** object to `ExamplePatient1` — a valid
FHIR id that is not lowercase-hyphenated. Convention rules cover that gap.

### Underscores in profile names

`profile-name-pascalcase` allows underscores (#594). HL7's invariant allows
them, so a name with one is usable as an identifier in generated code, which
is what the invariant asks for. And the German profiles use them throughout. Of
the StructureDefinition names in a local package cache on 2026-11-23, only
underscores stood between these and strict PascalCase:

| Packages | Names | With underscores |
|---|---|---|
| KBV | 143 | 143 (`KBV_PR_Base_Patient`) |
| MII | 420 | 412 (`MII_PR_Labor_Laboranforderung`) |
| gematik | 973 | 272 |
| de.basisprofil | 302 | 40 (`VitalSignDE_Blutdruck`) |

A rule that rejected underscores would flag nearly every profile in a KBV or
MII project, and say nothing about whether the names work as identifiers.
What it still reports is what HL7 warns about: a lower-case start (`boolean`,
`myProfile`), spaces, hyphens and dots.

How the rules are checked for false positives on real-world data, and what a
new or changed rule has to go through, is in [corpus-scan.md](corpus-scan.md).

## Suppressing a rule finding

Because each finding carries `lint:<rule>` as its message ID, suppress it like
any other issue:

```bash
fhirlint validate patient.json --suppress messageId:lint:id-kebab-case
```

```yaml
suppress:
  - messageId: lint:id-kebab-case
    reason: "Legacy ids kept for referential stability"
```

## Related

For **project-specific** assertions that go beyond these fixed conventions —
"every Patient must carry an MRN identifier", "no reference may point at
example.org" — use [custom FHIRPath rules](rules.md) (`rules:` / `--rules-file`).

## Deferred rules

The following conventions were considered but intentionally left out of the
initial rule set because they are prone to false positives and need more design:

- **element / sliceName camelCase** — acronym slice names (`MRN`, `NHS`) are
  legitimately not camelCase.
- **file-name matches id** — the input is often a temp file (stdin, URL, NDJSON
  split, Bundle entry expansion), so the on-disk name is not the authored one.

They can be added to the registry later without changing the config surface.
