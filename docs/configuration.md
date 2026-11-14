# Configuration

`fhirlint.yml`, checking it, and the reference of every flag and key.

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

A fully annotated example is provided in [`fhirlint.yml.example`](../fhirlint.yml.example), including documentation for `overrides:` (per-glob IGs, profiles, severity, and suppress rules) and `profile-map:` (automatically apply profiles based on resource type or filename glob).

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

A JSON Schema for `fhirlint.yml` is published at [`fhirlint.schema.json`](../fhirlint.schema.json). Add a modeline to the top of your config and editors with the YAML language server (VS Code, IntelliJ, Neovim) give you key completion, enum values, and inline errors as you type:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/fhirlint/fhirlint/main/fhirlint.schema.json
```

`fhirlint.yml.example` already carries this line. To get the schema locally, or to pin a copy in your own repo:

```bash
fhirlint config schema > fhirlint.schema.json
```

The schema is **generated from the same key definitions `config check` validates against**, so the two cannot disagree about which keys exist or which enum values are valid. A test fails the build if the committed schema drifts from those definitions.

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
| `--package-baseline` | `false` | Trust an IG package's own QA baseline: resources it already records errors for do not fail the run |
| `--no-terminology-server` | `false` | Disable terminology server — no data sent to `tx.fhir.org` |
| `--terminology-server` | — | Custom terminology server URL |
| `--tx-cache` | — | Terminology cache directory (`n/a` to disable) |
| `--tx-offline` | `false` | Replay recorded terminology responses; an unrecorded request is an error |
| `--tx-dir` | `.fhirlint-tx/` | Directory holding the terminology recording |
| `--allow-insecure-tx` | `false` | Use a plain-HTTP terminology server: exempts that one URL from the validator's SSRF protection and drops the warning |
| `--tx-log` | — | Write terminology request log to file |
| `--expansion-parameters` | — | `Parameters` resource pinning code system and value set versions |
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
| `--slowest` | `0` | After the summary, list the N files the validator took longest on (terminal format only) |
| `--redact` | `false` | Remove message text and source lines from all reports (see [PHI-safe reports](usage.md#phi-safe-reports)) |
| `--no-color` | `false` | Disable ANSI color output |
| `--watch` | — | Watch mode: `single` (changed files only) or `all` (all files on any change) |
| `--watch-interval` | — | How often `--watch` looks for changes, in milliseconds (default: 1000) |
| `--watch-settle-time` | — | How long `--watch` waits after a change before re-validating, in milliseconds (default: 100) |
| `--check-references` | `false` | Check that references resolve within the validated resource set. See [Referential integrity](checks.md#referential-integrity) |
| `--rules-file` | — | Load custom FHIRPath lint rules from a YAML file (overrides `rules:` in the config). See [Custom lint rules](checks.md#custom-lint-rules) |
| `--server` | — | Validate through a running validator server instead of a per-run JVM, e.g. `http://localhost:8080`. See [Server mode](usage.md#server-mode-warm-validator) |
| `--show-source` | `false` | Show the offending source line under each finding (terminal output only). See [Showing the offending line](usage.md#showing-the-offending-line) |
| `--skip-non-fhir` | `false` | Skip files that are not FHIR resources instead of reporting them as errors. See [Skipping non-FHIR files](usage.md#skipping-non-fhir-files) |
| `--validator-arg` | — | Extra argument passed straight to the validator JAR (repeatable, unvalidated). See [Passing raw validator arguments](usage.md#passing-raw-validator-arguments) |
| `--no-repo-config` | `false` | Ignore a `fhirlint.yml` and `.fhirlintignore` found in the checked directory; only `--config` is read. See [Limits for untrusted pull requests](ci.md#limits-for-untrusted-pull-requests) |
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
| `package-baseline` | bool | `--package-baseline` |
| `no-terminology-server` | bool | `--no-terminology-server` |
| `terminology-server` | string | `--terminology-server` |
| `tx-cache` | string | `--tx-cache` |
| `allow-insecure-tx` | bool | `--allow-insecure-tx` |
| `tx-log` | string | `--tx-log` |
| `expansion-parameters` | string | `--expansion-parameters` |
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
| `slowest` | int | `--slowest` |
| `redact` | bool | `--redact` |
| `no-color` | bool | `--no-color` |
| `watch` | string | `--watch` |
| `watch-interval` | int | `--watch-interval` |
| `watch-settle-time` | int | `--watch-settle-time` |
