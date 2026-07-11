<p align="center">
  <img src="assets/logo.png" alt="fhirlint" width="180"/>
</p>

# fhirlint

[![Tests](https://github.com/fhirlint/fhirlint/actions/workflows/test.yml/badge.svg)](https://github.com/fhirlint/fhirlint/actions/workflows/test.yml)
[![Lint](https://github.com/fhirlint/fhirlint/actions/workflows/lint.yml/badge.svg)](https://github.com/fhirlint/fhirlint/actions/workflows/lint.yml)
[![Security](https://github.com/fhirlint/fhirlint/actions/workflows/security.yml/badge.svg)](https://github.com/fhirlint/fhirlint/actions/workflows/security.yml)

A lightweight CLI for validating FHIR resources with a developer-friendly experience.

`fhirlint` wraps the official [HL7 FHIR Validator](https://github.com/hapifhir/org.hl7.fhir.core) and adds what it lacks: clean terminal output, multiple input sources, JSON, HTML, JUnit, SARIF, Markdown, and CodeClimate reports, pipeline-ready exit codes, watch mode, suppression rules, and built-in aliases for German FHIR profiles (KBV, MII, DiGA).

The validator JAR is downloaded automatically on first use — no manual setup required.

**Requirements:** Java 17+

---

## Table of contents

- [Installation](#installation)
- [Input sources](#input-sources)
- [Profiles & implementation guides](#profiles--implementation-guides)
- [Output formats](#output-formats)
- [Dataset statistics](#dataset-statistics)
- [Preprocessing](#preprocessing)
- [Suppressing known issues](#suppressing-known-issues)
- [Custom lint rules](#custom-lint-rules)
- [Explaining message IDs](#explaining-message-ids)
- [Evaluating FHIRPath expressions](#evaluating-fhirpath-expressions)
- [Baseline mode](#baseline-mode)
- [Comparing runs (change control)](#comparing-runs-change-control)
- [Comparing profiles](#comparing-profiles)
- [Computer System Validation (qualify)](#computer-system-validation-qualify)
- [Terminology server](#terminology-server)
- [Watch mode](#watch-mode)
- [Pipeline integration](#pipeline-integration)
- [Configuration file](#configuration-file-fhirlintymll)
- [Validating the configuration file](#validating-the-configuration-file)
- [Configuration reference](#configuration-reference)
- [Built-in profile aliases](#built-in-profile-aliases)
- [JAR management](#jar-management)
- [Go library](#go-library)
- [Guides](#guides)
- [Contributing](#contributing)

---

## Installation

### Pre-built binary (recommended)

Download the latest binary for your platform from the [releases page](https://github.com/fhirlint/fhirlint/releases):

```bash
# Linux (amd64)
gh release download --repo fhirlint/fhirlint --pattern "*linux_amd64.tar.gz"
tar xzf fhirlint_*_linux_amd64.tar.gz && sudo mv fhirlint /usr/local/bin/

# macOS (Apple Silicon)
gh release download --repo fhirlint/fhirlint --pattern "*darwin_arm64.tar.gz"
tar xzf fhirlint_*_darwin_arm64.tar.gz && sudo mv fhirlint /usr/local/bin/

# macOS (Intel)
gh release download --repo fhirlint/fhirlint --pattern "*darwin_amd64.tar.gz"
tar xzf fhirlint_*_darwin_amd64.tar.gz && sudo mv fhirlint /usr/local/bin/
```

Windows: download the `.zip` from the [releases page](https://github.com/fhirlint/fhirlint/releases) and add `fhirlint.exe` to your `PATH`.

### Homebrew (macOS)

```bash
brew install fhirlint/tap/fhirlint
```

This installs a JRE (`openjdk@17`) as a dependency, so no separate Java setup is needed. Upgrade with `brew upgrade fhirlint`.

### go install

```bash
go install github.com/fhirlint/fhirlint@latest
```

### Docker

```bash
# Validate files in the current directory
docker run --rm -v $(pwd):/work ghcr.io/fhirlint/fhirlint validate /work/fhir/

# Pin to a specific version
docker run --rm -v $(pwd):/work ghcr.io/fhirlint/fhirlint:1.0.0 validate /work/fhir/
```

The image includes a JRE — no separate Java installation required.

### GitHub Actions

Use the action to validate FHIR resources in one step — it installs fhirlint and runs `validate`:

```yaml
- uses: fhirlint/fhirlint@v1.3.0
  with:
    path: ./fhir/
    fail-on: error
    severity: warning
```

Supported inputs: `path`, `url`, `profile`, `ig`, `severity`, `fail-on`, `format`, `output`, and `version` (a release tag or `latest`). `profile` and `ig` accept multiple values, one per line. The action runs on Linux runners (Java 17+ is preinstalled on `ubuntu-latest`).

<details>
<summary>Manual install (any runner / OS)</summary>

```yaml
- name: Install fhirlint
  env:
    GH_TOKEN: ${{ github.token }}
  run: |
    gh release download --repo fhirlint/fhirlint --pattern "*linux_amd64.tar.gz"
    tar xzf fhirlint_*_linux_amd64.tar.gz
    sudo mv fhirlint /usr/local/bin/
    rm fhirlint_*_linux_amd64.tar.gz
```

Or use the Docker image directly in a workflow step:

```yaml
- name: Validate FHIR resources
  uses: docker://ghcr.io/fhirlint/fhirlint:latest
  with:
    args: validate /github/workspace/fhir/ --fail-on error
```

</details>

### Use as a Claude Code plugin

Validate FHIR resources from inside an agentic [Claude Code](https://claude.com/claude-code) session. The plugin is a thin wrapper around the `fhirlint` binary: Claude runs it, **interprets** the validator's findings, and fixes the offending resource in place.

```
/plugin marketplace add fhirlint/fhirlint
/plugin install fhirlint@fhirlint
```

This installs two skills:

- **validate** — runs `fhirlint validate`, explains the issues, and proposes/applies fixes to your resources.
- **audit** — runs `fhirlint audit` to check the validator JAR for updates and known security advisories.

Just ask Claude to "validate the FHIR resources in this project" and the skill activates.

**Requirements:** the plugin calls the `fhirlint` binary, so it must be on your `PATH` (install via Homebrew or `go install`, above). The first validation downloads the validator JAR (~250 MB) and needs **Java 17+** — the first run may take a minute and is not a hang.

#### Auto-validation hook (opt-in)

The plugin also ships a `PostToolUse` hook that validates a FHIR resource **automatically right after Claude edits it** and feeds any errors back into the session, so the model can fix the resource it just touched without you asking.

It is **opt-in** — installing the plugin does not change your editing behaviour. Enable it by setting an environment variable:

```bash
export FHIRLINT_AUTOVALIDATE=1   # in your shell profile
```

or per-project in `.claude/settings.json`:

```json
{
  "env": { "FHIRLINT_AUTOVALIDATE": "1" }
}
```

Unset the variable (or set it to `0`) to disable it again.

The hook is deliberately quiet:

- It runs only on the **file just edited** (`.json`/`.xml`) and only when that file is a FHIR resource (has a `resourceType` / the FHIR namespace), so unrelated files are skipped — no validation storms.
- It surfaces **errors only**. Advisory warnings (e.g. `dom-6`) would nag on every edit; use the **validate** skill or `fhirlint validate` to see those.
- It honours your `fhirlint.yml`, `.fhirlintignore`, and suppression rules, so its output matches the CLI.
- It **stays idle until the validator JAR is cached**, so an automatic edit never triggers a surprise ~250 MB download. Run `fhirlint validate` once (or let the validate skill run) to prime it. (`jq` is also required for the hook.)

#### Team rollout

Commit the marketplace and plugin to your repo's `.claude/settings.json` so the whole team gets it on trust:

```json
{
  "extraKnownMarketplaces": {
    "fhirlint": { "source": { "source": "github", "repo": "fhirlint/fhirlint" } }
  },
  "enabledPlugins": { "fhirlint@fhirlint": true },
  "env": { "FHIRLINT_AUTOVALIDATE": "1" }
}
```

Drop the `env` line to ship the skills without the auto-validation hook.

---

## Input sources

```bash
# Single file
fhirlint validate patient.json

# Directory — all .json, .xml, and .ndjson files, single JVM invocation
fhirlint validate ./fhir/resources/

# Stdin
cat patient.json | fhirlint validate

# HTTP endpoint (repeatable for batch validation)
fhirlint validate --url https://my-api/fhir/Patient/123
fhirlint validate \
  --url https://my-api/fhir/Patient/123 \
  --url https://my-api/fhir/Medication/abc \
  --url https://my-api/fhir/MedicationRequest/xyz

# FHIR Bulk Data export (.ndjson) — each line validated as a separate resource
fhirlint validate export-Patient.ndjson

# Extract each element of a JSON array and validate separately
fhirlint validate api-response.json --extract-each "$.medications"

# Also validate each entry.resource inside a Bundle as a standalone resource
fhirlint validate bundle.json --bundle-entries
fhirlint validate ./fhir/ --bundle-entries
```

When validating multiple resources (directory, multiple `--url`, `--extract-each`, or NDJSON), all resources are processed in a **single JVM invocation** to avoid repeated startup overhead.

---

## Profiles & implementation guides

```bash
# Built-in alias (see list below)
fhirlint validate patient.json --profile kbv-patient

# Full profile URL
fhirlint validate patient.json --profile https://fhir.kbv.de/StructureDefinition/KBV_PR_Base_Patient

# IG package (downloaded from the FHIR package registry)
fhirlint validate patient.json --ig kbv.basis#1.5.0

# Multiple profiles and IGs
fhirlint validate bundle.json --profile mii --ig custom.ig#1.0.0

# Local CodeSystem or ValueSet file (no full IG package needed)
fhirlint validate prescription.json --codesystem ./codesystems/internal-drugs.json
fhirlint validate prescription.json \
  --codesystem ./codesystems/internal-drugs.json \
  --valueset ./valuesets/internal-drug-vs.json
```

`--codesystem` and `--valueset` bundle the given files into a minimal temporary IG package so the validator can resolve codes without requiring a published `.tgz`.

---

## Output formats

```bash
# Colored terminal output (default)
fhirlint validate patient.json

# JSON report to stdout
fhirlint validate patient.json --format json

# HTML report to file
fhirlint validate patient.json --format html --output report.html

# JUnit XML report (for GitHub Actions, Jenkins, Azure DevOps test dashboards)
fhirlint validate ./fhir/ --format junit --output results.xml

# SARIF report (for GitHub Code Scanning / security dashboard)
fhirlint validate ./fhir/ --format sarif --output results.sarif

# Markdown summary (for posting as a GitHub PR comment)
fhirlint validate ./fhir/ --format markdown --output report.md

# CodeClimate report (for GitLab Code Quality merge-request widget)
fhirlint validate ./fhir/ --format codeclimate --output gl-code-quality.json

# Multiple formats in one run
fhirlint validate patient.json --format terminal --format json --output results.json

# Filter by minimum severity
fhirlint validate patient.json --severity warning   # hide information
fhirlint validate patient.json --severity error     # errors only

# Suppress output for valid files (only show files with issues)
fhirlint validate ./fhir/ --quiet

# Disable ANSI colors (useful for CI environments without color support)
fhirlint validate patient.json --no-color
```

---

## Dataset statistics

`fhirlint stats` gives a quick structural overview of a dataset — how many resources of each type, which profiles they declare, and an aggregate validation summary. Handy for CI health dashboards and spot-checking a FHIR export.

```bash
fhirlint stats ./fhir/
```

```
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

Resource-type and profile counts are gathered **offline** by parsing each file (`.json`, `.ndjson`, `.xml`); each NDJSON line counts as a resource. The validation summary runs the validator — skip it for an instant, offline overview:

```bash
fhirlint stats ./fhir/ --no-validate

# Machine-readable, for dashboards or badge generators
fhirlint stats ./fhir/ --format json --output stats.json
```

`stats` is informational and always exits `0` on success (it never fails the build); use `validate` for CI gating. `--exclude` patterns and `.fhirlintignore` are respected, just like `validate`.

---

## Preprocessing

```bash
# Extract a nested FHIR resource before validating (e.g. from an API wrapper)
fhirlint validate api-response.json --extract "$.data.fhir"
fhirlint validate --url https://my-api/patient --extract "$.entry[0].resource"

# Extract each element of a JSON array as a separate FHIR resource
fhirlint validate api-response.json --extract-each "$.medications"

# Remove fields before validating (repeatable)
fhirlint validate patient.json --ignore "$.meta.tag" --ignore "$.text"
```

`--extract` and `--extract-each` are mutually exclusive. `--extract-each` labels each result as `filename[0] (ResourceType/id)` for easy identification.

Both `--extract` and `--ignore` also work on XML input using the same `$.element.child` path syntax. For `--extract` on XML, the path must point to the actual FHIR resource element (e.g., `$.entry.resource.Patient`) since XML namespaces are injected automatically. See [Validating partial JSON](docs/extract.md) for details.

---

## Suppressing known issues

When a deviation from the FHIR spec is intentional and accepted, use `--suppress` to silence the issue without disabling all failure detection:

```bash
# Suppress by HL7 message ID (most precise)
fhirlint validate patient.json --suppress messageId:dom-6

# Suppress by FHIR constraint ID
fhirlint validate patient.json --suppress constraint:dom-6

# Suppress all issues on a specific field path
fhirlint validate prescription.json --suppress expression:MedicationRequest.intent

# Show suppressed issues with a muted label instead of hiding them
fhirlint validate patient.json --suppress messageId:dom-6 --show-suppressed
```

Suppressed issues are:
- Excluded from the exit-code calculation (won't trigger `--fail-on error`)
- Hidden from terminal output by default; shown with `↷ SUPP` when `--show-suppressed` is set
- Included in a separate `suppressed` array in JSON/HTML output for auditability

A rule that matches nothing emits a warning (`suppress rule "X" matched 0 issues`) to catch stale suppressions.

Suppression rules can also be committed to `fhirlint.yml` to make accepted deviations explicit and reviewable:

```yaml
suppress:
  - constraint: dom-6
  - expression: MedicationRequest.intent
  - expression: Patient.text
    severity: warning   # only suppress warnings on this field
```

---

## Custom lint rules

Profile validation checks conformance to a StructureDefinition, but it cannot express **project-specific** expectations such as "every `Patient` must carry an MRN identifier". Custom lint rules fill that gap: each rule asserts a [FHIRPath](https://hl7.org/fhirpath/) expression over every validated resource, and a resource that fails the assertion produces a finding that flows through the normal reporters, severity filter, suppression, and baseline machinery.

Define rules under a `rules:` key in `fhirlint.yml`, or in a separate file passed with `--rules-file`:

```yaml
rules:
  - id: patient-needs-mrn
    resource: Patient        # optional: only applies to this resourceType
    assert: "identifier.where(system='http://hospital.example/mrn').exists()"
    message: "Patient is missing an MRN identifier"
    severity: error          # error | warning | information (default: error)

  - id: no-example-refs
    assert: "descendants().reference.exists() implies reference.startsWith('http://example.org').not()"
    severity: warning
```

```bash
# Rules from fhirlint.yml are applied automatically
fhirlint validate ./fhir/

# Or load them from a dedicated file
fhirlint validate ./fhir/ --rules-file lint-rules.yml
```

Each rule failure is reported with the message ID `rule:<id>`, so it can be suppressed like any other issue:

```bash
fhirlint validate patient.json --suppress messageId:rule:patient-needs-mrn
```

Rules are evaluated in-process against FHIR **JSON** (R4/R4B/R5) — no JVM round-trip — so they add negligible overhead even across large directories. XML resources are skipped with a notice. A malformed or unsupported expression is reported when rules are loaded (and by `fhirlint config check`), not silently ignored.

See the [Custom lint rules guide](docs/rules.md) for the full list of supported FHIRPath functions and operators.

---

## Explaining message IDs

Validation output references message IDs like `dom-6` or `ele-1` without saying what they mean. `fhirlint explain` looks one up — what the rule is, where it comes from, and how to fix it — fully offline, no JAR required.

```bash
fhirlint explain dom-6
```

```
dom-6 — A resource should have narrative for robust management
Defined in: FHIR R4 Core (best-practice invariant on DomainResource)

  Every DomainResource should contain a human-readable narrative in the
  `text` field ...

How to fix:
  Add a `text` field to your resource:
  ...

Suppress if intentional:
  fhirlint validate ... --suppress messageId:dom-6
```

When a message ID has a built-in explanation, terminal output appends a hint so you know help is one command away:

```
  ⚠ WARN   A resource should have narrative for robust management
           @ Patient
           ↳ Run: fhirlint explain dom-6
```

List every ID with a built-in explanation:

```bash
fhirlint explain --list
```

fhirlint ships explanations for common FHIR core invariants (`dom-*`, `ele-1`, `ext-1`, `bdl-*`, `obs-*`); the set grows over time. Unknown IDs exit non-zero with a pointer to the [HL7 FHIR specification](https://hl7.org/fhir/).

---

## Evaluating FHIRPath expressions

FHIRPath is the expression language behind FHIR invariants/constraints (`dom-6`, `ele-1`, …), slicing discriminators, and search parameters. When an invariant fails and it's unclear *why*, `fhirlint fhirpath` evaluates an expression — or a sub-expression of the failing rule — against your real resource so you can see exactly what it returns.

```bash
# Navigate a resource
fhirlint fhirpath "Patient.name.given" patient.json

# From stdin (JSON and XML auto-detected, like validate)
cat patient.json | fhirlint fhirpath "Observation.value.exists()"

# Test a boolean invariant
fhirlint fhirpath "contact.all(name or telecom or address or organization)" patient.json

# Filtered navigation (e.g. a slicing discriminator)
fhirlint fhirpath "identifier.where(system='http://fhir.de/sid/gkv/kvid-10')" patient.json
```

A single value is printed plainly, multiple items are indexed, and an empty result set is shown as `(empty)` so "no match" is distinguishable from an error:

```
[0] Erika
[1] Maria
```

Machine-readable output for scripting composes in pipelines:

```bash
fhirlint fhirpath "Patient.name.given" patient.json --format json
```

```json
{
  "expression": "Patient.name.given",
  "result": ["Erika", "Maria"]
}
```

Evaluation is an **inspection aid, not a pass/fail gate** — an empty or `false` result still exits `0`. Only a malformed expression, an unparseable resource, or a tool failure exits `2`. `--fhir-version` (default `4.0.1`) sets the context version. Terminology is disabled for speed and offline use, so expressions that need a terminology server (e.g. `memberOf`) aren't supported. First run needs Java 17+ and the validator JAR (the same one `validate` downloads).

---

## Baseline mode

When adopting fhirlint on an existing codebase, there may be many pre-existing issues that you can't fix all at once. Baseline mode lets you capture the current state and only fail the build on new regressions.

```bash
# Generate a baseline from current issues (the build always succeeds here)
fhirlint validate ./fhir/ --generate-baseline fhirlint-baseline.json

# Use the baseline: only new issues (regressions) fail the build
fhirlint validate ./fhir/ --baseline fhirlint-baseline.json

# Regenerate when the codebase intentionally changes
fhirlint validate ./fhir/ --generate-baseline fhirlint-baseline.json
```

Issues recorded in the baseline are suppressed on subsequent runs. New issues that were not in the baseline still fail the build according to `--fail-on`. When the codebase is fixed and an issue no longer appears, fhirlint emits a warning (`warn: N baseline occurrence(s) no longer found`) — regenerate the baseline to remove the stale entries.

Commit `fhirlint-baseline.json` to version control so the suppressed issues are visible and reviewable by the team.

You can also set the baseline file in `fhirlint.yml`:

```yaml
baseline: fhirlint-baseline.json
```

**Distinction from `--suppress`:** `--suppress` is for intentional, accepted deviations (permanent exceptions). Baseline mode is for managing technical debt — issues you plan to fix eventually but can't address right now.

See [Baseline mode guide](docs/baseline.md) for a full CI workflow walkthrough.

---

## Comparing runs (change control)

`fhirlint diff` compares two JSON reports and categorises every issue as new, resolved, or unchanged. In regulated environments (MDR, ISO 13485, IEC 62304) it produces machine-generated evidence that a change introduces no new validation issues — attach the output to a change request or store it in the Design History File.

```bash
# Produce a report on each side, then compare
fhirlint validate ./fhir/ --format json --output baseline.json   # e.g. on main
fhirlint validate ./fhir/ --format json --output current.json    # on the change

fhirlint diff baseline.json current.json
```

```
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

---

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

```
Comparing http://example.org/.../KBV_PR_Base_Patient → http://example.org/.../KBV_PR_Base_Patient
  3 difference(s)

  ✗ StructureDefinition.version  Values for version differ: '1.4.0' vs '1.5.0'
  ~ Patient.birthDate            Element minimum cardinalities differ: '0' vs '1'
  ~ Patient.name                 Added constraint: kbv-1
```

`--format json` emits the structured difference list (severity, path, message) for CI gating; `--format html` writes the validator's full side-by-side comparison site (to `--output`, default `./fhirlint-compare`), suitable for attaching to migration notes or a Design History File.

Exit codes mirror `diff`: `0` no differences, `1` differences found (breaks the build), `2` fhirlint itself failed (e.g. an unresolved profile). Terminology is disabled, so comparison is fast and offline. First run needs Java 17+ and the validator JAR (the same one `validate` downloads).

---

## Computer System Validation (qualify)

Medical-device teams running fhirlint in a validated process must perform Computer System Validation (CSV) before production use. `fhirlint qualify` runs the tool against a built-in set of known-valid and known-invalid FHIR resources and produces a formal **Operational Qualification (OQ)** report — documented evidence that fhirlint correctly accepts valid resources and rejects invalid ones.

```bash
fhirlint qualify
```

```
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

---

## Terminology server

By default, fhirlint sends code lookups to `https://tx.fhir.org`. This behaviour can be tuned:

> **Note for CI and production use:** `tx.fhir.org` is a public development server — HL7 explicitly states it is not provisioned for CI pipelines or production use, has no SLA, and can go down without notice. For CI, either disable it with `--no-terminology-server` (terminology checks are skipped) or cache responses with `--tx-cache` to minimize requests. For production, run your own terminology server.

```bash
# Disable the terminology server entirely (offline / privacy-sensitive environments)
fhirlint validate patient.json --no-terminology-server

# Use a custom terminology server
fhirlint validate patient.json --terminology-server https://tx.internal.example.com

# Cache terminology responses between runs (useful in CI)
fhirlint validate patient.json --tx-cache .fhirlint-tx-cache/

# Disable terminology caching
fhirlint validate patient.json --tx-cache n/a

# Validation messages in German
fhirlint validate patient.json --locale de

# Suppress warnings about example.org placeholder URLs in test fixtures
fhirlint validate patient.json --allow-example-urls

# Control how best-practice constraints (e.g. dom-6) are handled
fhirlint validate patient.json --best-practice ignore    # silence all dom-6 warnings
fhirlint validate patient.json --best-practice error     # escalate to errors
```

**Caching the terminology responses in CI** significantly speeds up repeated runs. Add the cache directory to `actions/cache`:

```yaml
- uses: actions/cache@v4
  with:
    path: .fhirlint-tx-cache
    key: fhirlint-tx-${{ runner.os }}-${{ hashFiles('fhirlint.yml') }}

- name: Validate FHIR resources
  run: fhirlint validate ./fhir/ --tx-cache .fhirlint-tx-cache/
```

---

## Watch mode

Re-validate automatically whenever a file changes — useful during local development:

```bash
# Re-validate only changed files
fhirlint validate ./fhir/ --watch

# Re-validate all files on any change
fhirlint validate ./fhir/ --watch=all

# Custom polling interval (milliseconds)
fhirlint validate ./fhir/ --watch --watch-interval 500
```

Watch mode streams the validator output directly to the terminal. Press `Ctrl-C` to stop. It is not compatible with `--format json --output` or `--url`.

---

## Pipeline integration

```bash
# Exit non-zero if any errors are found (default)
fhirlint validate patient.json --fail-on error

# Exit non-zero for warnings too
fhirlint validate patient.json --fail-on warning

# Never fail on validation issues (reporting-only runs)
fhirlint validate patient.json --fail-on never
```

### IG lock file

`--lock` writes a `fhirlint.lock` file containing SHA256 hashes of all resolved IG packages. On subsequent runs (without `--lock`), fhirlint verifies that cached packages match the recorded hashes, ensuring reproducible builds.

```bash
# Generate or update the lock file
fhirlint validate ./fhir/ --lock

# Subsequent runs verify the lock automatically (no flag needed)
fhirlint validate ./fhir/
```

Commit `fhirlint.lock` to version control. This prevents silent package changes from affecting CI results.

### Result caching

`--cache` caches validation results per file content hash (keyed by content hash + FHIR version + profiles + IGs). Unchanged files are not re-validated, which significantly speeds up repeated runs in CI.

```bash
fhirlint validate ./fhir/ --cache
fhirlint validate ./fhir/ --cache --cache-dir .fhirlint-cache/
```

Cache the result directory in CI with `actions/cache`:

```yaml
- uses: actions/cache@v4
  with:
    path: .fhirlint-cache
    key: fhirlint-results-${{ runner.os }}-${{ hashFiles('fhirlint.yml', 'fhir/**') }}
    restore-keys: fhirlint-results-${{ runner.os }}-

- name: Validate FHIR resources
  run: fhirlint validate ./fhir/ --cache --cache-dir .fhirlint-cache/
```

### Example GitHub Actions workflow

```yaml
- name: Validate FHIR resources
  run: |
    fhirlint validate ./fhir/ \
      --format json --output fhir-report.json \
      --tx-cache .fhirlint-tx-cache/ \
      --cache --cache-dir .fhirlint-cache/ \
      --fail-on error

- uses: actions/upload-artifact@v4
  if: always()
  with:
    name: fhir-validation-report
    path: fhir-report.json
```

---

## Configuration file (`fhirlint.yml`)

Place a `fhirlint.yml` (or `.fhirlint.yml`) in your project root to set project-level defaults. CLI flags always take precedence over config file values.

```yaml
# fhirlint.yml
fhir-version: 4.0.1
severity: warning
fail-on: error

profile:
  - kbv-basis

ig:
  - kbv.basis#1.5.0

format:
  - terminal

suppress:
  - constraint: dom-6
  - expression: Patient.text
    severity: warning

tx-cache: .fhirlint-tx-cache/
best-practice: ignore
locale: de
allow-example-urls: true
```

A fully annotated example is provided in [`fhirlint.yml.example`](fhirlint.yml.example), including documentation for `overrides:` (per-glob IGs, profiles, severity, and suppress rules) and `profile-map:` (automatically apply profiles based on resource type or filename glob).

---

## Validating the configuration file

`fhirlint config check` validates the `fhirlint.yml` in the current directory:

```bash
fhirlint config check
```

It reports:
- Unknown keys and likely typos (with "did you mean?" suggestions)
- Wrong value types (e.g. a string where a boolean is expected)
- Invalid enum values (e.g. an unknown `severity` or `fail-on` value)
- Malformed suppress rules

Exit code `0` means the configuration is valid; exit code `1` means issues were found.

---

## Configuration reference

### All CLI flags

| Flag | Default | Description |
|------|---------|-------------|
| `--fhir-version` | `4.0.1` | FHIR version: `4.0.1`, `4.3.0`, `5.0.0` |
| `--profile`, `-p` | — | Profile alias or URL (repeatable) |
| `--ig` | — | IG package, e.g. `kbv.basis#1.5.0` (repeatable) |
| `--codesystem` | — | Local FHIR `CodeSystem` file (repeatable) |
| `--valueset` | — | Local FHIR `ValueSet` file (repeatable) |
| `--format`, `-f` | `terminal` | Output format: `terminal`, `json`, `html`, `junit`, `sarif` (repeatable) |
| `--output`, `-o` | stdout | Output file for `json`/`html` reports |
| `--severity`, `-s` | `information` | Minimum severity to display: `information`, `warning`, `error` |
| `--fail-on` | `error` | Exit non-zero when issues at this level or above are found: `error`, `warning`, `never` |
| `--max-warnings` | `-1` | Exit non-zero when warning count exceeds N (`-1` = disabled) |
| `--exclude` | — | Exclude files/dirs matching pattern (repeatable, gitignore-style) |
| `--bundle-entries` | `false` | Also validate each `entry.resource` in a FHIR Bundle separately |
| `--url` | — | Fetch and validate from an HTTP endpoint (repeatable) |
| `--url-timeout` | `30s` | Timeout for HTTP fetches via `--url` |
| `--extract` | — | JSONPath to extract from input before validating |
| `--extract-each` | — | JSONPath to an array — validates each element separately |
| `--ignore` | — | JSONPath field to remove before validating (repeatable) |
| `--suppress` | — | Silence a known issue: `messageId:X`, `constraint:X`, or `expression:X` (repeatable) |
| `--show-suppressed` | `false` | Show suppressed issues with a muted `↷ SUPP` label |
| `--baseline` | — | Baseline file — only new issues (regressions) fail the build |
| `--generate-baseline` | — | Generate a baseline file from current issues |
| `--no-terminology-server` | `false` | Disable terminology server — no data sent to `tx.fhir.org` |
| `--terminology-server` | — | Custom terminology server URL |
| `--tx-cache` | — | Terminology cache directory (`n/a` to disable) |
| `--allow-insecure-tx` | `false` | Suppress warning when terminology server uses HTTP |
| `--tx-log` | — | Write terminology request log to file |
| `--locale` | — | Locale for validation messages, e.g. `de`, `fr` |
| `--allow-example-urls` | `false` | Suppress warnings about `example.org` placeholder URLs |
| `--jurisdiction` | — | Jurisdiction for country-specific bindings, e.g. `urn:iso:std:iso:3166#DE` |
| `--display-issues-are-warnings` | `false` | Downgrade coded-display mismatch errors to warnings |
| `--po` | — | Load message translations from a `.po` file at runtime (repeatable) |
| `--best-practice` | — | Best-practice constraint handling: `ignore`, `hint`, `warning`, `error` |
| `--timeout` | `5m` | Timeout for the Java validator process |
| `--cache` | `false` | Cache validation results per file hash |
| `--cache-dir` | — | Directory for result cache (default: `~/.fhirlint/result-cache/`) |
| `--lock` | `false` | Write/update `fhirlint.lock` with IG package SHA256 hashes |
| `--quiet`, `-q` | `false` | Suppress per-file output for valid files (terminal format only) |
| `--no-color` | `false` | Disable ANSI color output |
| `--watch` | — | Watch mode: `single` (changed files only) or `all` (all files on any change) |
| `--watch-interval` | — | Polling interval for `--watch` in milliseconds |
| `--jar` | — | Path to a local validator JAR (overrides auto-download; also via `FHIRLINT_JAR`) |

### `fhirlint.yml` keys

All CLI flags have a corresponding config file key. The key is the long flag name without `--`. Repeatable flags take a YAML list; the `suppress` key accepts either strings (`messageId:dom-6`) or objects (`{messageId: dom-6, severity: warning}`).

| Key | Type | Equivalent flag |
|-----|------|-----------------|
| `fhir-version` | string | `--fhir-version` |
| `profile` | list | `--profile` |
| `ig` | list | `--ig` |
| `codesystem` | list | `--codesystem` |
| `valueset` | list | `--valueset` |
| `format` | list | `--format` |
| `output` | string | `--output` |
| `severity` | string | `--severity` |
| `fail-on` | string | `--fail-on` |
| `max-warnings` | int | `--max-warnings` |
| `exclude` | list | `--exclude` |
| `bundle-entries` | bool | `--bundle-entries` |
| `url` | list | `--url` |
| `url-timeout` | string | `--url-timeout` |
| `extract` | string | `--extract` |
| `extract-each` | string | `--extract-each` |
| `ignore` | list | `--ignore` |
| `suppress` | list | `--suppress` |
| `show-suppressed` | bool | `--show-suppressed` |
| `baseline` | string | `--baseline` |
| `no-terminology-server` | bool | `--no-terminology-server` |
| `terminology-server` | string | `--terminology-server` |
| `tx-cache` | string | `--tx-cache` |
| `allow-insecure-tx` | bool | `--allow-insecure-tx` |
| `tx-log` | string | `--tx-log` |
| `locale` | string | `--locale` |
| `allow-example-urls` | bool | `--allow-example-urls` |
| `jurisdiction` | string | `--jurisdiction` |
| `display-issues-are-warnings` | bool | `--display-issues-are-warnings` |
| `po` | list | `--po` |
| `best-practice` | string | `--best-practice` |
| `timeout` | string | `--timeout` |
| `cache` | bool | `--cache` |
| `cache-dir` | string | `--cache-dir` |
| `quiet` | bool | `--quiet` |
| `no-color` | bool | `--no-color` |
| `watch` | string | `--watch` |
| `watch-interval` | int | `--watch-interval` |

---

## Built-in profile aliases

| Alias | Resolves to |
|-------|-------------|
| `kbv-basis` | `kbv.basis#1.5.0` |
| `kbv-patient` | `kbv.basis#1.5.0` |
| `mii` | `de.medizininformatikinitiative.kerndatensatz#2024.0.0` |
| `diga` | `de.bfarm.diga#1.2.0` |

```bash
# List all available aliases
fhirlint profiles
```

---

## JAR management

`fhirlint` uses the official [HL7 FHIR Validator](https://github.com/hapifhir/org.hl7.fhir.core/releases) maintained by HAPI FHIR. The JAR (~250 MB) is downloaded automatically on first use and cached at `~/.fhirlint/validator_cli.jar`.

```bash
# Show fhirlint version and the cached JAR version
fhirlint version

# Update the cached JAR to the latest release
fhirlint update
```

To use a specific or pre-downloaded JAR:

```bash
# Via flag
fhirlint validate patient.json --jar /path/to/validator_cli.jar

# Via environment variable
export FHIRLINT_JAR=/path/to/validator_cli.jar
fhirlint validate patient.json
```

---

## Examples

- [example-fhir-medication-api](https://github.com/fhirlint/example-fhir-medication-api) — Symfony REST API serving FHIR R4 Medication and MedicationRequest resources, with fhirlint validating all fixtures in CI

---

## Go library

fhirlint can be embedded as a Go library via `pkg/fhirlint`:

```bash
go get github.com/fhirlint/fhirlint/pkg/fhirlint
```

```go
import "github.com/fhirlint/fhirlint/pkg/fhirlint"

// Validate from bytes (JSON or XML detected automatically)
result, err := fhirlint.Validate(patientJSON, fhirlint.Options{
    FHIRVersion:         "4.0.1",
    NoTerminologyServer: true,
})

// Validate a single file
result, err := fhirlint.ValidateFile("./patient.json", fhirlint.Options{
    FHIRVersion: "4.0.1",
    IGs:         []string{"kbv.basis#1.5.0"},
})

// Validate all files in a directory (single JVM invocation)
results, err := fhirlint.ValidateDir("./fhir/", fhirlint.Options{
    FHIRVersion: "4.0.1",
})

// Validate from an HTTP endpoint
result, err := fhirlint.ValidateURL("https://hapi.fhir.org/baseR4/Patient/1", fhirlint.Options{
    FHIRVersion: "4.0.1",
})
```

The library requires Java 17+ and downloads the validator JAR on first use (~250 MB). Set `JARPath` or `FHIRLINT_JAR` to provide a pre-downloaded JAR.

---

## Guides

Detailed guides for common workflows:

- [CI/CD Integration](docs/ci.md) — GitHub Actions & GitLab CI setup, JAR and terminology caching, report artifacts
- [Validating partial JSON](docs/extract.md) — `--extract` and `--extract-each` for non-FHIR API wrappers and array responses, including XML support
- [Suppression rules](docs/suppression.md) — when to suppress vs. fix, selector types, committing decisions to `fhirlint.yml`
- [Baseline mode](docs/baseline.md) — incremental adoption, managing technical debt, CI regression detection
- [German FHIR profiles](docs/german-profiles.md) — KBV, MII, DiGA: aliases, version pinning, combining profiles

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for setup instructions, the PR checklist, and how to add new flags.

## License

Apache 2.0 — see [LICENSE](LICENSE).

---

HL7® and FHIR® are registered trademarks of Health Level Seven International.
