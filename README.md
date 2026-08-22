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
- [Profile coverage](#profile-coverage)
- [Preprocessing](#preprocessing)
- [Suppressing known issues](#suppressing-known-issues)
- [Custom lint rules](#custom-lint-rules)
- [Style & naming rules](#style--naming-rules)
- [Referential integrity](#referential-integrity)
- [Explaining message IDs](#explaining-message-ids)
- [Evaluating FHIRPath expressions](#evaluating-fhirpath-expressions)
- [Baseline mode](#baseline-mode)
- [Comparing runs (change control)](#comparing-runs-change-control)
- [Comparing profiles](#comparing-profiles)
- [Computer System Validation (qualify)](#computer-system-validation-qualify)
- [Editor integration (language server)](#editor-integration-language-server)
- [Terminology server](#terminology-server)
  - [Offline terminology](#offline-terminology)
- [Watch mode](#watch-mode)
- [Server mode (warm validator)](#server-mode-warm-validator)
- [Pipeline integration](#pipeline-integration)
- [Configuration file](#configuration-file-fhirlintymll)
- [Validating the configuration file](#validating-the-configuration-file)
- [Configuration reference](#configuration-reference)
- [Built-in profile aliases](#built-in-profile-aliases)
- [JAR management](#jar-management)
- [Auditing the toolchain](#auditing-the-toolchain)
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

Release archives carry a build-provenance attestation, so you can check an archive really came from this repo's release workflow before unpacking it:

```bash
gh attestation verify fhirlint_*_linux_amd64.tar.gz --repo fhirlint/fhirlint
```

`checksums.txt` is attested too, so the checksum route is trustworthy end to end — verify the file itself, then use it to check anything it lists:

```bash
gh attestation verify checksums.txt --repo fhirlint/fhirlint
sha256sum --check --ignore-missing checksums.txt
```

It covers the archives and their SBOMs, so an attested `checksums.txt` makes those tamper-evident without each needing its own attestation.

Each archive also ships an SPDX SBOM next to it (`<archive>.sbom.json`) listing the Go modules it was built from. The SBOM files carry a provenance attestation of their own, so you can confirm one came from the release build before trusting its contents:

```bash
gh release download --repo fhirlint/fhirlint --pattern "*linux_amd64.tar.gz.sbom.json"
gh attestation verify fhirlint_*_linux_amd64.tar.gz.sbom.json --repo fhirlint/fhirlint
```

Note what that does and does not say. It establishes that the release build produced this SBOM file — not, cryptographically, that this SBOM describes that particular archive. The link between the two is the filename convention plus `checksums.txt`, which is itself attested and lists both.

A true SBOM attestation binds one SBOM to one subject artifact, so covering five archives means five separate attestations at release time. Given that the archives, their SBOMs and the checksums file are all provenance-attested and mutually covered by the attested checksums, that extra release-pipeline machinery buys little here. The [container image](#verifying-the-image) does carry a real SBOM attestation, because there it is a single subject and costs nothing.

Attestations and SBOMs apply from **v1.5.0** onward; v1.4.0 and earlier have neither.

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
docker run --rm -v $(pwd):/work ghcr.io/fhirlint/fhirlint:1.9.0 validate /work/fhir/
```

The image includes a JRE — no separate Java installation required.

#### Running as non-root

The container runs as uid `65532`, not root. Reading mounted resources works as shown above, but **writing** into a mounted directory — a report via `--output`, or `--baseline` — fails unless the host directory is writable by that uid:

```
Error: html report: open /work/report.html: permission denied
```

Run as your own user, with group `0`, when the container needs to write to the mount:

```bash
docker run --rm --user $(id -u):0 -v $(pwd):/work \
  ghcr.io/fhirlint/fhirlint validate /work/fhir/ --format html -o /work/report.html
```

Group `0` matters: the baked-in validator JAR and the cache directory are group-owned by `0`, so the validator still finds them under an arbitrary uid. A fully arbitrary `--user <uid>:<gid>` is not supported.

#### Verifying the image

From **v1.5.0** onward, images are signed with [cosign](https://docs.sigstore.dev/) (keyless, via Sigstore) and carry a build-provenance attestation. The SBOM attestation applies from **v1.6.0** onward — the v1.5.0 image is signed and provenance-attested but has no SBOM attestation ([#266](https://github.com/fhirlint/fhirlint/issues/266)).

Check the signature:

```bash
cosign verify ghcr.io/fhirlint/fhirlint:<version> \
  --certificate-identity-regexp '^https://github\.com/fhirlint/fhirlint/\.github/workflows/docker\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Check that the image was built by this repo's release workflow, and inspect its SBOM:

```bash
gh attestation verify oci://ghcr.io/fhirlint/fhirlint:<version> --repo fhirlint/fhirlint
cosign download attestation ghcr.io/fhirlint/fhirlint:<version>
```

Signature and attestations are bound to the image **digest**, so verifying a tag checks whatever that tag currently points at. Pin the digest (`ghcr.io/fhirlint/fhirlint@sha256:...`) when you need the stronger guarantee.

Images published before v1.5.0 are unsigned.

### GitHub Actions

Use the action to validate FHIR resources in one step — it installs fhirlint and runs `validate`:

```yaml
- uses: fhirlint/fhirlint@v1.9.0
  with:
    path: ./fhir/
    fail-on: error
    severity: warning
```

Supported inputs: `path`, `url`, `profile`, `ig`, `severity`, `fail-on`, `format`, `output`, `since`, `tx-offline`, `tx-dir`, and `version` (a release tag or `latest`). `profile` and `ig` accept multiple values, one per line. The action runs on Linux runners (Java 17+ is preinstalled on `ubuntu-latest`).

#### Validating only what the pull request changed

`since` scopes the run to files changed against the base branch. It needs the full history, so `actions/checkout` must be told not to make a shallow clone:

```yaml
- uses: actions/checkout@v5
  with:
    fetch-depth: 0          # required: the default shallow clone has no merge base

- uses: fhirlint/fhirlint@v1.9.0
  with:
    path: ./fhir/
    since: origin/${{ github.base_ref }}
```

With `fetch-depth: 0` missing, the action stops with an error naming the fix rather than passing git's own message through.

#### Running without a terminology server

`tx-offline` replays a recording made by `fhirlint tx warm`, so the run does not depend on `tx.fhir.org`:

```yaml
- uses: fhirlint/fhirlint@v1.9.0
  with:
    path: ./fhir/
    tx-offline: true
```

Point `tx-dir` at the recording if it is not in the default `.fhirlint-tx/`. See [offline terminology](#offline-terminology).

#### Inline PR annotations

`--format github` emits [workflow commands](https://docs.github.com/en/actions/reference/workflow-commands-for-github-actions), which the runner turns into annotations shown directly on the offending lines in the PR's "Files changed" view:

```yaml
- uses: fhirlint/fhirlint@v1.9.0
  with:
    path: ./fhir/
    format: github
```

No extra permissions and no upload step — unlike SARIF, which needs `security-events: write` and `upload-sarif` to reach the Security tab. Severities map to `error` (also for `fatal`), `warning` and `notice`; each annotation is titled with the HL7 message id and the FHIRPath expression.

Combine it with a human-readable format in the same run:

```bash
fhirlint validate ./fhir/ --format terminal --format github
```

Annotations must be written to the job's stdout for the runner to see them, so `--output` does not apply to this format.

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

### Pre-commit hook

fhirlint ships hook definitions for the [pre-commit](https://pre-commit.com/) framework, so resources are validated before they ever reach CI.

```yaml
# .pre-commit-config.yaml
repos:
  - repo: https://github.com/fhirlint/fhirlint
    rev: v1.9.0
    hooks:
      - id: fhirlint
        files: ^input/resources/.*\.json$
```

```bash
pre-commit install
```

Both hooks pass `--skip-non-fhir`, so unrelated files such as `package.json` are skipped instead of failing the commit. Narrowing `files:` to the directory holding your resources is still worthwhile — it keeps the hook from opening every JSON file in the repo on each commit — but it is no longer required to get a usable result. A malformed resource is still validated and still fails, so the hook cannot silently pass a broken file.

Two hook ids are available:

| id | Runs via | Notes |
|----|----------|-------|
| `fhirlint` | `language: golang` | Builds fhirlint from source on first install. Downloads the validator JAR (~250 MB) on the first validation run. |
| `fhirlint-docker` | `language: docker_image` | Uses the prebuilt image with the JAR baked in — no Go toolchain, no first-run JAR download. Requires Docker. The image tag floats, so `rev:` pins the hook definition but not the image contents. |

Both pass the staged files to a single `fhirlint validate` invocation (one JVM start for the whole commit) and default to `args: [--quiet, --skip-non-fhir]`. Setting `args:` yourself replaces that default entirely, so repeat the flags you want to keep:

```yaml
      - id: fhirlint
        files: ^input/resources/.*\.json$
        args: [--quiet, --skip-non-fhir, --profile, kbv-patient, --fail-on, error]
```

### Use as a Claude Code plugin

Validate FHIR resources from inside an agentic [Claude Code](https://claude.com/claude-code) session. The plugin is a thin wrapper around the `fhirlint` binary: Claude runs it, **interprets** the validator's findings, and fixes the offending resource in place.

```
/plugin marketplace add fhirlint/fhirlint
/plugin install fhirlint@fhirlint
```

This installs two skills:

- **validate** — runs `fhirlint validate`, explains the issues, and proposes/applies fixes to your resources.
- **audit** — runs `fhirlint audit` to check the validator JAR and the IG packages pinned in `fhirlint.lock` for updates and known security advisories.

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

```
Error: --extract cannot be used with FHIR Mapping Language input (.fml):
fhirlint passes it to the validator unchanged
```

### Running where `$HOME` is not the passwd home

CI runners sometimes have an unwritable home directory, and jobs export a writable `$HOME` instead. The JVM does not follow: on Linux it takes `user.home` from the OS passwd entry and ignores `$HOME`.

```
$ HOME=/tmp/writable java -XshowSettings:properties -version 2>&1 | grep user.home
    user.home = /var/www          # not /tmp/writable
```

fhirlint therefore starts every validator process with `-Duser.home` set to the same home it uses itself, so the two agree on where `~/.fhirlint` and the `~/.fhir` package cache live. Nothing to configure — on an ordinary machine the two are the same directory.

If you set `user.home` yourself through `JAVA_TOOL_OPTIONS`, that wins; fhirlint does not override it.

### Skipping non-FHIR files

Pointed at a whole repo, fhirlint reports every unrelated `.json` as a hard failure:

```
▶ package.json
  ✗ FATAL  Unable to find resourceType property
```

`--skip-non-fhir` drops those instead:

```bash
fhirlint validate ./ --skip-non-fhir
# Skipped 3 non-FHIR file(s) (--skip-non-fhir)
```

A file is only dropped when it parses cleanly **and** demonstrably lacks the FHIR marker — a `resourceType` for JSON, the `http://hl7.org/fhir` namespace for XML. Anything malformed, truncated or unreadable is still validated and still fails, so a broken resource can never disappear from the report by mistake. The number of skipped files is always printed to stderr. Line-delimited exports and mapping files are never skipped — a bulk export is FHIR by definition, and a mapping file is not something to second-guess from its first bytes.

The flag is opt-in: without it, non-FHIR input is reported as before.

### Passing raw validator arguments

Every validator option fhirlint supports is mapped explicitly, so a newly added upstream flag normally needs a fhirlint release before you can reach it. `--validator-arg` forwards arguments to the JAR verbatim, as an escape hatch:

```bash
fhirlint validate patient.json --validator-arg=-some-new-flag --validator-arg=value
```

Repeat the flag once per argument — a flag and its value are two separate arguments. They are appended after the arguments fhirlint builds, so for options the validator resolves last-wins, yours take effect.

This is deliberately unchecked: fhirlint does not know what these arguments mean, and a malformed one breaks the run with whatever error the JAR produces. Prefer a dedicated flag where one exists.

Arguments that collide with what fhirlint manages itself are rejected up front, because the report parsing depends on them:

```
$ fhirlint validate patient.json --validator-arg=-output-style --validator-arg=text
Error: --validator-arg "-output-style" is not allowed: fhirlint needs -output-style json to read the results
```

`-output`, `-output-style` and `-jar` are reserved this way. Use `--jar` or `FHIRLINT_JAR` to point at a different validator JAR.

`--extract-each`, `--url` and `--watch` take a single input and cannot be combined with a list of paths.

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

### Grouping repeated findings

Validating a directory prints every finding of every file, so one `dom-6` across 500 resources is 500 blocks of output. `--group` collapses identical findings into one block each:

```
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

### Showing the offending line

`--show-source` prints the source line under each finding, with a caret at the reported column:

```
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

### PHI-safe reports

Findings carry more of the resource than is obvious. The validator quotes the offending value into its message text, and `--show-source` renders the offending line verbatim. On real patient data both are PHI — and several output paths outlive the run:

- `--format sarif` is meant for GitHub code scanning. An upload puts the finding text into the security tab and keeps it in its history.
- `--format json`, `html` and `junit` are routinely archived as CI artifacts.

`--redact` strips resource-derived content from every report:

```bash
fhirlint validate ./patients/ --format sarif --redact
```

```
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

Resource-type and profile counts are gathered **offline** by parsing each file (`.json`, `.jsonl`, `.ndjson`, `.xml`); each line of a line-delimited export counts as a resource. The validation summary runs the validator — skip it for an instant, offline overview:

```bash
fhirlint stats ./fhir/ --no-validate

# Machine-readable, for dashboards or badge generators
fhirlint stats ./fhir/ --format json --output stats.json
```

`stats` is informational and always exits `0` on success (it never fails the build); use `validate` for CI gating. `--exclude` patterns and `.fhirlintignore` are respected, just like `validate`.

---

## Profile coverage

`fhirlint coverage` measures how much of a profile a set of resources actually exercises, with an emphasis on the `mustSupport` elements.

The validator cannot answer this. It sees one resource at a time and only checks what is present, so it has no way to notice that across your whole example set nobody has ever populated `Patient.identifier:VersichertenId-GKV`. An IG can ship thirty green examples and still leave half its must-support elements untouched.

```bash
fhirlint coverage ./examples/ --ig de.gematik.isik-basismodul#4.0.3 --profile ISiKPatient
```

```
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

Profiles come from IG packages in the local FHIR package cache (`~/.fhir/packages`). A package that is not cached yet is downloaded from the FHIR package registry and verified against the checksum the registry publishes for it. `--offline` forbids that and fails instead, for hermetic builds:

```bash
fhirlint coverage ./examples/ --ig kbv.basis#1.9.0 --offline
```

The checksum is the registry's own `dist.shasum`. It catches a truncated or tampered transfer; it is not a defence against the registry itself, since the digest and the archive come from the same place. A mismatch fails the install. A checksum that cannot be fetched at all is a visible warning rather than a failure, matching how the validator JAR is handled — but it is never silent. (The JAR itself gets a stronger check where upstream allows one: see [SECURITY.md](SECURITY.md#properties-you-can-rely-on).)

### How resources are attributed

By `meta.profile`. A resource that declares the profile is measured against it.

Naming profiles with `--profile` additionally allows resources of the matching resource type that declare *no* profile to be measured. Without `--profile`, that is deliberately off: attributing a bare `Patient` to all thirty Patient profiles in an IG produces noise, not information. Resources measured this way are counted separately in the output, because coverage established that way describes your dataset rather than conformance to the profile.

A profile that no resource was attributed to is left out entirely rather than listed at 0%. Nothing was measured for it, and a row reading "0%" would be a verdict on data that does not exist.

### What "not measurable" means

Slices are how German profiles carry most of their must-support elements — 54 of 64 for ISiKPatient — so coverage resolves slice membership rather than skipping it. A slice is matched by the `pattern[x]` or `fixed[x]` its definition declares, by an extension's URL, or by a discriminator pointing at a child element that fixes a value. When the definition lives in another package — a `HumanName` constrained through `humanname-de-basis`, say — the lookup follows it there, which is why supporting packages in your cache are loaded alongside the ones you name.

Some slices cannot be decided this way. The common case is a slice identified only by a value set binding: deciding membership would mean expanding the value set, which is terminology work, and coverage runs entirely offline. Those elements are reported as **not measurable** and excluded from the denominator:

```
  not measurable: 8 element(s)
      slice is identified by a value set binding (http://hl7.org/fhir/ValueSet/organization-type), which coverage does not expand
```

They count as neither covered nor uncovered. Counting them as misses would report a limit of the tool as a gap in your data.

The other common cause is a slice that is perfectly well defined, just not in a package you have. German profiles delegate datatype constraints — a `HumanName` through `humanname-de-basis` — and the report names the profile that is missing so you can add it:

```
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

### Expiring suppressions

A suppression added to work around a temporary problem tends to outlive it, quietly hiding findings nobody revisits. Give it a deadline in `fhirlint.yml`:

```yaml
suppress:
  - constraint: dom-6
    reason: "narrative added in the next sprint"
    expires: 2026-12-31
```

The date is **inclusive**: the rule suppresses throughout 2026-12-31 and lapses on 2027-01-01.

Once it lapses the rule stops suppressing — the findings come back, and if they are errors the build fails again. That is the point of the feature, so fhirlint says exactly what happened rather than leaving a mysterious failure:

```
warn: suppress rule "constraint:dom-6" expired on 2026-12-31 and no longer suppresses anything
```

For the 14 days before, it warns while still suppressing, so the lapse is not a surprise on the day:

```
warn: suppress rule "constraint:dom-6" expires on 2026-12-31
```

An unparseable date is an error rather than a silently ignored field — a typo must not turn a temporary suppression into a permanent one:

```
Error: invalid suppress expires "31.12.2026": use YYYY-MM-DD
```

Expiry is config-only. `--suppress` on the command line is for one-off runs, where a deadline has little meaning.

### Requiring a reason

`reason` is optional, so a rule can silence a finding with nothing recorded about why. For projects where an unexplained suppression is hard to defend later, make it mandatory:

```yaml
require-suppress-reason: true
```

A rule without a reason then fails the run, naming the offenders:

```
Error: require-suppress-reason is set, but 1 suppress rule(s) have no reason: "constraint:dom-6"
```

It is opt-in, since turning it on by default would break every config with a bare suppression. There is a `--require-suppress-reason` flag for enabling it from the command line, e.g. to enforce the policy in CI without changing the committed config.

The check covers **every** place a suppression can be written — `--suppress` on the command line, the global `suppress:` block, `suppress:` nested under `overrides:`, and the `severity-override:` rules described below. A policy that only covered one of them could be sidestepped by moving the rule somewhere else. Whitespace does not count as a reason.

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

### Re-levelling instead of hiding

Suppression is all-or-nothing. Some findings deserve the middle option: an error caused by a defect in someone else's published profile will not be fixed on your schedule, must not fail CI, and must not disappear either — silently dropping it means nobody notices when the profile is eventually corrected.

`severity-override:` changes what a finding *is*, using the same selectors as `suppress`:

```yaml
severity-override:
  - messageId: Rule_bdl_1
    severity: warning     # the level to apply — required
    reason: "constraint is wrong in DAV-PR-ERP-AbgabedatenBundle 1.0.3"
    expires: 2026-12-31

  - pattern: "Unknown extension.*"
    severity: error       # upgrades work too
    reason: "extensions must be declared in our IG"
```

Note the difference in what `severity` means: under `suppress:` it narrows *which* findings a rule matches, while under `severity-override:` it is the level to apply. That is why it is mandatory here — a rule without one says nothing.

The new level is the real one from that point on. It drives terminal output, `severity` filtering, `--fail-on`, `--max-warnings`, the baseline and the exit code, so a downgraded error genuinely stops failing the build instead of merely looking different. Rules run **before** suppression, so a `suppress` rule with a `severity` filter matches against the new level.

Reports do not lose the original. Terminal output marks a re-levelled finding, and JSON and SARIF carry both levels, so a report cannot mislead a reader who does not have the config to hand:

```
  ⚠ WARN   Rule bdl-1: A Bundle entry must have a fullUrl
           ↕ reported as error
```

```json
{ "severity": "warning", "originalSeverity": "error", "messageId": "Rule_bdl_1", ... }
```

`reason` and `expires` behave exactly as they do for suppressions, including the warning when a rule lapses or matches nothing, and `require-suppress-reason` covers these rules too — a downgrade stops a finding failing the build just as effectively as hiding it does. Config only: a re-levelling carries a reason and usually a date, which is not something to retype on every invocation.

### Sharing the rules with the validator

Suppression rules only hold for runs that go through fhirlint. A raw `validator_cli.jar` run, or an IG Publisher build, still reports everything the project has accepted. Both read the validator's own advisor file, so export yours:

```bash
fhirlint suppress export -o advisor.json

java -jar validator_cli.jar patient.json -advisor-file advisor.json
```

The advisor format filters by message ID and element path only, so `messageId` and `expression` rules export while `constraint`, `pattern` and severity-filtered rules cannot. Every dropped rule is listed with the reason, and `--strict` turns that into a non-zero exit for CI. See the [suppression guide](docs/suppression.md#sharing-rules-with-the-validator-advisor-file) for the full mapping.

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

## Style & naming rules

Profile validation checks conformance to the FHIR spec, but it does not enforce **authoring conventions** — lowercase-hyphen resource ids, a house canonical-URL base, PascalCase profile names. Built-in lint rules add those checks. They are **opt-in**: nothing runs unless you enable it under the `lint:` key in `fhirlint.yml`, and each rule carries its own severity.

```yaml
lint:
  id-kebab-case: warning              # resource id should be lowercase kebab-case
  profile-name-pascalcase: warning    # StructureDefinition.name should be PascalCase
  canonical-url-pattern:              # canonical url must start with a base
    severity: error
    base: "https://example.org/fhir/"
```

Findings carry the message ID `lint:<rule>`, so they behave like any other issue — shown in every report, filtered by `--severity`, gated by `--fail-on`, and suppressible or baselineable:

```bash
fhirlint validate ./fhir/ --suppress messageId:lint:id-kebab-case
```

These rules catch conventions the validator itself accepts. For example the id `ExamplePatient1` is a valid FHIR id (the JAR reports no error), but `id-kebab-case` flags it as not lowercase-hyphenated. Rules run in-process against FHIR **JSON**; XML resources are skipped with a notice.

See the [Style & naming rules guide](docs/lint-rules.md) for the full rule list and options. For **project-specific** assertions beyond these built-ins, see custom FHIRPath rules (`rules:` / `--rules-file`).

---

## Referential integrity

Profile validation checks each resource in isolation — it does not catch a **dangling reference**, where a `reference` points at a resource that is not present in the dataset. Pass `--check-references` to validate the reference graph across the whole input set (a directory, a Bundle, or NDJSON):

```bash
fhirlint validate ./fhir/ --check-references
```

fhirlint indexes every resource's identity (`ResourceType/id`, and Bundle entry `fullUrl`s) and then resolves each literal reference against that index:

- **Local references** — a relative `Patient/123`, a `urn:uuid:…` matching a Bundle entry, or a contained `#id` — that do not resolve are reported as an **error** with message ID `ref:unresolved`.
- **Absolute references** to a server outside the set (`https://other.example/fhir/Patient/1`) cannot be checked and are reported as **information** with message ID `ref:external`.
- References by `identifier` only (no literal `.reference`) and canonical URLs are not reference-resolved.

The check runs in-process (no JVM round-trip) and findings behave like any other issue — filtered by `--severity`, gated by `--fail-on`, and suppressible or baselineable:

```bash
fhirlint validate ./fhir/ --check-references --suppress messageId:ref:external
```

It is opt-in: when you validate a subset of a dataset, references to resources you did not include will (correctly) report as unresolved. XML resources are skipped with a notice.

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

**Caching the terminology responses in CI** speeds up repeated runs. Add the cache directory to `actions/cache`:

```yaml
- uses: actions/cache@v4
  with:
    path: .fhirlint-tx-cache
    key: fhirlint-tx-${{ runner.os }}-${{ hashFiles('fhirlint.yml') }}

- name: Validate FHIR resources
  run: fhirlint validate ./fhir/ --tx-cache .fhirlint-tx-cache/
```

> **`--tx-cache` does not make a run independent of the terminology server.** It is a latency optimisation, not a reproducibility measure. The validator fetches the server's capability statement at startup, before validating anything, and aborts when that fails — warm cache or not. With a fully populated cache and an unreachable server you get no report at all:
>
> ```
> FHIRException: Error fetching the server's capability statement: Failed to connect to ...
> ```
>
> For runs that must not depend on the server, see [offline terminology](#offline-terminology) below.

### When the validator stops checking codes

Checking a code costs a terminology round trip, so the validator caps how many it will check for a single ValueSet include, ConceptMap group or CodeSystem supplement. Past the cap it checks **none** of them and issues a hint:

```
Checks skipped: 2 — too many codes to check against a code system. See --codesystem-size-limit.
```

The cap is 1000 by default. A German project validating against ICD-10-GM, OPS or ATC is over it immediately, so a green run there is not evidence that the codes are valid — it can mean nobody looked. fhirlint counts these hints separately in the summary for that reason: they arrive at hint severity, which most projects filter out, and the message saying a check did not run would otherwise be the first thing hidden.

```bash
# Check them all, however many there are
fhirlint validate ./ig/ --codesystem-size-limit 0

# Or raise the cap
fhirlint validate ./ig/ --codesystem-size-limit 5000
```

Expect a remote terminology server to be slow about it — one round trip per code. Recording once with [`fhirlint tx warm`](#offline-terminology) and replaying makes repeat runs cheap.

`fhirlint explain VALUESET_INC_TOO_MANY_CODES` (also `CONCEPTMAP_VS_TOO_MANY_CODES`, `CODESYSTEM_CS_SUPP_TOO_MANY_CODES`) spells this out offline. The CodeSystem-supplement form is new in validator 6.10.2: a supplement that used to be checked can stop being checked after upgrading.

### Air-gapped runs (`--offline`)

`--tx-offline` covers terminology. `--offline` covers the rest of the run:

```bash
fhirlint validate ./fhir/ --offline
```

- the validator JAR is taken from the cache and never downloaded
- the update check is skipped
- IG packages must already be in `~/.fhir/packages`, and a missing one is named before the JAR starts rather than surfacing as a package-resolution error
- terminology is skipped, unless a recording is replayed with `--tx-offline`
- the validator is told to block its own HTTP with `-no-http-access`

The last point is what makes it a guarantee rather than a promise about fhirlint's own code paths — the JAR refuses every request itself, including any URL that content asks it to follow.

**One combination gets a weaker guarantee, and says so.** `--tx-offline` replays terminology from a local server on loopback, and the validator's block has no loopback exemption — it refuses `http://127.0.0.1` exactly as it refuses `https://tx.fhir.org`. So `--offline --tx-offline` cannot use the block:

```
--offline: terminology is replayed from a local server, so the validator's own
network block cannot be used. fhirlint downloads nothing; the JAR could still
follow a URL if content asks it to. For a hard block, drop --tx-offline or
isolate the network.
```

`--offline` needs validator 6.10.0 or newer (where `-no-http-access` was added) and refuses to run against an older JAR rather than failing obscurely. `--offline` and `--terminology-server` are mutually exclusive.

The same flag is on [`fhirlint serve`](#server-mode-warm-validator) and [`fhirlint lsp`](#editor-integration-language-server), where the decision is made once for the server's lifetime:

```bash
fhirlint serve --offline --ig kbv.basis#1.9.0
fhirlint lsp --offline
```

`lsp --offline --server <url>` is the one case where it does nothing: that server belongs to another process, started with whatever policy its owner chose, and the run says so instead of implying otherwise.

### Authenticating to a terminology server

A terminology server that is not `tx.fhir.org` usually wants a credential. fhirlint reads it from the environment — one variable per scheme, and the one that is set decides the scheme:

| Variable | Sent as |
|---|---|
| `FHIRLINT_TX_TOKEN` | `Authorization: Bearer <token>` |
| `FHIRLINT_TX_APIKEY` | `Api-Key: <key>` |
| `FHIRLINT_TX_AUTH` | `Authorization: Basic <base64(user:password)>`, value is `username:password` |

```bash
export FHIRLINT_TX_TOKEN="$ONTOSERVER_TOKEN"
fhirlint validate ./fhir/ --terminology-server https://ontoserver.example.org/fhir
```

```
Authenticating to https://ontoserver.example.org/fhir (bearer token from FHIRLINT_TX_TOKEN)
```

Environment only, deliberately: a flag lands in shell history and CI logs, and a config value lands in the repository. This is the same reasoning as `FHIRLINT_PROXY_AUTH`, which is proxy authentication and unrelated to terminology.

Rules worth knowing:

- **Only an explicitly named server is authenticated.** Without `--terminology-server` the validator talks to the public `tx.fhir.org`, and a credential that happens to be in the environment is not sent there.
- **Never over plain HTTP.** `http://` plus a credential is refused rather than leaked.
- **Setting two variables is an error.** Two credentials means one is stale, and picking a winner silently is how the wrong one gets used for months.
- `fhirlint serve` authenticates the same way, once for the server's lifetime.
- `fhirlint tx warm --terminology-server …` authenticates while recording. The credential is not stored in the recording, so replaying it in CI needs no secret at all — which is the point.

### Offline terminology

`--tx-offline` replays terminology responses recorded earlier, so a validation run needs no terminology server at all. This is what makes a run reproducible: same inputs, same recording, same result, no network.

Record once against the real server:

```bash
fhirlint tx warm ./fhir/
# Recording terminology traffic from https://tx.fhir.org into .fhirlint-tx/ (42 file(s))…
# Recorded 137 terminology interaction(s) (137 new) in .fhirlint-tx/
```

Then replay, offline, as often as you like:

```bash
fhirlint validate ./fhir/ --tx-offline
# Replaying 137 recorded terminology interaction(s) from .fhirlint-tx/
```

Under the hood fhirlint runs a loopback HTTP server that stands in for the terminology server and points the validator at it. Recording proxies to the real server and stores each request and response; replay serves them from disk. Recordings are stored as one pretty-printed JSON file per interaction, so a committed recording stays reviewable in a diff.

**A request that was not recorded is an error, never a silent fallback to the network:**

```
Error: 2 terminology request(s) were not in the recording in .fhirlint-tx/:
  POST /r4/ValueSet/$validate-code (system http://loinc.org, code 8302-2)
  POST /r4/CodeSystem/$validate-code (system http://snomed.info/sct, code 271649006)
Re-record with: fhirlint tx warm ./fhir/
```

This matters more than it looks: the validator downgrades some terminology failures to warnings and carries on, so an incomplete recording could otherwise produce a green run that quietly skipped real terminology checks. fhirlint tracks the misses itself and fails the run regardless of how the validator reacted.

Record with the same profiles and IGs you validate with, since those determine which codes get checked:

```bash
fhirlint tx warm ./fhir/ --profile kbv-patient --ig kbv.basis#1.5.0
fhirlint validate ./fhir/ --profile kbv-patient --ig kbv.basis#1.5.0 --tx-offline
```

Use `--tx-dir` to put the recording somewhere other than `.fhirlint-tx/`. In CI, either commit the recording or rebuild it as an artifact:

```yaml
- name: Validate FHIR resources
  run: fhirlint validate ./fhir/ --tx-offline
```

See the [Offline terminology guide](docs/terminology-offline.md) for the recording format, CI patterns and server mode.

`--tx-offline` cannot be combined with `--no-terminology-server` (which skips terminology instead of replaying it), `--terminology-server` (which it replaces), or `--server` (whose terminology is fixed when the validator server starts).

### Behind a proxy

Two different programs make network calls here, and they read their configuration differently:

| Call | Made by | Reads |
|---|---|---|
| Validator JAR download, `--url` inputs, update check | fhirlint | `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY` |
| Terminology server (`tx.fhir.org`) | the validator JAR | nothing, unless told |

That asymmetry produces a confusing failure on a proxied network: the JAR downloads fine, then validation stalls against the terminology server. `--proxy` and `--https-proxy` close it:

```bash
fhirlint validate ./fhir/ --https-proxy proxy.example.org:3128
```

If unset, both default to the standard environment variables, so exporting them the way every other tool expects is usually enough:

```bash
export HTTPS_PROXY=http://proxy.example.org:3128
fhirlint validate ./fhir/
```

The JAR wants a bare `host:port` while the environment variables are URLs, so the scheme is stripped for you. Credentials embedded in the URL (`http://user:pass@proxy:3128`) are split out and passed separately, as the validator requires.

`NO_PROXY` applies to fhirlint's own requests but **not** to the validator's — it has no equivalent option. On a network where the terminology server is reachable directly but everything else goes through a proxy, set `--tx` to a local terminology server or `--no-tx` instead.

#### Proxy credentials

For a proxy requiring basic auth, use the environment:

```bash
export FHIRLINT_PROXY_AUTH='username:password'
```

There is deliberately no `--proxy-auth` flag and no `fhirlint.yml` key: the first would put the credential in shell history and CI job logs, the second in a file meant to be committed.

Be aware of what this does not fix. The validator takes the credential as a command-line argument, so it is visible in `ps` to other users on the same host for the duration of the run. fhirlint cannot change that. Where you have the choice, a proxy that does not require basic auth avoids the problem entirely.

---

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

`--fhir-version`, `--ig` and `--profile` fall back to `fhirlint.yml`, so a project that already has a config needs no editor-side setup. See the [Language server guide](docs/lsp.md) for Neovim, Helix and VS Code configuration.

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

See the [Server mode guide](docs/daemon.md) for details and CI patterns.

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

`--lock` writes a `fhirlint.lock` file containing SHA256 hashes of all resolved IG packages **and the validator version they were validated against**. On subsequent runs (without `--lock`), fhirlint verifies that cached packages match the recorded hashes and that the validator in use is the recorded one.

```bash
# Generate or update the lock file
fhirlint validate ./fhir/ --lock

# Subsequent runs verify the lock automatically (no flag needed)
fhirlint validate ./fhir/
```

Commit `fhirlint.lock` to version control. This prevents silent package changes from affecting CI results.

Pinning keeps a run reproducible, but it does not tell you whether the pins are still the right ones. [`fhirlint audit`](#auditing-the-toolchain) reads the lock file and reports which packages have moved on upstream.

### Pinning the validator

The IG packages are only half the input. By default fhirlint downloads whatever HL7 published most recently, so a fresh CI runner can pick up a new validator and report different findings from the same sources and the same lock file.

`--validator-version` pins it:

```yaml
# fhirlint.yml
validator-version: "6.9.12"
```

```bash
fhirlint validate ./fhir/ --validator-version 6.9.12
```

The cache holds one JAR at a time, so switching between pinned versions re-downloads it. Move a pin deliberately with:

```bash
fhirlint update --validator-version 6.9.13
```

The lock file records the version in use, and a later run against a different validator fails rather than quietly producing different results:

```
Error: lock file expects validator 6.9.12, running 6.9.13 — pin it with
--validator-version 6.9.12, or run with --lock to accept 6.9.13
```

Lock files written before this existed carry no validator version; those runs warn instead of failing, so nothing breaks on upgrade. `--jar` takes precedence over a pin — you are pointing at a specific file — and warns if that file is a different version than the pin.

### Bounding a run

A pathological or very large input can stall a pipeline or produce a report nobody reads. Two bounds cap it:

```bash
fhirlint validate ./fhir/ --validation-timeout 2m --max-messages 500
```

Both ask the validator to stop and hand back the issues it found so far, so the run still reports something useful. That makes them different from `--timeout`, which kills the Java process outright and yields nothing:

| | Stops | Result |
|---|---|---|
| `--timeout` | the JVM | nothing — the run fails |
| `--validation-timeout` | validation | partial findings, run reported as inconclusive |
| `--max-messages` | validation | partial findings, run reported as inconclusive |

**Hitting a bound fails the run.** This is not caution for its own sake: when the validator stops early it returns only the messages gathered so far, so files with real errors come back with none and are counted as valid. The same input that reports `Files: 2  Valid: 0  Errors: 5` unbounded reports `Files: 2  Valid: 2  Errors: 0` under `--max-messages 1`. Exiting 0 there would turn a bound into a way to make a red pipeline green.

```
Error: validation stopped early because --max-messages was reached, so the results
are partial and files with errors may be reported as valid — raise the bound, or
set --fail-on never to accept partial results
```

Set `--fail-on never` if you genuinely want partial results accepted.

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

### Validating only what changed

On a large repository, validating every resource on every pull request costs minutes to confirm that three files are still fine. `--since` scopes the run to what changed against a git ref:

```bash
fhirlint validate ./fhir/ --since main
fhirlint validate ./fhir/ --since HEAD~1
```

Three sets are validated:

- **Committed changes**, compared as `main...HEAD` — changes since the merge base, i.e. what the branch contributes, not whatever else landed on `main` meanwhile.
- **Uncommitted changes** against `HEAD`, staged or not, so a local run sees the file you are editing.
- **Untracked files** that are not gitignored, so a resource you just created is not silently skipped.

Deleted files are skipped; a renamed file is validated at its new path. Files that did not change are reported and then left alone:

```
Skipped 214 unchanged file(s) (--since main)
```

An empty change set exits 0 and says so rather than passing silently:

```
No changed files to validate (--since main)
```

`--since` needs a git working tree, so it cannot be combined with `--url`, stdin input or `--watch`. An unresolvable ref, or running outside a repository, is an error — quietly validating everything instead would hide a broken CI configuration.

**With `--check-references`,** the unchanged files are still indexed for reference resolution even though they are not validated. Reference integrity is a property of the whole dataset, so a changed resource pointing at an untouched one resolves normally instead of being reported as a dangling reference.

This composes with `--cache` rather than replacing it: `--since` needs no state at all and works on a cold runner, while the result cache also helps for repeated local runs and for content that changed and changed back.

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

### Editor support (JSON Schema)

A JSON Schema for `fhirlint.yml` is published at [`fhirlint.schema.json`](fhirlint.schema.json). Add a modeline to the top of your config and editors with the YAML language server (VS Code, IntelliJ, Neovim) give you key completion, enum values, and inline errors as you type:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/fhirlint/fhirlint/main/fhirlint.schema.json
```

`fhirlint.yml.example` already carries this line. To get the schema locally, or to pin a copy in your own repo:

```bash
fhirlint config schema > fhirlint.schema.json
```

The schema is **generated from the same key definitions `config check` validates against**, so the two cannot disagree about which keys exist or which enum values are valid. A test fails the build if the committed schema drifts from those definitions.

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
| `--since` | — | Validate only files changed against this git ref, e.g. `main` |
| `--bundle-entries` | `false` | Also validate each `entry.resource` in a FHIR Bundle separately |
| `--url` | — | Fetch and validate from an HTTP endpoint (repeatable) |
| `--url-timeout` | `30s` | Timeout for HTTP fetches via `--url` |
| `--extract` | — | JSONPath to extract from input before validating |
| `--extract-each` | — | JSONPath to an array — validates each element separately |
| `--ignore` | — | JSONPath field to remove before validating (repeatable) |
| `--suppress` | — | Silence a known issue: `messageId:X`, `constraint:X`, or `expression:X` (repeatable) |
| `--show-suppressed` | `false` | Show suppressed issues with a muted `↷ SUPP` label |
| `--require-suppress-reason` | `false` | Fail when a suppression rule has no `reason` |
| `--baseline` | — | Baseline file — only new issues (regressions) fail the build |
| `--generate-baseline` | — | Generate a baseline file from current issues |
| `--no-terminology-server` | `false` | Disable terminology server — no data sent to `tx.fhir.org` |
| `--terminology-server` | — | Custom terminology server URL |
| `--tx-cache` | — | Terminology cache directory (`n/a` to disable) |
| `--tx-offline` | `false` | Replay recorded terminology responses; an unrecorded request is an error |
| `--tx-dir` | `.fhirlint-tx/` | Directory holding the terminology recording |
| `--allow-insecure-tx` | `false` | Suppress warning when terminology server uses HTTP |
| `--tx-log` | — | Write terminology request log to file |
| `--locale` | — | Locale for validation messages, e.g. `de`, `fr` |
| `--allow-example-urls` | `false` | Suppress warnings about `example.org` placeholder URLs |
| `--jurisdiction` | — | Jurisdiction for country-specific bindings, e.g. `urn:iso:std:iso:3166#DE` |
| `--display-issues-are-warnings` | `false` | Downgrade coded-display mismatch errors to warnings |
| `--po` | — | Load message translations from a `.po` file at runtime (repeatable) |
| `--best-practice` | — | Best-practice constraint handling: `ignore`, `hint`, `warning`, `error` |
| `--timeout` | `5m` | Timeout for the Java validator process |
| `--validation-timeout` | — | Stop validating after this long and report partial results (e.g. `90s`, `2m`) |
| `--max-messages` | `0` | Stop after this many validation messages and report partial results (`0` = unbounded) |
| `--offline` | `false` | Forbid all network access: cached JAR, cached IG packages, and the validator's own HTTP blocked |
| `--codesystem-size-limit` | `-1` | Max codes checked per ValueSet include, ConceptMap group or CodeSystem supplement (`0` = no limit, `-1` = the validator's own default of 1000) |
| `--proxy` | `$HTTP_PROXY` | HTTP proxy for the validator's terminology calls, `host:port` |
| `--https-proxy` | `$HTTPS_PROXY` | HTTPS proxy for the validator's terminology calls, `host:port` |
| `--cache` | `false` | Cache validation results per file hash |
| `--cache-dir` | — | Directory for result cache (default: `~/.fhirlint/result-cache/`) |
| `--lock` | `false` | Write/update `fhirlint.lock` with IG package SHA256 hashes |
| `--quiet`, `-q` | `false` | Suppress per-file output for valid files (terminal format only) |
| `--group` | `false` | Group repeated findings into one block each with a count (terminal format only) |
| `--redact` | `false` | Remove message text and source lines from all reports (see [PHI-safe reports](#phi-safe-reports)) |
| `--no-color` | `false` | Disable ANSI color output |
| `--watch` | — | Watch mode: `single` (changed files only) or `all` (all files on any change) |
| `--watch-interval` | — | Polling interval for `--watch` in milliseconds |
| `--jar` | — | Path to a local validator JAR (overrides auto-download; also via `FHIRLINT_JAR`) |
| `--validator-version` | latest | Pin the auto-downloaded validator to an upstream release, e.g. `6.9.12` (also via `FHIRLINT_VALIDATOR_VERSION`) |

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
| `since` | string | `--since` |
| `tx-offline` | bool | `--tx-offline` |
| `tx-dir` | string | `--tx-dir` |
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
| `group` | bool | `--group` |
| `redact` | bool | `--redact` |
| `no-color` | bool | `--no-color` |
| `watch` | string | `--watch` |
| `watch-interval` | int | `--watch-interval` |

---

## Built-in profile aliases

Aliases are a convenience shortcut for common IG packages. Pass the full `name#version` reference directly to target a different version.

**German profiles** (see the [German profiles guide](docs/german-profiles.md)):

| Alias | Resolves to |
|-------|-------------|
| `kbv-basis` | `kbv.basis#1.9.0` |
| `kbv-patient` | `kbv.basis#1.9.0` |
| `mii` | all six MII Kerndatensatz modules (see below) |
| `mii-person` | `de.medizininformatikinitiative.kerndatensatz.person#2025.0.1` |
| `mii-fall` | `de.medizininformatikinitiative.kerndatensatz.fall#2025.0.1` |
| `mii-diagnose` | `de.medizininformatikinitiative.kerndatensatz.diagnose#2025.0.1` |
| `mii-prozedur` | `de.medizininformatikinitiative.kerndatensatz.prozedur#2025.0.1` |
| `mii-laborbefund` | `de.medizininformatikinitiative.kerndatensatz.laborbefund#2026.0.3` |
| `mii-medikation` | `de.medizininformatikinitiative.kerndatensatz.medikation#2026.0.1` |
| `diga` | `kbv.mio.diga#1.1.0` |

An alias can stand for more than one package. The MII publishes no umbrella
package — the Kerndatensatz ships module by module — so `mii` loads all six
modules, and the `mii-*` aliases name them individually. The modules are not on
a common release train, so `mii` pulls two versions of the shared
`kerndatensatz.meta` package; if that matters for your project, name the modules
you actually need instead.

**International profiles:**

| Alias | Resolves to | Covers |
|-------|-------------|--------|
| `us-core` | `hl7.fhir.us.core#9.0.0` | US Core (HL7 US Realm) |
| `ips` | `hl7.fhir.uv.ips#2.0.1` | International Patient Summary |
| `ipa` | `hl7.fhir.uv.ipa#1.1.0` | International Patient Access |
| `uk-core` | `fhir.r4.ukcore.stu2#2.0.2` | UK Core (NHS England, STU2) |

```bash
# List all available aliases
fhirlint profiles

# Validate against an international profile alias
fhirlint validate patient.json --profile us-core
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

### Moving the cache

`FHIRLINT_CACHE_DIR` relocates everything fhirlint caches — the JAR, the version and verification-status files, and the `--cache` result cache:

```bash
export FHIRLINT_CACHE_DIR=/mnt/build-cache/fhirlint
```

Useful for containers whose home directory is read-only, CI runners that cache a mounted volume between jobs, and working on two projects pinned to different validator versions without re-downloading on every switch.

`--cache-dir` still overrides the result-cache location on its own, and takes precedence over `FHIRLINT_CACHE_DIR` for that one directory.

---

## Auditing the toolchain

Two things decide what a validation run reports: the validator JAR and the IG packages. `fhirlint audit` checks both.

```bash
fhirlint audit
```

```
Validator JAR
  current:  6.10.1
  latest:   6.10.2
  ✗ outdated — run: fhirlint update

Security advisories (hapifhir/org.hl7.fhir.core)
  ✓ 15 advisory/advisories published, none affect your version (6.10.1)

IG packages (fhirlint.lock)
  ✓ de.gematik.isik-basismodul  4.0.3 — current
  ✗ hl7.fhir.de.core            1.0.0 — not found in the registry
  ✓ hl7.fhir.r4.core            4.0.1 — current
  ✗ kbv.basis                   1.4.0 → 1.9.0 available
  (2 of 4 package(s) need attention)
```

The JAR half checks the installed version against the latest release and against the published [security advisories](https://github.com/hapifhir/org.hl7.fhir.core/security/advisories) for `org.hl7.fhir.core`.

The IG half reads [`fhirlint.lock`](#ig-lock-file) and asks the FHIR package registry about each pinned package. There is nothing to configure: the lock file already knows which packages you use. Without a lock file the section is skipped with a hint, and the JAR check still runs.

A package is reported as:

| | meaning |
|---|---|
| `→ X available` | a newer version exists upstream |
| `not found in the registry` | the pin no longer resolves and will fail on a cold cache |
| `deprecated upstream` | the publisher marked this version deprecated |
| `registry latest is X (versions not comparable)` | the versions differ but could not be ordered |
| `ahead of registry latest` | you pinned a version newer than the registry's `latest`, e.g. a pre-release |

The last two are worth explaining. FHIR IG versions are usually semver, but the registry does not enforce it, so fhirlint only calls a package *outdated* when it could actually establish that your pin is older. When a publisher switches versioning scheme, you are told the versions differ rather than being given a confident but invented direction.

### Exit codes

| code | meaning |
|---|---|
| `0` | nothing to report |
| `1` | the JAR is outdated or missing, or an advisory affects the installed version |
| `2` | only IG package findings |

An outdated IG package is a maintenance signal and an advisory against the JAR is a security one, so they get separate codes. `1` keeps the meaning it always had, and a JAR problem takes precedence when both are present.

`--format json` prints the whole report and always exits `0`, so a pipeline can decide for itself:

```bash
fhirlint audit --format json | jq '.igPackages[] | select(.outdated)'
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
- [Offline terminology](docs/terminology-offline.md) — recording and replaying terminology so a run needs no terminology server
- [Language server](docs/lsp.md) — editor setup, hover, quick fixes, and how diagnostics differ from a CLI run

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for setup instructions, the PR checklist, and how to add new flags.

## Security

What fhirlint treats as trusted and untrusted, what counts as a vulnerability, and how to report one: [SECURITY.md](SECURITY.md).

## License

Apache 2.0 — see [LICENSE](LICENSE).

---

HL7® and FHIR® are registered trademarks of Health Level Seven International.
