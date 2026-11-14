# Analysing datasets and profiles

Dataset statistics, profile coverage, comparing runs and profiles, and the Computer System Validation report.

## Dataset statistics

`fhirlint stats` gives a quick structural overview of a dataset — how many resources of each type, which profiles they declare, and an aggregate validation summary. Handy for CI health dashboards and spot-checking a FHIR export.

```bash
fhirlint stats ./fhir/
```

```text
Resource types
  Observation        34
  MedicationRequest  15
  Patient            12
  Medication          8

Profiles declared
  https://fhir.kbv.de/StructureDefinition/KBV_PR_Base_Medication  8
  (none)                                                          61

Validation summary
  Files  69   Valid  64 (93%)   Warnings  18   Errors  3
```

Resource-type and profile counts are gathered **offline** by parsing each file (`.json`, `.jsonl`, `.ndjson`, `.xml`); each line of a line-delimited export counts as a resource. The validation summary runs the validator — skip it for an instant, offline overview:

```bash
fhirlint stats ./fhir/ --no-validate

# Machine-readable, for dashboards or badge generators
fhirlint stats ./fhir/ --format json --output stats.json
```

`stats` is informational and always exits `0` on success (it never fails the build); use `validate` for CI gating. `--exclude` patterns and `.fhirlintignore` are respected, just like `validate`.

## Profile coverage

`fhirlint coverage` measures how much of a profile a set of resources actually exercises, with an emphasis on the `mustSupport` elements.

The validator cannot answer this. It sees one resource at a time and only checks what is present, so it has no way to notice that across your whole example set nobody has ever populated `Patient.identifier:VersichertenId-GKV`. An IG can ship thirty green examples and still leave half its must-support elements untouched.

```bash
fhirlint coverage ./examples/ --ig de.gematik.isik-basismodul#4.0.3 --profile ISiKPatient
```

```text
ISiKPatient  (Patient)
  resources:      9  (1 matched by resource type, not by meta.profile)
  must-support:   52/64  (81%)
  never populated:
    Patient.identifier:VersichertenId-GKV
    Patient.identifier:VersichertenId-GKV.type
    Patient.name:Geburtsname.family.extension:namenszusatz
    Patient.gender.extension:Geschlecht-Administrativ
    … and 8 more (use --verbose for the full list)

61 resource(s) scanned, 52 not attributed to any profile
```

Profiles come from IG packages in the local FHIR package cache (`~/.fhir/packages`). A package that is not cached yet is downloaded from the FHIR package registries — `packages2.fhir.org` first, `packages.fhir.org` as fallback, the validator's own order — and verified against the checksum the registry publishes for it. `--offline` forbids that and fails instead, for hermetic builds:

```bash
fhirlint coverage ./examples/ --ig kbv.basis#1.9.0 --offline
```

The checksum is the registry's own `dist.shasum`. It catches a truncated or tampered transfer; it is not a defence against the registry itself, since the digest and the archive come from the same place. A mismatch fails the install. A checksum that cannot be fetched at all is a visible warning rather than a failure, matching how the validator JAR is handled — but it is never silent. (The JAR itself gets a stronger check where upstream allows one: see [SECURITY.md](../SECURITY.md#properties-you-can-rely-on).)

### How resources are attributed

By `meta.profile`. A resource that declares the profile is measured against it.

Naming profiles with `--profile` additionally allows resources of the matching resource type that declare *no* profile to be measured. Without `--profile`, that is deliberately off: attributing a bare `Patient` to all thirty Patient profiles in an IG produces noise, not information. Resources measured this way are counted separately in the output, because coverage established that way describes your dataset rather than conformance to the profile.

A profile that no resource was attributed to is left out entirely rather than listed at 0%. Nothing was measured for it, and a row reading "0%" would be a verdict on data that does not exist.

### What "not measurable" means

Slices are how German profiles carry most of their must-support elements — 54 of 64 for ISiKPatient — so coverage resolves slice membership rather than skipping it. A slice is matched by the `pattern[x]` or `fixed[x]` its definition declares, by an extension's URL, or by a discriminator pointing at a child element that fixes a value. When the definition lives in another package — a `HumanName` constrained through `humanname-de-basis`, say — the lookup follows it there, which is why supporting packages in your cache are loaded alongside the ones you name.

Some slices cannot be decided this way. The common case is a slice identified only by a value set binding: deciding membership would mean expanding the value set, which is terminology work, and coverage runs entirely offline. Those elements are reported as **not measurable** and excluded from the denominator:

```text
  not measurable: 8 element(s)
      slice is identified by a value set binding (http://hl7.org/fhir/ValueSet/organization-type), which coverage does not expand
```

They count as neither covered nor uncovered. Counting them as misses would report a limit of the tool as a gap in your data.

The other common cause is a slice that is perfectly well defined, just not in a package you have. German profiles delegate datatype constraints — a `HumanName` through `humanname-de-basis` — and the report names the profile that is missing so you can add it:

```text
  not measurable: 12 element(s)
      slice is defined in http://fhir.de/StructureDefinition/humanname-de-basis, which is not loaded — install the IG package that provides it
```

Adding `--ig de.basisprofil.r4#1.5.4` to that run takes it from 43 of 52 elements measured to all 64.

The same principle applies to a profile whose base is not in the cache: the must-support list is then a lower bound, and the report says so rather than presenting it as complete.

### In CI

`--min-coverage` exits non-zero when any profile falls below a threshold, so a new must-support element without an example fails the build:

```bash
fhirlint coverage ./examples/ --ig kbv.basis#1.9.0 --min-coverage 80
```

`--format json` gives the full per-element detail, including which elements were unresolved and why:

```bash
fhirlint coverage ./examples/ --ig kbv.basis#1.9.0 -f json | jq '.profiles[] | {name, percent, missing: [.elements[] | select(.populated == false and .unresolved != true) | .id]}'
```

### Limits

- JSON and NDJSON input only. XML resources are reported as skipped rather than silently ignored.
- Analysis is offline: no terminology server is contacted and no value set is expanded. Only fetching a package that is not cached touches the network, and `--offline` turns that off too.
- `type`, `profile` and `exists` slice discriminators are not derived. Across every package examined they are fewer than 1 in 300; a choice element sliced by type is already resolved through the field name in the instance.
- Coverage measures whether an element was ever populated by at least one resource, not how often. An element populated by one example out of thirty counts as covered.

## Comparing runs (change control)

`fhirlint diff` compares two JSON reports and categorises every issue as new, resolved, or unchanged. In regulated environments (MDR, ISO 13485, IEC 62304) it produces machine-generated evidence that a change introduces no new validation issues — attach the output to a change request or store it in the Design History File.

```bash
# Produce a report on each side, then compare
fhirlint validate ./fhir/ --format json --output baseline.json   # e.g. on main
fhirlint validate ./fhir/ --format json --output current.json    # on the change

fhirlint diff baseline.json current.json
```

```text
New issues (1)
  ✗ ERROR medication-request-003.json  dom-4 @ MedicationRequest

Resolved issues (2)
  ✓      medication-001.json  dom-6 @ Medication
  ✓      observation-012.json  TERMINOLOGY_TX_WARNING @ Observation

Unchanged issues (14)
  (use --show-unchanged to list)

Summary: 1 new · 2 resolved · 14 unchanged
```

Issues are matched by file, message ID, and location (line/column shifts are ignored), so reformatting alone is never reported as a change.

Exit codes make it CI-ready: `0` no new issues, `1` new issues found (breaks the build), `2` fhirlint itself failed (e.g. malformed input).

`--format json` emits the structured diff; `--format sarif` emits **only the new issues**, so uploading it to GitHub Code Scanning annotates a pull request with just its regressions, free of pre-existing noise.

When the two reports name different validators, the diff says so before anything else — `note: the reports were produced by different validators — baseline 6.10.3, current 6.10.4` — because a changed validator explains more diffs than changed resources do. The JSON diff carries both sides' `meta` as `baseline` and `current`. Reports written before fhirlint recorded this compare silently, as before.

## Comparing profiles

`fhirlint compare` diffs two **StructureDefinitions (profiles)** and reports how their constraints differ. This is *profile* diffing — distinct from `fhirlint diff` above, which compares two *validation reports* at the instance level:

| Command | Compares | Answers |
| --- | --- | --- |
| `fhirlint diff` | two validation **reports** (`validate --format json`) | "did this change introduce new validation issues?" |
| `fhirlint compare` | two **profiles** (StructureDefinitions) | "how do these two profiles' constraints differ?" |

It surfaces the validator's built-in `ComparisonService`. Each side is a local StructureDefinition file or a profile from an IG package; package aliases (e.g. `kbv-basis`) resolve just like `validate --profile`.

```bash
# Two versions of the same profile — what might break on upgrade?
fhirlint compare \
  --left  kbv.basis#1.4.0 --left-profile  KBV_PR_Base_Patient \
  --right kbv.basis#1.5.0 --right-profile KBV_PR_Base_Patient

# Local house profile vs. published base profile
fhirlint compare \
  --left  ./profiles/our-patient.json \
  --right kbv.basis#1.5.0 --right-profile KBV_PR_Base_Patient

# Two local files
fhirlint compare --left ./a.json --right ./b.json
```

For a local file the canonical URL is read from the file; for a package, name the profile with `--left-profile` / `--right-profile` (its canonical URL or id). Use `--ig` to load extra packages needed for resolution.

```text
Comparing http://example.org/.../KBV_PR_Base_Patient → http://example.org/.../KBV_PR_Base_Patient
  3 difference(s)

  ✗ StructureDefinition.version  Values for version differ: '1.4.0' vs '1.5.0'
  ~ Patient.birthDate            Element minimum cardinalities differ: '0' vs '1'
  ~ Patient.name                 Added constraint: kbv-1
```

`--format json` emits the structured difference list (severity, path, message) for CI gating; `--format html` writes the validator's full side-by-side comparison site (to `--output`, default `./fhirlint-compare`), suitable for attaching to migration notes or a Design History File.

Exit codes mirror `diff`: `0` no differences, `1` differences found (breaks the build), `2` fhirlint itself failed (e.g. an unresolved profile). Terminology is disabled, so comparison is fast and offline. First run needs Java 17+ and the validator JAR (the same one `validate` downloads).

## Computer System Validation (qualify)

Medical-device teams running fhirlint in a validated process must perform Computer System Validation (CSV) before production use. `fhirlint qualify` runs the tool against a built-in set of known-valid and known-invalid FHIR resources and produces a formal **Operational Qualification (OQ)** report — documented evidence that fhirlint correctly accepts valid resources and rejects invalid ones.

```bash
fhirlint qualify
```

```bash
fhirlint Computer System Validation — Operational Qualification
  Tool version:  v1.0.0
  JAR version:   6.9.10
  JAR SHA256:    aba1fe09...
  FHIR version:  4.0.1
  Terminology:   offline
  Timestamp:     2026-05-24T16:03:10Z

Test cases: 7 passed · 0 failed

  PASS  invalid/patient-bad-gender.json    → error detected (expected)
  PASS  valid/patient-complete.json        → accepted, no errors (expected)
  ...

Result: QUALIFIED ✓
```

It exits `0` when qualified, `1` when any case fails, and `2` on a tool error. Validation runs **offline** by default (no terminology server) so results are reproducible; pass `--terminology-server <url>` to validate against a live server.

The report records everything needed for traceability — tool version, validator JAR version and SHA256, FHIR version, timestamp, and per-case results:

```bash
# HTML for a Design History File (print to PDF from any browser for QMS upload)
fhirlint qualify --format html --output qualification-report.html

# JSON for automated CSV pipelines
fhirlint qualify --format json --output qualification-report.json
```

### Custom test suites

Supply your own cases alongside the built-in ones. Each FHIR file needs a companion `<name>.expected.json`:

```bash
fhirlint qualify --test-suite ./qualification/test-cases/
```

```jsonc
// patient-no-gender.expected.json
{
  "description": "Patient without the required gender element",
  "valid": false,                    // false = the tool must report an error
  "messageIds": ["dom-6"]            // optional: these message IDs must appear
}
```

> **Regulatory context:** ISO 13485 §7.6 (verification of monitoring equipment), IEC 62304 §8.1 (validation of software tools used in development), FDA 21 CFR Part 11 (validation of computerised systems), GAMP 5 Category 4 (IQ/OQ/PQ documentation).
