# Using fhirlint

Input sources, profiles and implementation guides, output formats, preprocessing, explaining findings, FHIRPath, watch mode, the warm validator server and the language server.

## Input sources

```bash
# Single file
fhirlint validate patient.json

# Several files and/or directories
fhirlint validate patient.json observation.json ./fhir/resources/

# Directory — every recognised FHIR file, single JVM invocation
fhirlint validate ./fhir/resources/

# Stdin
cat patient.json | fhirlint validate

# HTTP endpoint (repeatable for batch validation)
fhirlint validate --url https://my-api/fhir/Patient/123
fhirlint validate \
  --url https://my-api/fhir/Patient/123 \
  --url https://my-api/fhir/Medication/abc \
  --url https://my-api/fhir/MedicationRequest/xyz

# Line-delimited export (.ndjson or .jsonl) — each line validated separately
fhirlint validate export-Patient.ndjson
fhirlint validate patients.jsonl

# FHIR Mapping Language — the validator builds the StructureMap and validates it
fhirlint validate transform.fml

# Extract each element of a JSON array and validate separately
fhirlint validate api-response.json --extract-each "$.medications"

# Also validate each entry.resource inside a Bundle as a standalone resource
fhirlint validate bundle.json --bundle-entries
fhirlint validate ./fhir/ --bundle-entries
```

A `--url` response is streamed to a temporary file and may be at most 512 MiB; a larger one fails the run.

When validating multiple resources (several paths, directory, multiple `--url`, `--extract-each`, or NDJSON), all resources are processed in a **single JVM invocation** to avoid repeated startup overhead. Overlapping paths are de-duplicated, so naming a file that a listed directory already covers validates it once.

### Recognised file formats

| Extension | Read as | fhirlint parses it |
|---|---|---|
| `.json` | one resource per file | yes |
| `.xml` | one resource per file | yes |
| `.ndjson`, `.jsonl` | one resource **per line** | yes |
| `.fml`, `.map` | FHIR Mapping Language | no — passed to the validator unchanged |

Anything else in a directory is ignored.

`.jsonl` is the same format as `.ndjson` under the name most data tooling produces — DuckDB, Spark, `jq`, pandas — so a dataset assembled outside a FHIR server is handled like a Bulk Data export, with one result per line.

The last column matters for the commands that read files themselves. `stats`, `coverage`, referential integrity, `--extract` and `--ignore` understand JSON and XML; a FHIR Mapping Language file is reported as skipped, with the reason, rather than counted as a broken resource. `--extract` and `--ignore` refuse it outright:

```text
Error: --extract cannot be used with FHIR Mapping Language input (.fml):
fhirlint passes it to the validator unchanged
```

### Running where `$HOME` is not the passwd home

CI runners sometimes have an unwritable home directory, and jobs export a writable `$HOME` instead. The JVM does not follow: on Linux it takes `user.home` from the OS passwd entry and ignores `$HOME`.

```console
$ HOME=/tmp/writable java -XshowSettings:properties -version 2>&1 | grep user.home
    user.home = /var/www          # not /tmp/writable
```

fhirlint therefore starts every validator process with `-Duser.home` set to the same home it uses itself, so the two agree on where `~/.fhirlint` and the `~/.fhir` package cache live. Nothing to configure — on an ordinary machine the two are the same directory.

If you set `user.home` yourself through `JAVA_TOOL_OPTIONS`, that wins; fhirlint does not override it.

### Skipping non-FHIR files

Pointed at a whole repo, fhirlint reports every unrelated `.json` as a hard failure:

```text
▶ package.json
  ✗ FATAL  Unable to find resourceType property
```

`--skip-non-fhir` drops those instead:

```bash
fhirlint validate ./ --skip-non-fhir
# Skipped 3 non-FHIR file(s) (--skip-non-fhir)
```

A file is only dropped when it parses cleanly **and** demonstrably lacks the FHIR marker — a `resourceType` for JSON, the `http://hl7.org/fhir` namespace for XML. That verdict does not depend on file size: the top-level keys are walked without reading their values, so a package's `.index.json` is skipped whether it lists five files or five thousand. Anything malformed, truncated or unreadable is still validated and still fails, so a broken resource can never disappear from the report by mistake. The number of skipped files is always printed to stderr. Line-delimited exports and mapping files are never skipped — a bulk export is FHIR by definition, and a mapping file is not something to second-guess from its first bytes.

The flag is opt-in: without it, non-FHIR input is reported as before.

### Passing raw validator arguments

Every validator option fhirlint supports is mapped explicitly, so a newly added upstream flag normally needs a fhirlint release before you can reach it. `--validator-arg` forwards arguments to the JAR verbatim, as an escape hatch:

```bash
fhirlint validate patient.json --validator-arg=-some-new-flag --validator-arg=value
```

Repeat the flag once per argument — a flag and its value are two separate arguments. They are appended after the arguments fhirlint builds, so for options the validator resolves last-wins, yours take effect.

This is deliberately unchecked: fhirlint does not know what these arguments mean, and a malformed one breaks the run with whatever error the JAR produces. Prefer a dedicated flag where one exists.

Arguments that collide with what fhirlint manages itself are rejected up front, because the report parsing depends on them:

```console
$ fhirlint validate patient.json --validator-arg=-output-style --validator-arg=text
Error: --validator-arg "-output-style" is not allowed: fhirlint needs -output-style json to read the results
```

`-output`, `-output-style` and `-jar` are reserved this way. Use `--jar` or `FHIRLINT_JAR` to point at a different validator JAR.

`--extract-each`, `--url` and `--watch` take a single input and cannot be combined with a list of paths.

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

### Choosing profiles by content

`profile-map:` in `fhirlint.yml` adds profiles to files by resource type or filename glob. An entry can also carry a FHIRPath condition under `when`, so that only matching resources get the profile. For example, only an Observation whose code is a LOINC vital sign gets the vital-signs profile:

```yaml
profile-map:
  Observation:
    - profile: http://hl7.org/fhir/StructureDefinition/vitalsigns
      when: "code.coding.where(system='http://loinc.org' and (code='8867-4' or code='8310-5')).exists()"
    - https://example.org/StructureDefinition/my-observation   # always, as before
  Patient: https://example.org/StructureDefinition/my-patient
```

Every entry whose condition holds applies. The condition uses the same [FHIRPath subset](rules.md#supported-fhirpath-subset) as custom lint rules, which has no union (`|`), so write `(code='a' or code='b')` rather than `code in ('a' | 'b')`. A condition that does not compile is a config error, from `validate` and `config check` alike. One that fails while being evaluated, for example because it does not return a boolean, does not apply and prints a warning.

Conditions need JSON input. For an XML file the conditional entries are skipped with a warning, and the unconditional ones still apply. `--format json` lists what was added and why, per file:

```json
"appliedProfiles": [
  { "profile": "http://hl7.org/fhir/StructureDefinition/vitalsigns", "reason": "profile-map: Observation when code.coding.where(…).exists()" }
]
```

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

# GitHub Actions annotations, shown inline in the PR's "Files changed" diff
fhirlint validate ./fhir/ --format github

# Multiple formats in one run
fhirlint validate patient.json --format terminal --format json --output results.json

# Filter by minimum severity
fhirlint validate patient.json --severity warning   # hide information
fhirlint validate patient.json --severity error     # errors only

# Suppress output for valid files (only show files with issues)
fhirlint validate ./fhir/ --quiet

# Group repeated findings instead of printing every occurrence
fhirlint validate ./fhir/ --group

# Disable ANSI colors (useful for CI environments without color support)
fhirlint validate patient.json --no-color
```

The JSON report has a schema, [`fhirlint-report.schema.json`](../fhirlint-report.schema.json); fields are only ever added. CI checks the SARIF, Code Climate, JUnit and JSON reports against their consumers' schemas on every pull request.

### What produced the report

A report is read long after the run, and the validator changes what it finds from release to release, so every archived format says what produced it — once, not per file:

| format | where |
|---|---|
| `json` | a top-level `meta` object: `{"fhirlint": "1.17.0", "validator": "6.10.4", "fhirVersion": "4.0.1"}` |
| `sarif` | `tool.driver` stays fhirlint; the validator is a `tool.extensions[]` entry named `HL7 FHIR Validator` with its version |
| `junit` | `<properties>` on the `<testsuite>`: `fhirlint.version`, `validator.version`, `fhir.version` |
| `html` | the meta line under the heading |
| `markdown` | a footnote |

`validator` is the JAR fhirlint executed — the manifest of `--jar`/`FHIRLINT_JAR` when set, otherwise the pin or the cached JAR. Validators from 6.10.5 on also state what they are on every OperationOutcome, as the `validator-version` extension, with Git SHA and build date; when present that line is carried too (`meta.validatorBuild`, the SARIF extension's `build` property, `validator.build` in JUnit) and preferred where there is room for only one. A field that cannot be told is omitted, never filled with `unknown`.

### Grouping repeated findings

Validating a directory prints every finding of every file, so one `dom-6` across 500 resources is 500 blocks of output. `--group` collapses identical findings into one block each:

```text
  ⚠ WARN   dom-6  ·  483 files
           A resource should have narrative for robust management
           fhir/Patient-001.json, fhir/Patient-002.json, fhir/Patient-003.json  … and 480 more

  ✗ ERROR  Rule_bdl_1  ·  2 files
           Bundle entry missing fullUrl
           fhir/bundle-a.json:14, fhir/bundle-b.json:9
```

Findings group by severity, message ID and message text, so two messages naming different codes stay separate — the repetition worth collapsing is the same problem in many places, not different problems that look alike. Groups are ordered most severe first, then most frequent, which is roughly the order you would work through them.

Where a file has the same finding more than once — a Bundle failing one constraint in several entries — the header says so: `12 occurrences in 4 files`. Occurrence counts, and therefore the summary line and the exit code, are identical with grouping on or off. This is presentation only, and only for the terminal: JSON, SARIF, JUnit, CodeClimate and `github` stay exhaustive, because machine consumers need every occurrence.

It is opt-in rather than automatic above some threshold, so nothing scraping terminal output changes shape on the day a dataset grows past the limit.

With `--show-source`, one snippet is printed per group, from the first occurrence. With `--show-suppressed`, suppressed findings are grouped the same way. `--quiet` has no additional effect: grouped output never lists clean files in the first place.

### Finding slow files

The validator logs how long it took on each file. fhirlint keeps that time as `durationMs` on each file in `--format json`, and `--slowest N` lists the slowest files after the terminal summary:

```bash
fhirlint validate ./fhir/ --slowest 5
```

```text
Slowest files (time the validator logged per file):
     4.302 s  fhir/Patient-a.json
     0.590 s  fhir/Patient-b.json
```

The time is per file, and it does not include package loading: neither the packages loaded at startup nor those the validator loads lazily while on a file. Measured with 6.10.3, a US Core Patient logged 0.8 s while the 15 packages loaded for it took over 20 s. So a run can be slow while every file looks fast; the run's total is in the validator's `Done. Times:` line. A cached result (`--cache`) took no validation time in the run, so it has no `durationMs` and is not listed. The same goes for a file the validator crashed on.

### Showing the offending line

`--show-source` prints the source line under each finding, with a caret at the reported column:

```text
  ✗ ERROR  Not a valid date format: 'not-a-date'
           @ Patient.birthDate (line 5, col 28)
           5 │   "birthDate": "not-a-date"
             │                            ^
```

It is opt-in. fhirlint output tends to end up in CI logs, and adding two lines per finding for everyone by default is a cost you did not ask for — turn it on locally, or set `show-source: true` in `fhirlint.yml` for a project that wants it everywhere.

Two things worth knowing about the caret:

- The column comes from the HL7 validator, which reports the **end** of the element rather than the start of the value. fhirlint shows exactly what the validator reported instead of guessing a nicer position.
- With `--extract`, `--ignore` or `--bundle-entries`, the line numbers refer to the **preprocessed** resource that was actually validated, not to your original file. That was already true of the `(line N, col N)` text; the snippet just makes it visible. Your source file is never modified.

Long lines — minified resources are often one enormous line — are truncated around the column, marked with `…`. Where the line cannot be shown faithfully (input read from a stream, a line past the end of the file), no snippet is printed rather than a wrong one.

### Very long messages

The validator quotes the offending value into its message, sometimes several times: a 300,000-character `gender` code gives a 900,000-character message. Every report except `json` cuts a message longer than 2,000 characters in the middle, keeping the start (what is wrong) and the end (where), and marks the cut:

```text
The value provided ('xxxx … [898,442 characters omitted; full text with --format json] … xxxx') was not found …
```

This applies to the terminal, `github`, `markdown`, `junit`, `html`, `sarif`, `codeclimate` and the language server. `json` keeps the exact text. Suppression `pattern:` rules and baselines still match against the full message.

### PHI-safe reports

Findings carry more of the resource than is obvious. The validator quotes the offending value into its message text, and `--show-source` renders the offending line verbatim. On real patient data both are PHI — and several output paths outlive the run:

- `--format sarif` is meant for GitHub code scanning. An upload puts the finding text into the security tab and keeps it in its history.
- `--format json`, `html` and `junit` are routinely archived as CI artifacts.

`--redact` strips resource-derived content from every report:

```bash
fhirlint validate ./patients/ --format sarif --redact
```

```text
  ✗ ERROR  [redacted] Type_Specific_Checks_DT_Date_Valid
           @ Patient.birthDate (line 5, col 28)
```

What is removed: the validator's message text, and source lines (which become unrenderable, not merely switched off — `--show-source` alongside `--redact` is not an error, it simply loses).

What is kept: severity, the FHIRPath location, the message ID, the file path, and the reason on a suppressed finding — that one comes from your own config, not from the resource. Together that is enough to act on a finding via `fhirlint explain <messageId>`.

Every finding is marked `"redacted": true` in the machine-readable formats, and a note goes to stderr after the reports are written, so a stripped report is never mistaken for a complete one.

Set it in `fhirlint.yml` rather than per invocation if the whole project needs it — then a new CI job cannot forget the flag:

```yaml
redact: true
```

**On the guarantee.** Removal is total, not selective: message text goes entirely rather than being filtered for values. Filtering would mean recognising every shape in which the validator embeds a value, across a message catalogue that is large and changes between releases, and a redaction check that is wrong occasionally is worse than none — because it gets trusted. The cost is real: you lose "is not a valid date" along with the date. `fhirlint explain` and the location are what you work from instead.

The same option exists in the Go library as `Options.Redact`, for callers forwarding findings to a log aggregator, an issue tracker or a third-party dashboard.

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

Both `--extract` and `--ignore` also work on XML input using the same `$.element.child` path syntax. For `--extract` on XML, the path must point to the actual FHIR resource element (e.g., `$.entry.resource.Patient`) since XML namespaces are injected automatically. See [Validating partial JSON](extract.md) for details.

## Explaining message IDs

Validation output references message IDs like `dom-6` or `ele-1` without saying what they mean. `fhirlint explain` looks one up — what the rule is, where it comes from, and how to fix it — fully offline, no JAR required.

```bash
fhirlint explain dom-6
```

```text
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

```text
  ⚠ WARN   A resource should have narrative for robust management
           @ Patient
           ↳ Run: fhirlint explain dom-6
```

List every ID with a built-in explanation:

```bash
fhirlint explain --list
```

fhirlint ships explanations for common FHIR core invariants (`dom-*`, `ele-1`, `ext-1`, `bdl-*`, `obs-*`); the set grows over time. Unknown IDs exit non-zero with a pointer to the [HL7 FHIR specification](https://hl7.org/fhir/).

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

```text
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

## Editor integration (language server)

`fhirlint lsp` runs a Language Server Protocol server over stdio, so findings appear inline while you edit instead of only when you run the CLI:

```bash
fhirlint lsp
```

It is launched by your editor, not by hand. Diagnostics carry the HL7 message id as the diagnostic code, hovering a finding renders the same offline explanation `fhirlint explain` prints, and a quick fix writes a suppression into `fhirlint.yml`.

A validator server is started once and kept warm for the session, so validating a buffer takes milliseconds rather than the tens of seconds a cold JVM needs. Share one across editors by starting it yourself:

```bash
fhirlint serve --port 8080 --ig hl7.fhir.us.core#9.0.0
fhirlint lsp --server http://localhost:8080
```

`--fhir-version`, `--ig` and `--profile` fall back to `fhirlint.yml`, so a project that already has a config needs no editor-side setup. See the [Language server guide](lsp.md) for Neovim, Helix and VS Code configuration.

## Watch mode

Re-validate automatically whenever a file changes — useful during local development:

```bash
# Re-validate only changed files
fhirlint validate ./fhir/ --watch

# Re-validate all files on any change
fhirlint validate ./fhir/ --watch=all

# How often to look for changes (milliseconds, default 1000)
fhirlint validate ./fhir/ --watch --watch-interval 500

# How long to wait after a change before re-validating (milliseconds, default 100)
fhirlint validate ./fhir/ --watch --watch-settle-time 750
```

The two delays do different jobs. `--watch-interval` is how often fhirlint looks; `--watch-settle-time` is how long it waits once it has seen something. Raise the settle time when a generator writes many files at once — SUSHI rewriting `fsh-generated/`, an IG build emitting hundreds of files — and re-validation would otherwise start against a half-written tree. Raising `--watch-interval` does not help there: it only notices the change later, then still settles for the same 100 ms.

Watch mode streams the validator output directly to the terminal. Press `Ctrl-C` to stop. It is not compatible with `--format json --output` or `--url`.

## Server mode (warm validator)

Every ordinary `fhirlint validate` run starts a fresh JVM and reloads the validator's packages and terminology — tens of seconds of startup before the first resource is even looked at. For repeated validations (an editor loop, a large dataset, a CI matrix) that startup cost dominates.

`fhirlint serve` runs the validator **once** and keeps it warm as an HTTP service, so subsequent validations take milliseconds instead of tens of seconds:

```bash
# Terminal 1 — start the warm validator (loads packages once)
fhirlint serve --port 8080 --ig hl7.fhir.us.core#9.0.0

# Terminal 2 / CI — validate against it
fhirlint validate ./fhir/ --server http://localhost:8080
```

In practice this turns a ~30 s cold run into well under a second once the server is warm.

The server's **FHIR version, IGs and terminology settings are fixed at startup** — load the IGs you need with `--ig` (aliases like `us-core` work). Per request only the resource and its profile vary, so:

- Client flags that change *validation semantics* — `--fhir-version`, `--ig`, `--best-practice`, `--jurisdiction`, … — are **not** applied per request; the server's startup configuration governs them (fhirlint warns if you pass `--fhir-version`/`--ig` alongside `--server`).
- Client-side, post-validation options still work as usual: `--severity`, `--fail-on`, `--format`, suppression, baseline, custom rules, lint rules, and `--check-references`.

`serve` runs until `Ctrl-C`. It streams package-loading progress while starting and prints the URL once ready. `--server` is not compatible with `--watch`. XML and JSON resources are both supported.

See the [Server mode guide](daemon.md) for details and CI patterns.

## Built-in profile aliases

Aliases are a convenience shortcut for common IG packages. Pass the full `name#version` reference directly to target a different version.

**German profiles** (see the [German profiles guide](german-profiles.md)):

| Alias | Resolves to |
|-------|-------------|
| `kbv-basis` | `kbv.basis#1.10.0` |
| `kbv-patient` | `kbv.basis#1.10.0` |
| `mii` | all 16 MII Kerndatensatz packages (see below) |
| `mii-base` | `de.medizininformatikinitiative.kerndatensatz.base#2026.0.1` |
| `mii-laborbefund` | `de.medizininformatikinitiative.kerndatensatz.laborbefund#2026.0.3` |
| `mii-medikation` | `de.medizininformatikinitiative.kerndatensatz.medikation#2026.0.1` |
| `mii-icu` | `de.medizininformatikinitiative.kerndatensatz.icu#2026.0.2` |
| `mii-onkologie` | `de.medizininformatikinitiative.kerndatensatz.onkologie#2026.0.3` |
| `mii-biobank` | `de.medizininformatikinitiative.kerndatensatz.biobank#2026.0.1` |
| `mii-consent` | `de.medizininformatikinitiative.kerndatensatz.consent#2026.0.0` |
| `mii-molgen` | `de.medizininformatikinitiative.kerndatensatz.molgen#2026.0.4` |
| `mii-patho` | `de.medizininformatikinitiative.kerndatensatz.patho#2026.0.2` |
| `mii-studie` | `de.medizininformatikinitiative.kerndatensatz.studie#2026.0.2` |
| `mii-mikrobiologie` | `de.medizininformatikinitiative.kerndatensatz.mikrobiologie#2025.0.2` |
| `mii-bildgebung` | `de.medizininformatikinitiative.kerndatensatz.bildgebung#2026.0.0` |
| `mii-dokument` | `de.medizininformatikinitiative.kerndatensatz.dokument#2026.0.1` |
| `mii-mtb` | `de.medizininformatikinitiative.kerndatensatz.mtb#2026.0.1` |
| `mii-pros` | `de.medizininformatikinitiative.kerndatensatz.pros#2026.7.0` |
| `mii-seltene` | `de.medizininformatikinitiative.kerndatensatz.seltene#2026.0.1` |
| `diga` | `kbv.mio.diga#1.1.0` |
| `isik` | `de.gematik.isik#6.0.0` (all ISiK modules, one package) |

An alias can stand for more than one package. The MII publishes no umbrella
package — the Kerndatensatz ships module by module — so `mii` loads all 16
packages, and the `mii-*` aliases name them individually.

Alias versions follow the registry's `dist-tags.latest` rather than the highest
number it serves, because publishing a package and blessing it are separate acts.
When that tag points at a *pre-release*, the pin does not follow it: a ballot is a
draft for comment, and an alias is a default. As of September 2026 all twelve MII
modules tag a `2027.0.0-ballot*` version, so every `mii` pin is deliberately the
last final release rather than what the registry currently calls latest.
`fhirlint audit` reports this as `registry latest is a pre-release` instead of
calling the pin outdated — see [auditing the toolchain](toolchain.md#auditing-the-toolchain).

`mii-base` covers what `mii-person`, `mii-fall`, `mii-diagnose` and
`mii-prozedur` used to. Those four modules stop at `2025.0.1`; their profiles
moved wholesale into `kerndatensatz.base` for the 2026 train, and the newer
modules depend on base rather than on them.

`isik` is a single package. The per-module `de.gematik.isik-*` packages froze at
`4.0.x`, which is ISiK Stufe 4; from Stufe 5 on, gematik ships everything as
`de.gematik.isik`. At `6.0.0` that carries 178 profiles across 39 resource types,
including the ICU and laboratory content that never had its own published
package. There are no `isik-<module>` aliases, because there is no longer a
separate package to narrow a load to.
