# fhirlint

[![Tests](https://github.com/fhirlint/fhirlint/actions/workflows/test.yml/badge.svg)](https://github.com/fhirlint/fhirlint/actions/workflows/test.yml)
[![Lint](https://github.com/fhirlint/fhirlint/actions/workflows/lint.yml/badge.svg)](https://github.com/fhirlint/fhirlint/actions/workflows/lint.yml)
[![Security](https://github.com/fhirlint/fhirlint/actions/workflows/security.yml/badge.svg)](https://github.com/fhirlint/fhirlint/actions/workflows/security.yml)

A lightweight CLI for validating FHIR resources with a developer-friendly experience.

`fhirlint` wraps the official [HL7 FHIR Validator](https://github.com/hapifhir/org.hl7.fhir.core) and adds what it lacks: clean terminal output, multiple input sources, JSON and HTML reports, pipeline-ready exit codes, and built-in aliases for German FHIR profiles (KBV, MII, DiGA).

The validator JAR is downloaded automatically on first use — no manual setup required.

## Requirements

- **Java 11+** — `fhirlint` calls the official HL7 validator under the hood
- Go 1.22+ (only for building from source)

## Installation

```bash
go install github.com/fhirlint/fhirlint@latest
```

Or download a pre-built binary from the [releases page](https://github.com/fhirlint/fhirlint/releases).

## Usage

### Input sources

```bash
# File
fhirlint validate patient.json

# Directory (all .json and .xml files)
fhirlint validate ./fhir/resources/

# Stdin
cat patient.json | fhirlint validate

# HTTP endpoint
fhirlint validate --url https://my-api/fhir/Patient/123
```

### Profiles and implementation guides

```bash
# Built-in alias
fhirlint validate patient.json --profile kbv-patient

# Full IG package reference
fhirlint validate patient.json --ig kbv.basis#1.5.0

# Multiple profiles
fhirlint validate bundle.json --profile mii --ig custom.ig#1.0.0
```

### Output formats

```bash
# Colored terminal output (default)
fhirlint validate patient.json

# JSON report to stdout
fhirlint validate patient.json --format json

# HTML report to file
fhirlint validate patient.json --format html --output report.html

# Multiple formats at once
fhirlint validate patient.json --format terminal --format json --output results.json
```

### Preprocessing

```bash
# Extract a nested FHIR resource before validating (e.g. from an API wrapper)
fhirlint validate --url https://my-api/patient --extract "$.data.fhir"

# Ignore fields before validating (e.g. non-conformant extensions)
fhirlint validate patient.json --ignore "$.meta.tag" --ignore "$.text"
```

### Filtering output

```bash
# Only show errors, hide warnings and info
fhirlint validate patient.json --severity error

# Show everything including informational messages
fhirlint validate patient.json --severity information
```

## Pipeline integration

```bash
# Exit non-zero if any errors are found (default)
fhirlint validate patient.json --fail-on error

# Exit non-zero for warnings too
fhirlint validate patient.json --fail-on warning

# Never fail on validation issues (useful for reporting-only runs)
fhirlint validate patient.json --fail-on never
```

Example GitHub Actions step:

```yaml
- name: Validate FHIR resources
  run: fhirlint validate ./fhir/ --format json --output fhir-report.json --fail-on error

- uses: actions/upload-artifact@v4
  if: always()
  with:
    name: fhir-validation-report
    path: fhir-report.json
```

## All flags

| Flag | Default | Description |
|------|---------|-------------|
| `--profile`, `-p` | — | Profile alias or URL (repeatable) |
| `--ig` | — | IG package, e.g. `kbv.basis#1.5.0` (repeatable) |
| `--fhir-version` | `4.0.1` | FHIR version (`4.0.1`, `4.3.0`, `5.0.0`) |
| `--format`, `-f` | `terminal` | Output format: `terminal`, `json`, `html` (repeatable) |
| `--output`, `-o` | stdout | Output file for `json`/`html` |
| `--severity`, `-s` | `information` | Minimum severity to show: `information`, `warning`, `error` |
| `--fail-on` | `error` | Exit non-zero on: `error`, `warning`, `never` |
| `--url` | — | Fetch and validate from an HTTP endpoint |
| `--extract` | — | JSONPath to extract before validating (e.g. `$.data.resource`) |
| `--ignore` | — | JSONPath field(s) to remove before validating (repeatable) |

## Built-in profile aliases

| Alias | Package |
|-------|---------|
| `kbv-basis` | `kbv.basis#1.5.0` |
| `kbv-patient` | `kbv.basis#1.5.0` |
| `mii` | `de.medizininformatikinitiative.kerndatensatz#2024.0.0` |
| `diga` | `de.bfarm.diga#1.2.0` |

```bash
# List all available aliases
fhirlint profiles
```

## JAR management

`fhirlint` uses the official [HL7 FHIR Validator](https://github.com/hapifhir/org.hl7.fhir.core/releases) maintained by HAPI FHIR. The JAR (~250 MB) is downloaded automatically on first use and cached at `~/.fhirlint/validator_cli.jar`.

```bash
# Show fhirlint version and the cached validator JAR version
fhirlint version

# Update the cached JAR to the latest release
fhirlint update
```

## License

Apache 2.0 — see [LICENSE](LICENSE).

---

HL7® and FHIR® are registered trademarks of Health Level Seven International.
