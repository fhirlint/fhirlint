# Architecture

fhirlint is a single Go binary that wraps the official HL7 FHIR Validator, a Java program (the JAR). fhirlint finds the inputs and prepares them, runs the JAR, and adds its own checks, suppressions, baselines and reports on top. It has no server component of its own. Its state is the cached JAR, an optional result cache and terminology recordings the user asks for. This document describes how a run works, who and what takes part in building and shipping fhirlint, and what fhirlint defends against.

## A run

```text
args, fhirlint.yml ─▶ resolve inputs ─▶ preprocess ─▶ prepare the validator ─▶ result cache ─▶ run the JAR
                                                                                                 │
   exit code ◀─ reports ◀─ redact ◀─ baseline ◀─ suppressions ◀─ severity overrides ◀─ own checks ◀─ parse output
```

1. **Configuration** (`cmd`). Flags win over `fhirlint.yml`, which wins over the defaults. Per-file `overrides:` and a `profile-map` pick options per input. `.fhirlintignore` adds exclude patterns. `--no-repo-config` ignores the `fhirlint.yml` and `.fhirlintignore` found in the checked directory.
2. **Resolve inputs** (`internal/input`, `cmd/validate.go`). Arguments become a list of files: paths, directory walks, stdin, `--url` downloads, NDJSON files, or the files `git` reports as changed for `--since`. `--exclude` patterns drop files here, and so does `--skip-non-fhir` for files that don't look like FHIR. A symbolic link inside a walked directory that leads outside it is skipped with a warning.
3. **Preprocess** (`cmd/validate.go`, `internal/ndjson`). `--extract` and `--extract-each` pull resources out of an envelope, `--bundle-entries` splits a Bundle, and `--ignore` deletes paths, in JSON and in XML. All of this works on temporary copies. The input files are never changed.
4. **Prepare the validator** (`internal/validator`, `internal/profiles`, `internal/localig`, `internal/txreplay`, `internal/txauth`):
   - **Java:** a usable Java 17 or later must be present.
   - **The JAR:** it is downloaded from GitHub Releases on first use, unless `--jar` names one. A download is verified before it is used (see [SECURITY.md](SECURITY.md#properties-you-can-rely-on)).
   - **Profiles:** built-in aliases such as `kbv.basis` or `isik` become IG packages.
   - **Local terminology:** `--codesystem` and `--valueset` files are bundled into a temporary IG.
   - **The terminology server:** the JAR uses tx.fhir.org, the server `--terminology-server` names, or none. With `--tx-offline`, a local replay server answers from a recording.
5. **Result cache** (`internal/resultcache`). With `--cache`, an input whose content hash and validator options match an earlier run gets that run's result, without starting the JAR.
6. **Run the JAR** (`internal/validator`). `java` runs as a child process with its arguments as a list, never through a shell. It writes its result as JSON to a temporary file. `--timeout` and `--validation-timeout` bound the run. Inputs with different options run in separate groups. With `--server`, the inputs go to a warm validator that `fhirlint serve` keeps running on `localhost` ([docs/daemon.md](docs/daemon.md)).
7. **Parse output** (`internal/validator`). The JAR's OperationOutcome becomes findings with file, location, severity, message and HL7 message id. A JAR that crashed, timed out or wrote nothing is reported as such, never as a clean result.
8. **Own checks** (`internal/lint`, `internal/rules`, `internal/refcheck`). Custom FHIRPath rules and the built-in lint rules run on each input. With `--check-references`, the reference check runs across the whole set. Their findings take the same path as the JAR's.
9. **Severity overrides, suppressions, baseline** (`internal/suppress`, `internal/baseline`, `internal/igbaseline`). Severity overrides are applied first, so every later stage sees the same severity. Next come suppression rules from flags and `fhirlint.yml`, which can carry a reason and an expiry date. Then come the per-file overrides, and then `--baseline`, which moves known findings aside. Suppressed findings are kept and counted, and `--show-suppressed` shows them.
10. **Redact** (`internal/redact`). With `--redact`, content quoted from the resources is removed from the findings before anything renders them.
11. **Reports** (`internal/reporter`). Terminal, JSON, HTML, JUnit, SARIF, Code Climate, GitHub annotations and Markdown, to stdout or files. Terminal output goes through `internal/termsafe`, which makes control characters from the input visible instead of passing them to the terminal.
12. **Exit code** (`cmd`). `0` passed, `1` for findings at or above `--fail-on` (or over `--max-warnings`), `2` when fhirlint itself failed. With `--lock`, the run also checks or writes `fhirlint.lock`, the IG packages it resolved.

### Interfaces

fhirlint's external interfaces:

- **The CLI and its configuration:** the command line, `fhirlint.yml` (schema: [`fhirlint.schema.json`](fhirlint.schema.json)), `.fhirlintignore`, `fhirlint.lock` and the exit codes, all documented in the [README](README.md).
- **Reports:** the JSON report (schema: [`fhirlint-report.schema.json`](fhirlint-report.schema.json)) and the other report formats.
- **Long-running modes:** the language server `fhirlint lsp` over stdio ([docs/lsp.md](docs/lsp.md)), the warm validator `fhirlint serve` ([docs/daemon.md](docs/daemon.md)) and `--watch`.
- **The Go library:** [`pkg/fhirlint`](https://pkg.go.dev/github.com/fhirlint/fhirlint/pkg/fhirlint).
- **Subcommands:** among them `audit`, `update`, `cache`, `tx`, `coverage`, `qualify`, `diff`, `explain` and `upgrade-check`.

Integrations wrap the same binary: the container image, the GitHub Action ([`action.yml`](action.yml)), the GitLab CI/CD component ([docs/ci.md](docs/ci.md)), and the Claude Code plugin with its hooks and skills. fhirlint is distributed as release archives, the image `ghcr.io/fhirlint/fhirlint`, a Homebrew cask and `go install`.

The network endpoints:

- **fhirlint itself:**
  - GitHub, to download the JAR and its signature, check for updates and read advisories;
  - the FHIR package registry (packages.fhir.org), for `audit`, `coverage` and `--lock`;
  - the URLs given to `--url`.
- **The JAR:** the terminology server and the package registry.

`--offline` and `--tx-offline` keep the run off the network as far as the JAR allows.

## Building and shipping

| Actor | Role |
|---|---|
| Maintainer | Merges pull requests into `main` after review, sets `sensitive-paths-reviewed` after reviewing a sensitive change, and pushes release tags ([GOVERNANCE.md](GOVERNANCE.md)) |
| GitHub Actions | Runs CI on every pull request and push ([TESTING.md](TESTING.md)), builds releases and images |
| `release.yml` | Builds the archives with GoReleaser, after `require-green-ci.yml` confirms that CI passed for the tagged commit and a smoke test of the archives. It attests the archives, SBOMs and `checksums.txt` with signed SLSA provenance, attaches the attestation bundle to the release, and publishes the Homebrew cask |
| `docker.yml` | Builds the image, attests its provenance and SBOM, and signs it with cosign |
| Sigstore (Fulcio, Rekor) | Issues short-lived signing certificates and records signatures in a transparency log |
| GitHub Releases, `ghcr.io/fhirlint/fhirlint`, `fhirlint/homebrew-tap` | Distribute archives, checksums, SBOMs, attestation bundles, the image and the cask |
| GitLab (`gitlab.com/fhirlint/fhirlint`) | A read-only mirror of `main` and the tags, which hosts the CI/CD Catalog component; `gitlab-mirror.yml` pushes to it |
| HL7 (`hapifhir/org.hl7.fhir.core`) | Publishes the validator JAR that fhirlint downloads at runtime, signed with HL7's release key, which fhirlint pins |
| Dependabot | Proposes updates as pull requests: Go modules, actions, base images and CI tools, after a 7-day cooldown; security updates at once |

A release starts when the maintainer pushes a `vX.Y.Z` tag. Release builds are reproducible: `reproducible-build.yml` builds the archives twice in different directories and requires them to be byte-identical, and the release job checks that the published archives equal the smoke-tested ones.

The Claude Code plugin is installed from this repository's marketplace (`.claude-plugin/marketplace.json`), with the repository root as its source. Plugin users therefore run the hooks and skills as they are on `main`, which is why those paths sit behind the sensitive-paths gate.

## Threat model

fhirlint reads resources that it has no reason to trust: data from an API or a partner, or the files in a fork's pull request. It runs in CI with whatever access that job has, downloads and runs a third-party Java program, and is itself a supply-chain dependency of its users. The [trust boundary in SECURITY.md](SECURITY.md#trust-boundary) lists what counts as a vulnerability. The most likely and most damaging problems, and how fhirlint counters them:

| Threat | Impact | Countermeasures |
|---|---|---|
| A tampered validator JAR | Code execution in every run, on every machine | The JAR's detached PGP signature is checked against HL7's release key, which is pinned in the repository. The GitHub release digest is used only when no signature exists, and it is recorded as the weaker check. A failed check discards the download. `fhirlint version` shows how the cached JAR was verified. `jar-monitor.yml` watches the key's expiry and published advisories |
| A crafted resource makes fhirlint misbehave: crash in a way that hides findings, hang, or exhaust memory | A green CI run that checked nothing, or a job that never finishes | A JAR that crashes, times out or writes nothing is reported as a failure, never as valid. `--timeout` and `--validation-timeout` bound the JAR. fhirlint's own parsers of input it didn't write (the JAR output, XML preprocessing, message catalogs, NDJSON, FHIRPath output, LSP messages and more) are fuzzed on every change and weekly |
| A crafted repository makes fhirlint read files outside it | Contents of other files on the runner in the job log, through parse errors and `--show-source` | A symbolic link inside a validated directory that leads outside it is skipped and named. A discovered `fhirlint.yml` or `.fhirlintignore` that links out is refused |
| A crafted repository changes how fhirlint runs | A fork's `fhirlint.yml` turns checks off, suppresses findings or points the cache at its own entries | `--no-repo-config` ignores the checked directory's `fhirlint.yml` and `.fhirlintignore`. A result cache inside the working directory, or owned by another user, is refused when it comes from `fhirlint.yml` or is the default ([docs/ci.md](docs/ci.md#limits-for-untrusted-pull-requests)) |
| A crafted repository makes fhirlint execute something | Code execution in the user's CI job or on a reviewer's machine | fhirlint never runs a shell. It starts `java`, `git` for `--since`, and itself for `upgrade-check`, each with an argument list. Arguments that fhirlint's output parsing depends on are rejected in `--validator-arg` |
| Output that breaks out of its format and injects into the tool that reads it, or text that forges lines in a terminal or job log | Misleading content in dashboards, hidden findings, or a fake "✓ Valid" in a log | JSON, SARIF and Code Climate are written with `encoding/json`, JUnit with `encoding/xml`, and HTML with `html/template`, which all escape their values. GitHub workflow commands are escaped. Terminal output makes control characters visible (`internal/termsafe`) |
| Patient data in a report | Health data in job logs, artifacts and code scanning, which are often more widely readable than the data | `--redact` removes content quoted from the resources before any report is written. Terminology requests can be replayed offline with `--tx-offline`, so that no codes leave the machine |
| A suppression, baseline or exit code that hides more than intended | Real findings silently dropped; a build that passes when it should fail | Suppressions can be required to carry a reason (`require-suppress-reason`) and can expire. Suppressed findings are counted and shown with `--show-suppressed`. Stale baseline entries are reported. The exit code follows the findings after severity overrides, which are applied before every other stage |
| A cached result reused for the wrong question | A finding that the current options would report is missing | The cache key covers the input's content and all validator options, minus a reviewed denylist. A test fails until every new option has been considered for the key |
| A false negative of fhirlint's own checks | Users trust data that is wrong | Unit tests for each rule, the corpus scan over about 13,000 real resources with a must-stay-reported list ([docs/corpus-scan.md](docs/corpus-scan.md)), and the integration tests against the real JAR |
| A compromised dependency or build step | A backdoored binary or image reaches users | Actions pinned by SHA, checked by pinact. Base images pinned by digest. Python CI tools hash-pinned. Dependabot with a cooldown. govulncheck, OSV-Scanner and dependency review with a license allowlist. zizmor, glsec and cfgaudit on the workflows. The sensitive-paths gate. Signed SLSA provenance and reproducible builds. Trivy, hadolint and dockle on the image |
| A compromised maintainer account | Malicious commits or releases | Two-factor authentication with secure methods only, required for the organization. `main` is protected for admins too: a pull request, seven required checks and a DCO sign-off. Releases are built only from tags whose CI passed |

Out of scope:

- an attacker who controls the machine fhirlint runs on;
- gaps in FHIR validation itself, which belong to the HL7 validator;
- `--validator-arg`, which passes arguments to the JAR unchecked by design.

## Assurance case

The claim: fhirlint can validate resources written by an attacker, inside the attacker's pull request pipeline, without being made to do anything but report findings about them, and without reporting success when it didn't check. The argument has four parts.

**1. The threats are known.** The threat model above lists what an attacker can try against fhirlint and against its supply chain. [SECURITY.md](SECURITY.md#trust-boundary) draws the trust boundary:

- **Untrusted:** the resources, terminology server responses, anything fetched with `--url`, and the JAR until it is verified. When the checked directory belongs to someone else, its `fhirlint.yml` and `.fhirlintignore` are untrusted too.
- **Trusted:** the fhirlint binary, the configuration that the user who runs fhirlint passes, and the verified JAR.

**2. The design follows secure-design principles** ([Saltzer and Schroeder](https://doi.org/10.1109/PROC.1975.9939)):

| Principle | How fhirlint applies it |
|---|---|
| Economy of mechanism | One static binary, and the validation itself is left to the official HL7 validator instead of being reimplemented. The whole run is the pipeline in [A run](#a-run) |
| Fail-safe defaults | A JAR that crashes, times out or writes no output fails the run instead of passing it. So does a terminology replay that misses, or a JAR that fails verification. Links out of a validated directory are skipped. Remote content is fetched only for `--url` |
| Complete mediation | Findings from the JAR, from the cache and from fhirlint's own checks pass through the same severity overrides, suppressions, baseline, redaction and exit-code logic. No path reaches a report around them |
| Least privilege | fhirlint writes only to temporary files, the cache, and the reports the user asks for, and it never changes the inputs. The container image runs as a non-root user. Each workflow job gets only the token permissions it needs |
| Least common mechanism | The JAR runs as a separate process with its arguments as a list. Its output comes back through a temporary file, not mixed into its log |
| Open design | Source, threat model and build are public. Releases are reproducible and carry signed provenance, so anyone can check that a binary comes from the tagged source |
| Psychological acceptability | Safe behaviour needs no configuration: the JAR is verified and control characters are escaped by default. Suppressions are visible in `fhirlint.yml`, can expire, and can be required to carry a reason |

**3. Common weaknesses are countered** where they apply to a tool like fhirlint:

| Weakness | Countermeasure |
|---|---|
| [CWE-22](https://cwe.mitre.org/data/definitions/22.html) / [CWE-59](https://cwe.mitre.org/data/definitions/59.html) Path traversal, link following | Symbolic links in validated directories that lead outside them are skipped, and so are discovered `fhirlint.yml` and `.fhirlintignore` files that link out. The result cache never reads or writes through a link. Tests cover links that stay inside and links that leave |
| [CWE-78](https://cwe.mitre.org/data/definitions/78.html) OS command injection | No shell. `java`, `git` and fhirlint's own binary get their arguments as a list |
| [CWE-116](https://cwe.mitre.org/data/definitions/116.html) / [CWE-117](https://cwe.mitre.org/data/definitions/117.html) Improper output encoding, log injection | Machine-readable formats are written with Go's encoders and `html/template`. GitHub workflow commands are escaped. Terminal output escapes control characters from the input (#496) |
| [CWE-200](https://cwe.mitre.org/data/definitions/200.html) Exposure of sensitive information | `--redact` for reports, `--tx-offline` for terminology traffic, and private reporting of vulnerabilities |
| [CWE-377](https://cwe.mitre.org/data/definitions/377.html) Insecure temporary files | Temporary files and directories are created with `os.CreateTemp` and `os.MkdirTemp` (unique names, owner-only permissions) and removed after use |
| [CWE-494](https://cwe.mitre.org/data/definitions/494.html) Download of code without an integrity check | The JAR is verified against HL7's pinned PGP key before use. fhirlint's own releases carry signed provenance and can be rebuilt ([README](README.md#pre-built-binary-recommended)) |
| [CWE-1333](https://cwe.mitre.org/data/definitions/1333.html) Inefficient regular expressions | Go's RE2-based `regexp`, which runs in linear time |
| [CWE-1395](https://cwe.mitre.org/data/definitions/1395.html) Vulnerable third-party components | govulncheck, OSV-Scanner and dependency review run on every change. govulncheck is a required check. Dependabot keeps dependencies current. `fhirlint audit` reports advisories against the installed JAR |

**4. The evidence is checked continuously:**

- **Unit tests** cover every package without Java. The integration tests run against the real JAR.
- **Fuzzing:** the parsers of input fhirlint didn't write are fuzzed on every change and weekly.
- **Required checks:** golangci-lint with gosec, govulncheck, cfgaudit, glsec, the sensitive-paths gate and DCO are required on every pull request. zizmor checks the workflows, and Trivy, hadolint and dockle check the image.
- **False positives:** the corpus scan compares the findings of fhirlint's own rules between two builds before each release.
- **Supply chain:** OpenSSF Scorecard measures it weekly ([docs/osps-baseline.md](docs/osps-baseline.md)).
