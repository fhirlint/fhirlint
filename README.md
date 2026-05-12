<p align="center">
  <img src="assets/logo.png" alt="fhirlint" width="180"/>
</p>

# fhirlint

[![Tests](https://github.com/fhirlint/fhirlint/actions/workflows/test.yml/badge.svg)](https://github.com/fhirlint/fhirlint/actions/workflows/test.yml)
[![Lint](https://github.com/fhirlint/fhirlint/actions/workflows/lint.yml/badge.svg)](https://github.com/fhirlint/fhirlint/actions/workflows/lint.yml)
[![Security](https://github.com/fhirlint/fhirlint/actions/workflows/security.yml/badge.svg)](https://github.com/fhirlint/fhirlint/actions/workflows/security.yml)

A lightweight CLI for validating FHIR resources with a developer-friendly experience.

`fhirlint` wraps the official [HL7 FHIR Validator](https://github.com/hapifhir/org.hl7.fhir.core) and adds what it lacks: clean terminal output, multiple input sources, JSON and HTML reports, pipeline-ready exit codes, watch mode, suppression rules, and built-in aliases for German FHIR profiles (KBV, MII, DiGA).

The validator JAR is downloaded automatically on first use — no manual setup required.

**Requirements:** Java 11+

---

## Table of contents

- [Installation](#installation)
- [Input sources](#input-sources)
- [Profiles & implementation guides](#profiles--implementation-guides)
- [Output formats](#output-formats)
- [Preprocessing](#preprocessing)
- [Suppressing known issues](#suppressing-known-issues)
- [Terminology server](#terminology-server)
- [Watch mode](#watch-mode)
- [Pipeline integration](#pipeline-integration)
- [Configuration file](#configuration-file-fhirlintymll)
- [Configuration reference](#configuration-reference)
- [Built-in profile aliases](#built-in-profile-aliases)
- [JAR management](#jar-management)
- [Go library](#go-library)
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

---

## Input sources

```bash
# Single file
fhirlint validate patient.json

# Directory — all .json and .xml files, single JVM invocation
fhirlint validate ./fhir/resources/

# Stdin
cat patient.json | fhirlint validate

# HTTP endpoint (repeatable for batch validation)
fhirlint validate --url https://my-api/fhir/Patient/123
fhirlint validate \
  --url https://my-api/fhir/Patient/123 \
  --url https://my-api/fhir/Medication/abc \
  --url https://my-api/fhir/MedicationRequest/xyz

# Extract each element of a JSON array and validate separately
fhirlint validate api-response.json --extract-each "$.medications"
```

When validating multiple resources (directory, multiple `--url`, or `--extract-each`), all resources are processed in a **single JVM invocation** to avoid repeated startup overhead.

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

# Multiple formats in one run
fhirlint validate patient.json --format terminal --format json --output results.json

# Filter by minimum severity
fhirlint validate patient.json --severity warning   # hide information
fhirlint validate patient.json --severity error     # errors only
```

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

## Terminology server

By default, fhirlint sends code lookups to `https://tx.fhir.org`. This behaviour can be tuned:

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

Example GitHub Actions workflow:

```yaml
- name: Validate FHIR resources
  run: |
    fhirlint validate ./fhir/ \
      --format json --output fhir-report.json \
      --tx-cache .fhirlint-tx-cache/ \
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

A fully annotated example is provided in [`fhirlint.yml.example`](fhirlint.yml.example).

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
| `--format`, `-f` | `terminal` | Output format: `terminal`, `json`, `html` (repeatable) |
| `--output`, `-o` | stdout | Output file for `json`/`html` reports |
| `--severity`, `-s` | `information` | Minimum severity to display: `information`, `warning`, `error` |
| `--fail-on` | `error` | Exit non-zero when issues at this level or above are found: `error`, `warning`, `never` |
| `--url` | — | Fetch and validate from an HTTP endpoint (repeatable) |
| `--extract` | — | JSONPath to extract from input before validating |
| `--extract-each` | — | JSONPath to an array — validates each element separately |
| `--ignore` | — | JSONPath field to remove before validating (repeatable) |
| `--suppress` | — | Silence a known issue: `messageId:X`, `constraint:X`, or `expression:X` (repeatable) |
| `--show-suppressed` | `false` | Show suppressed issues with a muted `↷ SUPP` label |
| `--no-terminology-server` | `false` | Disable terminology server — no data sent to `tx.fhir.org` |
| `--terminology-server` | — | Custom terminology server URL |
| `--tx-cache` | — | Terminology cache directory (`n/a` to disable) |
| `--locale` | — | Locale for validation messages, e.g. `de`, `fr` |
| `--allow-example-urls` | `false` | Suppress warnings about `example.org` placeholder URLs |
| `--best-practice` | — | Best-practice constraint handling: `ignore`, `hint`, `warning`, `error` |
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
| `url` | list | `--url` |
| `extract` | string | `--extract` |
| `extract-each` | string | `--extract-each` |
| `ignore` | list | `--ignore` |
| `suppress` | list | `--suppress` |
| `show-suppressed` | bool | `--show-suppressed` |
| `no-terminology-server` | bool | `--no-terminology-server` |
| `terminology-server` | string | `--terminology-server` |
| `tx-cache` | string | `--tx-cache` |
| `locale` | string | `--locale` |
| `allow-example-urls` | bool | `--allow-example-urls` |
| `best-practice` | string | `--best-practice` |
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

The library requires Java 11+ and downloads the validator JAR on first use (~250 MB). Set `JARPath` or `FHIRLINT_JAR` to provide a pre-downloaded JAR.

---

## Guides

Detailed guides for common workflows:

- [CI/CD Integration](docs/ci.md) — GitHub Actions & GitLab CI setup, JAR and terminology caching, report artifacts
- [Validating partial JSON](docs/extract.md) — `--extract` and `--extract-each` for non-FHIR API wrappers and array responses
- [Suppression rules](docs/suppression.md) — when to suppress vs. fix, selector types, committing decisions to `fhirlint.yml`
- [German FHIR profiles](docs/german-profiles.md) — KBV, MII, DiGA: aliases, version pinning, combining profiles

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for setup instructions, the PR checklist, and how to add new flags.

## License

Apache 2.0 — see [LICENSE](LICENSE).

---

HL7® and FHIR® are registered trademarks of Health Level Seven International.
