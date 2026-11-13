# Testing fhirlint

fhirlint sits between the HL7 FHIR Validator and the people and tools that act on its findings. It must not change what the validator decided, except where that is documented, and it must not lose anything on the way: a dropped finding gives false confidence, a wrong one teaches people to ignore the report. This page is the overview of how fhirlint is tested, what each layer protects against and when it runs. How to run each layer locally is in [CONTRIBUTING.md](CONTRIBUTING.md#running-tests).

## Principles

- **Every bug leaves a test behind.** A wrong parse of validator output becomes a stdout or OperationOutcome fixture in `internal/validator/testdata`, per validator version (as in #521). A crash found by fuzzing becomes a seed in `testdata/fuzz/`. A cache-key mistake becomes a row in `TestKeyedOptions_EveryFieldHasBeenConsidered`.
- **The validator is the reference.** fhirlint changes the validator's verdict only where documented: suppressions, the baseline, `--severity`, `--display-issues-are-warnings` and the 2,000-character message cap (#519). Tests cover what fhirlint adds, and that nothing gets lost between the JAR and the report.
- **When unsure, don't drop a finding.** Unknown output lines, unparseable OperationOutcome entries and missing message IDs are kept and reported, not ignored.
- **Pinned, then canary.** CI tests against a pinned validator release and, weekly, against the latest one (#310), so upstream drift shows within a week without blocking pull requests.
- **All platforms count.** Unit tests run on Linux, macOS and Windows; integration tests on Linux and macOS.

## Layers

Grouped by what they check: [findings](#findings), [reports and consistency](#reports-and-consistency), [robustness and performance](#robustness-and-performance), [code health](#code-health), [supply chain and repository security](#supply-chain-and-repository-security), [packages](#packages), [documentation](#documentation), the checks [before each release](#before-each-release) and the [scheduled watches](#scheduled-watches). Layers marked *planned* have an open issue.

### Findings

Whether fhirlint reports the validator's findings, all of them and only those.

| Layer | Protects against | Runs |
|---|---|---|
| Unit tests with faked validator output (`go test -short -race ./...`) | wrong parsing, mapping, filtering or output in fhirlint's own code | every pull request, Linux, macOS and Windows |
| Validator output fixtures per validator version (`internal/validator/testdata`) | a validator release whose stdout or OperationOutcome format fhirlint no longer reads | every pull request, as part of the unit tests |
| Integration tests with the real JAR (`-tags integration`), pinned validator in [`integration-test.yml`](.github/workflows/integration-test.yml) | a broken JAR invocation, download, Java detection or temp IG bundling (`internal/localig`), none of which the unit tests touch | pull requests that change `internal/validator`, `internal/localig`, `cmd/validate.go`, `go.mod` or `go.sum`; every push to `main`; Linux and macOS |
| Canary: the integration tests against the latest validator, with no JAR cache | an upstream release that breaks fhirlint, noticed within a week instead of at the next pin bump | weekly |
| Registry tests (`-tags registry`): the built-in profile aliases, the FHIR version table and the R6 sentinel against packages.fhir.org and tx.fhir.org | an alias pointing at a package or version that doesn't exist, a wrong terminology path | weekly via [`alias-monitor.yml`](.github/workflows/alias-monitor.yml), which opens an issue on failure |
| *Planned:* equivalent routes (file, stdin, server, cache, replay, …) give the same findings (#544) | a route that drops or changes a finding while the others don't | integration |

### Reports and consistency

Whether the output stays readable for the tools that consume it, and the parts that must agree do.

| Layer | Protects against | Runs |
|---|---|---|
| Cache key completeness: `TestKeyedOptions_EveryFieldHasBeenConsidered` | a new option that changes the result but not the `--cache` key, so a stale result is served | every pull request |
| Config consistency: `fhirlint.schema.json` generated equals committed, every config key known to `fhirlint config check`, `fhirlint.yml.example` checked with `check-jsonschema` | an editor schema or example config that disagrees with what fhirlint reads | every pull request |
| FHIR version table checks (#318, #494): README row and example comment against `validator.FHIRVersions` | a FHIR version added in one place and missed in another | every pull request |
| Report schemas (#542): golden SARIF, Code Climate, JUnit and JSON reports in `internal/reporter/testdata/golden`, built from findings that use every field (`TestGoldenReports`, `-update` to regenerate), and checked in CI (`lint.yml`, "Report schemas") against the OASIS SARIF 2.1.0 schema, Jenkins' JUnit XSD, the fields GitLab documents for Code Quality and [`fhirlint-report.schema.json`](fhirlint-report.schema.json) | a report GitHub code scanning or GitLab rejects or reads only in part; a JSON field renamed or retyped without notice | every pull request |
| Finding identity snapshot (`internal/identity`, #543): baseline entries, Code Climate fingerprint, SARIF `ruleId` and `partialFingerprints`, and which suppression rules match, for a fixed set of issues; `-update` to regenerate, and the PR explains the change | a refactor that silently makes existing findings look new in GitLab or code scanning, or a baseline or suppression stop matching | every pull request |

### Robustness and performance

| Layer | Protects against | Runs |
|---|---|---|
| [Fuzzing](CONTRIBUTING.md#running-tests) of every parser of input fhirlint didn't write, 24 targets listed in [`fuzz.yml`](.github/workflows/fuzz.yml) | crashes and hangs on hostile or broken resources, validator output, tx recordings and the LSP stream | 45 s per target on pull requests and pushes to `main`, 10 minutes weekly |
| Race detector | data races in the server, watch and LSP modes | every pull request, all three OSes |
| Hardening tests: control characters and clipping (`internal/termsafe`), symbolic links (`internal/input`, `internal/coverage`, `cmd`), cache entry trust (`internal/resultcache`), `--redact`, long messages | input that makes fhirlint read outside its directory, inject terminal escapes, trust a planted cache entry, or print data the user asked to redact | every pull request, as part of the unit tests |
| *Planned:* linear scaling test and benchmarks for the Go-side pipeline (#552) | a quadratic step that only shows on a whole IG | every pull request |

### Code health

| Layer | Protects against | Runs |
|---|---|---|
| [golangci-lint](CONTRIBUTING.md#linting) with gosec | bugs and insecure patterns in fhirlint's own code | every pull request |
| `go mod tidy -diff`, `go mod verify` | an untidy `go.mod`, a module that doesn't match its checksum | every pull request |
| Coverage (#545): statement floors on the total and on `pkg/fhirlint` and `internal/validator`, and coverage of the lines a pull request changes (`internal/tools/patchcoverage`); the integration profile is shown, not gated | code no test exercises, also while the total stays high | every pull request (Linux leg of the unit tests) |
| *Planned:* deadcode with an allowlist (#547) | exported functions nothing calls any more | every pull request |
| *Planned:* actionlint and ShellCheck (#546) | invalid workflow expressions and shell bugs in workflows, the plugin hook and CI scripts | pull requests that change them |

### Supply chain and repository security

| Layer | Protects against | Runs |
|---|---|---|
| govulncheck | a reachable known vulnerability in a Go dependency | every pull request, push to `main`, weekly |
| cfgaudit, glsec, zizmor | injection, excessive permissions, unpinned actions and dangerous triggers in the workflows and the GitLab templates | every pull request, push to `main`, weekly |
| Sensitive paths gate ([`.github/scripts/sensitive-paths.sh`](.github/scripts/sensitive-paths.sh), tests in `internal/sensitivepaths`) | an unreviewed change to files that reach plugin or pipeline users, the release build, the JAR verification or a maintainer's agent setup; fails until a maintainer sets `sensitive-paths-reviewed`. A new push removes the label unless the sensitive diff is unchanged (#541) | every pull request |
| Release gate (`require-green-ci.yml`) | a release or image built from a commit CI didn't pass | every tag push, before Release and Docker build anything |
| HL7 JAR signature check with a pinned key; key expiry and advisory watch ([`jar-monitor.yml`](.github/workflows/jar-monitor.yml)) | running a validator JAR that HL7 didn't sign, a signing key that expires unnoticed, a known advisory against the JAR | every download; weekly watch |
| *Planned:* OSV-Scanner, dependency review, pinact (#550) | unreachable vulnerabilities and non-Go manifests, a PR that adds a vulnerable dependency, an action whose version comment lies | every pull request |
| *Planned:* reproducible build (#549) | a release binary that differs from what the source builds | pull requests that change the build, pushes to `main` |

### Packages

| Layer | Protects against | Runs |
|---|---|---|
| Container image: hadolint, Trivy, dockle, runs as non-root, reports its version, validates `testdata` | Dockerfile mistakes, a fixable HIGH or CRITICAL vulnerability, an image that runs as root or doesn't start | every pull request, push to `main`, weekly |
| Composite Action smoke test ([`action-test.yml`](.github/workflows/action-test.yml)) | an Action that doesn't install or run | pull requests and pushes that change `action.yml` |
| *Planned:* release archive smoke test (#549) | an archive with a wrong binary or version, noticed only by users and `self-update` | before each release is published |

### Documentation

| Layer | Protects against | Runs |
|---|---|---|
| *Planned:* link check and markdownlint (#548) | dead links and anchors, broken tables and lists | pull requests that change Markdown, weekly |

### Before each release

| Check | Protects against |
|---|---|
| `fhirlint upgrade-check` on real IG data (kbv.basis) before moving the validator pin | a validator release that changes findings in ways the fixtures don't show |
| The weekly ecosystem review of FHIR and tooling news | upstream changes (validator, registry, terminology servers, profiles) that need an issue before they become a bug report |
| Is this page still accurate? | layers added or removed without the overview following |

### Scheduled watches

| Watch | Runs | On failure |
|---|---|---|
| Canary integration tests, latest validator | Monday | red run |
| `alias-monitor.yml`: aliases, FHIR version table, R6 sentinel | Monday | opens an issue |
| `jar-monitor.yml`: signing key expiry, advisories | Monday | opens an issue |
| Docker image scan, govulncheck, zizmor | Monday | red run |
| Fuzzing, 10 minutes per target | Sunday | red run, crash input as artifact |

## Keeping this page current

A pull request that adds, removes or changes a test layer updates this page in the same pull request; the pull request template has a checklist line for it.
