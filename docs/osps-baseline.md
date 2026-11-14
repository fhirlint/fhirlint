# OpenSSF Scorecard and OSPS Baseline

This page records how fhirlint measures up against two OpenSSF yardsticks (#578):

- [OpenSSF Scorecard](https://scorecard.dev/), an automated check of the repository's supply-chain practices. [`scorecard.yml`](../.github/workflows/scorecard.yml) runs it weekly, on every push to `main` and when branch protection changes. It uploads the findings to code scanning and publishes the score behind the README badge.
- The [Open Source Project Security Baseline](https://baseline.openssf.org/) (OSPS Baseline), release [v2026.08.28](https://github.com/ossf/security-baseline/releases/tag/v2026.08.28). It is a list of controls in three maturity levels. Below, each control is mapped to the evidence in this repository. The open items are listed, not hidden.

Assessed on 2026-11-14. Repository settings were read through the GitHub API on that day.

## Scorecard

A local run of Scorecard v5.6.0 against `main` before this change scored 7.4 out of 10. This table shows what was found and what #578 did about it:

| Check | Score | Finding | Done in #578 |
|---|---|---|---|
| Token-Permissions | 7 | `fuzz.yml`, `integration-test.yml`, `lint.yml`, `security.yml` and `test.yml` set permissions per job, but none at the top. | Each now sets `permissions: {}` at the top. |
| Pinned-Dependencies | 9 | The `golang` and `eclipse-temurin` base images in the `Dockerfile` were pinned by tag only. | They are now pinned by digest too. Dependabot's docker ecosystem moves the digest. |
| Vulnerabilities | 7 | GO-2026-6354, GO-2026-6355 (`x/crypto/ssh`) and GO-2026-5932 (`x/crypto/openpgp`), all in the indirect `golang.org/x/crypto` v0.55.0. fhirlint imports neither package (`x/crypto` comes in for `sha3`), so govulncheck and OSV-Scanner's call analysis don't report them. | Raised to v0.58.0, which fixes the two ssh entries. GO-2026-5932 is fixed only in v0.59.0, which was still inside the repository's seven-day Dependabot cooldown (see open items). |
| Signed-Releases | 0 | Scorecard looks for a signature or provenance file among the release assets. The SLSA provenance from `attest-build-provenance` was only in the GitHub attestations API. | `release.yml` attaches it to each release as `fhirlint_<version>.sigstore.json` and `.intoto.jsonl`. The score rises as new releases replace the last five that Scorecard checks. |
| Branch-Protection | 3 | The protection rules don't apply to administrators. Pull requests and reviews aren't required, and there are no code owners. | On 2026-11-14 branch protection was extended to administrators, and a pull request is now required, with no approvals, so a single maintainer can still merge. Required reviews, stale-review dismissal and code owners stay off; see Code-Review. |
| Code-Review | 0 | 0 of 24 recent changesets were approved by someone other than the author. | Not changed. Approvals aren't required, so a maintainer can merge alone. CI, the sensitive-paths gate and the PR template are the review that exists. |
| CII-Best-Practices | 0 | No OpenSSF Best Practices badge. | Not changed. The badge needs a maintainer to register the project at bestpractices.dev; see open items. |

The other checks scored 10: Binary-Artifacts, CI-Tests, Contributors, Dangerous-Workflow, Dependency-Update-Tool, Fuzzing, License, Maintained, Packaging, SAST and Security-Policy.

## OSPS Baseline

Status: **met**, **partly met** or **open**.

### Level 1

| Control | Status | Evidence |
|---|---|---|
| AC-01.01 MFA for sensitive actions | met | The organization requires two-factor authentication for every member, since 2026-11-14. |
| AC-02.01 Least privilege for new collaborators | met | The organization's default repository permission is `read`. Write and admin access is granted per person. |
| AC-03.01 No direct commits to the primary branch | met | Branch protection on `main` requires a pull request and seven status checks in strict mode, and it applies to administrators too. |
| AC-03.02 Primary branch can't be deleted | met | Branch protection disallows deletion and force pushes. |
| BR-01.01 Untrusted metadata is sanitized | met | Workflows pass event data to `run:` blocks through `env:`, never inline. zizmor, glsec and cfgaudit check for template injection on every pull request ([TESTING.md](../TESTING.md#supply-chain-and-repository-security)). |
| BR-01.03 Untrusted code has no privileged credentials | met | Pull requests run under `pull_request`, with a read-only token and no secrets. The one `pull_request_target` workflow, [`sensitive-paths.yml`](../.github/workflows/sensitive-paths.yml), checks out `main` only and reads the pull request through the API. |
| BR-03.01 Project channels use encryption | met | The repository, releases, `ghcr.io/fhirlint/fhirlint` and the Homebrew tap are all served over HTTPS. |
| BR-03.02 Distribution channels are authenticated | met | HTTPS, plus SLSA provenance on the archives, the SBOMs, `checksums.txt` and the image, and a cosign signature on the image ([README](../README.md#verifying-the-image)). |
| BR-07.01 No secrets in version control | met | Secret scanning and push protection are enabled. |
| DO-01.01 User guides | met | [README](../README.md) and [docs/](.). |
| DO-02.01 How to report defects | met | [CONTRIBUTING.md](../CONTRIBUTING.md#reporting-bugs). |
| GV-02.01 Public discussion | met | GitHub issues and pull requests. |
| GV-03.01 Contribution process | met | [CONTRIBUTING.md](../CONTRIBUTING.md). |
| LE-02.01, LE-02.02 OSI licence | met | Apache-2.0 for the source and the release assets. |
| LE-03.01, LE-03.02 Licence file | met | [`LICENSE`](../LICENSE) in the repository, and in every release archive (`.goreleaser.yml`, `archives.files`). |
| QA-01.01, QA-01.02 Public repository and history | met | Public on GitHub. Every change goes through Git history. |
| QA-02.01 Dependency list | met | `go.mod` and `go.sum`. The Python CI tools are hash-pinned in `.github/requirements`. |
| QA-04.01 List of codebases | met | See [Codebases](#codebases). |
| QA-05.01, QA-05.02 No generated executables or unreviewable binaries | met | None are tracked. Scorecard's Binary-Artifacts check scores 10. |
| VM-02.01 Security contacts | met | [SECURITY.md](../SECURITY.md): private advisories reach the maintainers. |

Level 1 is met.

### Level 2

| Control | Status | Evidence |
|---|---|---|
| AC-04.01 Default CI permissions are minimal | met | The repository's default `GITHUB_TOKEN` permission is `read`. Every workflow sets its permissions at the top, and every job sets its own. |
| BR-02.01 Unique version per release | met | Semantic version tags `vX.Y.Z`. |
| BR-04.01 Release notes | met | Each GitHub release has highlights, upgrade notes and a changelog generated from Conventional Commits (`.goreleaser.yml`, `changelog`). |
| BR-05.01 Standard dependency tooling | met | Go modules with the checksum database, and `go mod verify` before the release build. |
| BR-06.01 Signed releases or manifest | met | `checksums.txt` lists every asset's SHA-256 and carries SLSA provenance signed through Sigstore. |
| DO-06.01 How dependencies are selected and tracked | met | See [Dependencies](#dependencies). |
| DO-07.01 Build instructions | met | [CONTRIBUTING.md](../CONTRIBUTING.md#prerequisites) and [Local setup](../CONTRIBUTING.md#local-setup). |
| GV-01.01 Members with access to sensitive resources | met | [GOVERNANCE.md](../GOVERNANCE.md#members-with-access-to-sensitive-resources). |
| GV-01.02 Roles and responsibilities | met | [GOVERNANCE.md](../GOVERNANCE.md#roles). |
| GV-03.02 Requirements for contributions | met | [CONTRIBUTING.md](../CONTRIBUTING.md) (tests, linting, commit messages, AI-assisted contributions) and the [PR template](../.github/pull_request_template.md). |
| LE-01.01 Contributors assert their right to contribute | met | Every commit in a pull request needs a DCO sign-off. The DCO check enforces it ([`.github/dco.yml`](../.github/dco.yml), [CONTRIBUTING.md](../CONTRIBUTING.md#developer-certificate-of-origin)). |
| QA-03.01 Status checks pass before merging | met | Required checks on `main`: golangci-lint, unit tests, govulncheck, cfgaudit, glsec, the sensitive-paths gate and DCO, in strict mode. Releases additionally wait for all of CI (`require-green-ci.yml`). |
| QA-06.01 Automated tests before a change is accepted | met | Unit tests on Linux, macOS and Windows are a required check ([TESTING.md](../TESTING.md)). |
| SA-01.01 Design documentation of actions and actors | partly met | The [trust boundary](../SECURITY.md#trust-boundary) names who and what fhirlint trusts. The architecture map is in `CLAUDE.md`. There is no single design document of all actors and actions. |
| SA-02.01 External interfaces | met | CLI ([README](../README.md)), the Go library ([pkg.go.dev](https://pkg.go.dev/github.com/fhirlint/fhirlint/pkg/fhirlint)), the JSON schemas of the config and the report, the GitHub Action ([`action.yml`](../action.yml)), the GitLab component ([docs/ci.md](ci.md)) and the LSP server ([docs/lsp.md](lsp.md)). |
| SA-03.01 Security assessment | met | [SECURITY.md](../SECURITY.md): trust boundary, what is in scope and out of it, and the properties users can rely on. |
| VM-01.01 Coordinated disclosure with a response time | met | [SECURITY.md](../SECURITY.md): GitHub Security Advisory, with a response within 5 business days. |
| VM-03.01 Private reporting | met | Private vulnerability reporting is enabled. |
| VM-04.01 Published vulnerability data | met | Confirmed vulnerabilities are published as GitHub Security Advisories of this repository ([SECURITY.md](../SECURITY.md)). None have been published so far. |

Level 2 is open in one place: SA-01.01.

### Level 3

These aren't a goal yet. Already met:

- AC-04.02: minimal permissions per job.
- BR-02.02: every asset name carries the version.
- BR-07.02: the [secrets policy](../GOVERNANCE.md#secrets-and-credentials).
- GV-04.01: [granting access](../GOVERNANCE.md#granting-access).
- DO-03.01 and DO-03.02: [verifying archives](../README.md#pre-built-binary-recommended) and [the image](../README.md#verifying-the-image), including the signing workflow.
- QA-02.02: an SPDX SBOM per archive, and an SBOM attestation on the image.
- QA-06.02: [TESTING.md](../TESTING.md).
- VM-05.03: dependency review and OSV-Scanner on every pull request.

Open:

- BR-01.04: sanitizing trusted collaborator input.
- DO-04.01 and DO-05.01: [SECURITY.md](../SECURITY.md#supported-versions) says that only the latest release gets fixes, but it doesn't give a duration per release.
- QA-06.03: a written test policy for major changes.
- QA-07.01: approval by a non-author.
- SA-03.02: a threat model.
- VM-04.02: VEX documents.
- VM-05.01, VM-05.02, VM-06.01 and VM-06.02: written remediation thresholds for SCA and SAST findings.

## Codebases

| Repository | What it is |
|---|---|
| [fhirlint/fhirlint](https://github.com/fhirlint/fhirlint) | This repository: the CLI, the Go library, the GitHub Action, the GitLab component and the Claude Code plugin. Everything a release ships is built from here. |
| [fhirlint/homebrew-tap](https://github.com/fhirlint/homebrew-tap) | The Homebrew cask, written by GoReleaser on each release. |
| [fhirlint/web](https://github.com/fhirlint/web) | A browser UI for fhirlint. It is a separate project with its own releases. |
| [fhirlint/example-fhir-medication-api](https://github.com/fhirlint/example-fhir-medication-api) | An example project that validates its FHIR output with fhirlint. |

## Dependencies

- **Go modules:** `go.mod` and `go.sum`, verified against the Go checksum database. `go mod verify` runs before the release build.
- **Actions:** pinned by commit SHA, with the version in a comment. pinact checks that the comment matches the SHA.
- **Base images:** pinned by tag and digest.
- **Python CI tools:** hash-pinned in `.github/requirements`.
- **Updates:** Dependabot raises version updates weekly for each ecosystem, after a seven-day cooldown, and raises security updates as soon as an advisory appears.
- **Pull request gates:** dependency review blocks a pull request that adds a dependency with a known vulnerability or a licence outside the allowlist in `security.yml`. govulncheck and OSV-Scanner also run weekly on `main`.
- **The HL7 validator JAR:** it isn't a Go module and is tracked separately: verified against HL7's signing key on download, and watched for advisories by `jar-monitor.yml` ([SECURITY.md](../SECURITY.md#third-party-dependency-hl7-fhir-validator-jar)).

## Open items

1. **Register at bestpractices.dev** for the Best Practices badge (Scorecard CII-Best-Practices). That is also where an OSPS Baseline self-assessment can be published.
2. **Raise `golang.org/x/crypto` to v0.59.0** once it is past the cooldown, from 2026-11-18. That closes GO-2026-5932 for Scorecard. fhirlint doesn't import the affected package.
3. **One design document of actors and actions** (SA-01.01).

## Keeping this page current

Code scanning shows the Scorecard findings, with the category `scorecard`. When a check drops, fix it, or add it to the table above with the reason. When a new OSPS Baseline release comes out, check whether a control was added or moved to another level.
