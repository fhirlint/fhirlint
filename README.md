<p align="center">
  <img src="assets/logo.png" alt="fhirlint" width="180"/>
</p>

# fhirlint

[![Go Reference](https://pkg.go.dev/badge/github.com/fhirlint/fhirlint/pkg/fhirlint.svg)](https://pkg.go.dev/github.com/fhirlint/fhirlint/pkg/fhirlint)
[![Tests](https://github.com/fhirlint/fhirlint/actions/workflows/test.yml/badge.svg)](https://github.com/fhirlint/fhirlint/actions/workflows/test.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/fhirlint/fhirlint/badge)](https://scorecard.dev/viewer/?uri=github.com/fhirlint/fhirlint)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/16686/badge)](https://www.bestpractices.dev/projects/16686)
[![OpenSSF Baseline](https://www.bestpractices.dev/projects/16686/baseline)](https://www.bestpractices.dev/projects/16686)
[![SLSA Build Level 3](https://slsa.dev/images/gh-badge-level3.svg)](docs/verifying-releases.md)
[![agent security: cfgaudit](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/cfgaudit/cfgaudit/main/assets/badge/v1.json)](https://github.com/cfgaudit/cfgaudit)
[![CI/CD security: glsec](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/glsec/glsec/main/assets/badge/v1.json)](https://github.com/glsec/glsec)

**fhirlint** validates FHIR resources with the official [HL7 FHIR Validator](https://github.com/hapifhir/org.hl7.fhir.core) and adds what the validator lacks:

- readable terminal output and reports for CI (JSON, HTML, JUnit, SARIF, Markdown, Code Climate, GitHub annotations);
- pipeline-ready exit codes;
- suppressions with reasons and expiry dates, and baselines;
- watch mode and a language server;
- built-in aliases for German FHIR profiles (KBV, MII, ISiK, DiGA).

The validator JAR is downloaded and verified on first use.

```console
$ fhirlint validate --show-source patient.json
▶ patient.json
  ✗ ERROR  Not a valid date format: '1990-13-45'
           @ Patient.birthDate (line 4, col 28)
           4 │   "birthDate": "1990-13-45"
             │                            ^
  ⚠ WARN   Constraint failed: dom-6: 'A resource should have narrative for robust management' (defined in http://hl7.org/fhir/StructureDefinition/DomainResource) (Best Practice Recommendation)
           @ Patient (line 1, col 2)
           1 │ {
             │  ^

────────────────────────────────────────
Files: 1  Valid: 0  Errors: 1  Warnings: 1
```

**Requirements:** Java 17+, except for the container image and the Homebrew cask, which bring a JRE.

## Quick start

In a GitHub Actions workflow:

```yaml
- uses: fhirlint/fhirlint@v1.17.0
  with:
    path: ./fhir/
    fail-on: error
```

In GitLab CI, with the [CI/CD Catalog component](https://gitlab.com/explore/catalog/fhirlint/fhirlint):

```yaml
include:
  - component: gitlab.com/fhirlint/fhirlint/fhirlint@v1.17.0
    inputs:
      path: ./fhir/
```

On your machine:

```bash
brew install fhirlint/tap/fhirlint
fhirlint validate ./fhir/
fhirlint validate --profile kbv-basis ./fhir/   # with the KBV base profiles
```

## Why fhirlint?

The HL7 validator is the reference for FHIR validation, and fhirlint doesn't replace it: every finding comes from the validator. fhirlint makes it practical to run on every commit:

| | HL7 validator JAR | fhirlint |
|---|---|---|
| Output | Console log and an OperationOutcome | Terminal output with source lines and grouping; JSON, HTML, JUnit, SARIF, Markdown, Code Climate, GitHub annotations |
| Failing a build | Non-zero on errors | `--fail-on` and `--max-warnings` thresholds, per-file overrides |
| Known issues | Fix them or live with them | Suppressions with reason and expiry, severity overrides, baselines |
| German profiles | Package names and versions by hand | Aliases for KBV, MII, ISiK and DiGA |
| Repeated runs | JVM start and package loading each time | Result cache, `--since`, a warm validator server |
| Reproducibility | JAR version and terminology are yours to manage | Pinned and verified JAR, `fhirlint.lock`, recorded terminology for offline runs |
| Own checks | — | FHIRPath rules, style rules, a reference check across the whole set |

## Install

```bash
brew install fhirlint/tap/fhirlint                  # Homebrew, brings a JRE
go install github.com/fhirlint/fhirlint@latest      # Go
docker run --rm -v "$PWD:/work" ghcr.io/fhirlint/fhirlint:1.17.0 validate /work/fhir/
```

Pre-built binaries for Linux, macOS and Windows are on the [releases page](https://github.com/fhirlint/fhirlint/releases). Every way to install is described in [Installing fhirlint](docs/install.md), and how to check a download before you run it is in [Verifying a release](docs/verifying-releases.md).

## Usage

```bash
fhirlint validate patient.json                         # one file; exit 1 on errors
fhirlint validate ./fhir/ --format sarif -o out.sarif  # a directory, as a SARIF report
fhirlint validate ./fhir/ --ig kbv.basis#1.5.0         # against an implementation guide
fhirlint validate ./fhir/ --since main                 # only what changed against main
fhirlint validate ./fhir/ --generate-baseline base.json && fhirlint validate ./fhir/ --baseline base.json
fhirlint explain dom-6                                 # what a FHIR invariant means and how to fix it
```

The full documentation is in [docs/](docs/):

| Page | What it covers |
|---|---|
| [Using fhirlint](docs/usage.md) | Input sources, profiles and the built-in aliases, output formats, preprocessing, `explain`, FHIRPath, watch mode, the warm validator server, the language server |
| [Suppressions and own checks](docs/checks.md) | Suppressing known issues, severity overrides, custom FHIRPath rules, style rules, the reference check, baselines |
| [Terminology](docs/terminology.md) | The terminology server, what is and isn't checked, air-gapped and offline runs, authentication, proxies |
| [Pipeline integration](docs/pipeline.md) | The lock file, pinning the validator, bounding a run, crashes, the result cache, validating only what changed |
| [Analysing datasets and profiles](docs/analysis.md) | `stats`, `coverage`, comparing runs and profiles, the Computer System Validation report (`qualify`) |
| [Configuration](docs/configuration.md) | `fhirlint.yml`, `config check`, and the reference of every flag and key |
| [The toolchain](docs/toolchain.md) | The FHIR package cache, the validator JAR, update notices, `audit` and its exit codes |
| [Go library](docs/library.md) | `pkg/fhirlint` |

Guides for particular workflows: [CI/CD integration](docs/ci.md), [validating partial JSON](docs/extract.md), [suppression rules](docs/suppression.md), [baseline mode](docs/baseline.md), [German FHIR profiles](docs/german-profiles.md), [offline terminology](docs/terminology-offline.md), [language server](docs/lsp.md), [server mode](docs/daemon.md), [built-in lint rules](docs/lint-rules.md), [custom rules](docs/rules.md) and [the reference check](docs/references.md).

## German profiles

`--profile` takes an alias for the German base and specialisation profiles. It loads the IG package, pinned to a version that is checked against the package registry every week:

```bash
fhirlint profiles                              # list the aliases
fhirlint validate --profile mii ./mii/          # all MII Kerndatensatz modules
fhirlint validate --profile isik ./isik/        # ISiK
```

[German FHIR profiles](docs/german-profiles.md) covers combining profiles, version pinning, and why German code systems are not checked by default.

## Integrations

- **GitHub Actions:** the `fhirlint/fhirlint` action, with inline annotations in the pull request ([Integrations](docs/integrations.md#github-actions)).
- **GitLab CI:** the CI/CD Catalog component, with JUnit and Code Quality reports ([CI/CD integration](docs/ci.md#gitlab-cicd-component)).
- **pre-commit:** a hook that validates the FHIR files of a commit ([Integrations](docs/integrations.md#pre-commit-hook)).
- **Editors:** `fhirlint lsp` puts findings into any LSP editor while you type ([Language server](docs/lsp.md)).
- **Claude Code:** a plugin with a validation skill and an opt-in hook ([Integrations](docs/integrations.md#use-as-a-claude-code-plugin)).

## Verifying a release

Release archives, `checksums.txt`, the SBOMs and the container image are signed through Sigstore and carry SLSA Build Level 3 provenance from the release after 1.17.0 on, and release builds are reproducible. How to check them: [Verifying a release](docs/verifying-releases.md).

## Contributing

If fhirlint is useful to you, a GitHub star helps others find it.

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup, tests and the pull request checklist. To report a bug, see [Reporting bugs](CONTRIBUTING.md#reporting-bugs).

These pages describe the project:

- how fhirlint works inside, what it defends against and why its design holds up: [ARCHITECTURE.md](ARCHITECTURE.md);
- who maintains it and who has access to what: [GOVERNANCE.md](GOVERNANCE.md);
- how it is tested: [TESTING.md](TESTING.md);
- where it is heading: [ROADMAP.md](ROADMAP.md);
- how it measures against OpenSSF Scorecard and the OSPS Baseline: [docs/osps-baseline.md](docs/osps-baseline.md).

Everyone taking part is expected to follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## Repositories

| Repository | Contents |
|---|---|
| [github.com/fhirlint/fhirlint](https://github.com/fhirlint/fhirlint) | The CLI, the Go library, the GitHub Action, the Claude Code plugin, releases and the container image |
| [gitlab.com/fhirlint/fhirlint](https://gitlab.com/fhirlint/fhirlint) | A read-only mirror that hosts the GitLab CI/CD Catalog component |
| [github.com/fhirlint/homebrew-tap](https://github.com/fhirlint/homebrew-tap) | The Homebrew cask |
| [github.com/fhirlint/web](https://github.com/fhirlint/web) | A browser UI for fhirlint |
| [github.com/fhirlint/example-fhir-medication-api](https://github.com/fhirlint/example-fhir-medication-api) | Example: a Symfony REST API serving FHIR R4 Medication and MedicationRequest resources, with fhirlint validating its fixtures in CI |

## Security

To report a vulnerability in fhirlint, see [SECURITY.md](SECURITY.md). It also documents the trust boundary, which is worth reading before filing: a finding the HL7 validator misses is a normal bug or an upstream one, while fhirlint misbehaving on attacker-supplied input is a vulnerability.

## License

Apache 2.0. See [LICENSE](LICENSE). The repository follows the [REUSE specification](https://reuse.software/): every file carries or is assigned its copyright and license, and `reuse lint` passes.

HL7® and FHIR® are registered trademarks of Health Level Seven International.
