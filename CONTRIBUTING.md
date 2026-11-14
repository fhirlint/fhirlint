# Contributing to fhirlint

Thanks for your interest in contributing! This document covers everything from setting up your environment to getting a PR merged. Everyone taking part in fhirlint's issues, pull requests and reviews follows the [Code of Conduct](CODE_OF_CONDUCT.md).

## Prerequisites

- **Go 1.26+** — [install](https://go.dev/dl/)
- **Java 11+** — required to run integration tests (`java -version` to check)
- **golangci-lint v2** — for local linting (`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`)

## Local setup

```bash
git clone https://github.com/fhirlint/fhirlint
cd fhirlint
go build ./...
go test ./...
```

## Running tests

What each test layer protects against and when CI runs it is in [TESTING.md](TESTING.md).

```bash
# Unit tests (no Java required)
go test ./...

# Integration tests — downloads the validator JAR on first run (~250 MB), requires Java
go test ./... -tags integration

# …against a specific validator release instead of whatever is cached
FHIRLINT_VALIDATOR_VERSION=6.10.1 go test ./... -tags integration

# Registry checks — verify the built-in profile aliases and the FHIR version
# table against packages.fhir.org and tx.fhir.org. Run them when you touch a pin
# in internal/profiles/aliases.go or a row in internal/validator/fhirversion.go
go test -tags registry -v ./internal/profiles/ ./internal/validator/

# Corpus false-positive scan — run it when you add or change a built-in lint
# rule (internal/lint) or the reference check (internal/refcheck), and paste the
# summary into the PR; conventions in docs/corpus-scan.md
scripts/corpus-fetch.sh && scripts/rule-diff.sh

# Fuzz one target, e.g. the LSP framing. The targets are the Fuzz* functions;
# .github/workflows/fuzz.yml lists them all with their packages
go test ./internal/lsp -run '^$' -fuzz '^FuzzRead$' -fuzztime 60s -fuzzminimizetime 5s
```

The fuzz targets cover the Go code that reads input fhirlint did not write: resources (lint, refcheck, ndjson, stats), validator output and message texts, source snippets, tx recordings and the LSP stream. Their seeds run as normal tests in `go test ./...`. CI fuzzes each target for 45s on every pull request and for 10 minutes every Sunday. When a target crashes, Go writes the input to `testdata/fuzz/<Target>/` in that package; commit it with the fix, and it replays as a regression test from then on.

The integration tests live in `internal/validator/` (the JAR itself) and `cmd/` (the CLI end to end); both are behind the `integration` tag and skipped by default. CI runs the two packages as separate steps, because `go test` runs packages in parallel and on a cold cache both would download the same JAR at once.

CI runs them twice over: pull requests that change `internal/validator`,
`internal/localig`, `cmd/validate.go`, `go.mod` or `go.sum`, and pushes to `main`,
test against the version pinned in `.github/workflows/integration-test.yml`, and a weekly scheduled job tests
against the latest upstream release with no JAR cache, so an upstream
regression surfaces within a week. Bump the pin once that canary has been green
on the newer release.

### Coverage

The unit tests run `fhirlint validate` and the subcommands end to end without Java: `internal/tools/fakejava` stands in for the validator, and markers in the input (`FAKE_ERROR`, `FAKE_WARNING`, `FAKE_CRASH`) decide what it reports (#595). A change to the path from the flags to the exit code gets a test in `cmd/validate_fake_test.go` or next to it.

CI fails the unit-test job when the total statement coverage, or that of `pkg/fhirlint` or `internal/validator`, falls below its floor, and when a pull request's changed lines are mostly uncovered (#545). The floors and exclusions are in `.github/workflows/test.yml`; raise a floor on purpose, never lower one to make a change pass. Locally:

```bash
go test -short -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -1
# changed lines of your branch against main
git diff --unified=0 origin/main -- '*.go' ':!*_test.go' > change.diff
go run ./internal/tools/patchcoverage -min 60 -grace 5 -diff change.diff
```

## Linting

```bash
golangci-lint run
```

- Config is in `.golangci.yml`
- All linter warnings must be resolved before a PR is merged
- `//nolint` comments require a reason (e.g. `//nolint:gosec // intentional: ...`)
- **Static analysis findings block the merge, at every severity.** golangci-lint, gosec included, and govulncheck are required checks. A finding is fixed, or it is suppressed with a `//nolint:<linter>` comment that says why it doesn't apply. Without the reason, the suppression is rejected in review. zizmor, glsec and cfgaudit findings on the workflows are fixed the same way, or carry an inline `ignore` with a reason

### Workflows and shell scripts

CI runs [actionlint](https://github.com/rhysd/actionlint) on every workflow (with ShellCheck on their `run:` blocks) and [ShellCheck](https://www.shellcheck.net/) on `hooks/*.sh`, `.github/scripts/*.sh` and the shell inside `action.yml` and the GitLab templates (#546). Locally, with both on your `PATH`:

```bash
actionlint
shellcheck hooks/*.sh .github/scripts/*.sh
go run ./internal/tools/extractshell -o /tmp/extracted action.yml templates/fhirlint.yml .gitlab-ci.yml
shellcheck /tmp/extracted/*.sh
```

A disabled diagnostic (`# shellcheck disable=SCxxxx`, or an ignore in `.github/actionlint.yaml`) is as narrow as possible and says why.

### Unreachable code

CI runs `deadcode` (`golang.org/x/tools/cmd/deadcode`) and fails on an exported function no main package reaches (#547). Delete it, or, when a test is its only caller, move it into a `_test.go` file. Only the public API in `pkg/fhirlint`, and what a test in another package or behind a build tag needs, goes in `.github/deadcode-allow.txt`, with a reason. Locally:

```bash
go run golang.org/x/tools/cmd/deadcode@v0.52.0 ./...
```

### Documentation

Markdown is linted for structure and rendering (`.markdownlint-cli2.jsonc`) and its links, anchors included, are checked with lychee (`lychee.toml`) on pull requests that change it; a weekly run opens an issue for links that died since (#548). Locally:

```bash
npx markdownlint-cli2 "**/*.md"
docker run --rm -v "$PWD:/input:ro" -w /input lycheeverse/lychee:0.24.2 --config lychee.toml "**/*.md"
```

Code blocks name a language: `bash` for commands, `console` for a session with `$` prompts, `text` for output.

### Workflow timeouts and budgets

Every job in `.github/workflows/` sets `timeout-minutes`, from its measured duration plus margin, with the basis in a comment; every workflow that runs on its own has a duration budget in `.github/workflow-budgets.txt`. `go test ./internal/workflows` fails until both are there (#551). A weekly run opens an issue when a workflow's 90th percentile goes over its budget; `.github/scripts/duration-watch.sh fhirlint/fhirlint .github/workflow-budgets.txt 7` prints the same table locally.

## Commit messages

We use [Conventional Commits](https://www.conventionalcommits.org/):

```text
feat(#20): add --best-practice flag
fix(#12): detect XML content from stdin
docs: update README flag table
test: add buildArgs unit tests
refactor: extract buildArgs as pure function
```

- **One-liner only** — keep the subject line short and self-contained; put additional context in the PR description, not the commit body
- First line under 72 characters
- Reference the relevant issue number where applicable
- **No AI co-author lines** — do not add `Co-Authored-By: Claude` or similar AI attribution to commits
- **Signed off** with `git commit -s` (see [Developer Certificate of Origin](#developer-certificate-of-origin))

## Developer Certificate of Origin

Every commit must be signed off. A sign-off is a line at the end of the commit message:

```text
Signed-off-by: Your Name <you@example.com>
```

With it you certify the [Developer Certificate of Origin](https://developercertificate.org/): that you wrote the change or otherwise have the right to submit it under the project's license. `git commit -s` adds the line from your `user.name` and `user.email`; the name and email must match the commit's author.

A check on every pull request blocks commits without a sign-off. To fix them:

- the last commit: `git commit --amend -s --no-edit`, then force-push;
- several commits: `git rebase --signoff main`, then force-push;
- or, without rewriting history, add one empty commit that signs off the earlier ones, as the check's details page describes.

## Dependencies

fhirlint keeps its dependencies few and checked:

- **Go modules.** `go.mod` lists the direct dependencies and `go.sum` pins their hashes. CI fails if `go mod tidy` would change either file, and the release build runs `go mod verify`. A new dependency needs a reason in the pull request. Prefer the standard library, and pick maintained, widely used modules.
- **Updates.** Dependabot proposes updates for Go modules, GitHub Actions, the Docker base images and the CI's Python tools. Version updates wait 7 days after an upstream release (`cooldown` in `.github/dependabot.yml`), the window in which a compromised release is usually caught. Security updates are not delayed. Updates go through the same checks as any change.
- **Pinning.** GitHub Actions are pinned to commit SHAs with a version comment that pinact verifies. The CI's Python tools are pinned with hashes (`.github/requirements/`), and the Docker base images by tag and digest.
- **Checks.** govulncheck, OSV-Scanner and dependency review run on every pull request, and all three are required checks. OSV-Scanner covers `go.mod` and the Python requirements against known vulnerabilities and known malicious packages. Trivy scans the container image.
- **The validator JAR** is a runtime dependency outside Go modules. It is verified against HL7's signing key on download and watched for advisories ([SECURITY.md](SECURITY.md#third-party-dependency-hl7-fhir-validator-jar)).

What blocks a merge or a release:

- **Vulnerabilities.** Any of these blocks:
  - a known vulnerability, of any severity, in a dependency that a pull request adds or changes;
  - a vulnerability that govulncheck finds reachable from fhirlint's code;
  - an OSV-Scanner finding that affects fhirlint;
  - a HIGH or CRITICAL finding in the image for which a fix exists.

  A finding that doesn't affect fhirlint, for example unreachable code, may be accepted only with a written reason in the pull request. Once it reaches users, it also needs a VEX statement as described in [SECURITY.md](SECURITY.md#vulnerabilities-that-dont-affect-fhirlint).
- **Malicious packages.** Always. The dependency is removed, and if a release contained it, the release is withdrawn and a security advisory is published.
- **Licenses.** Dependencies must be under a permissive or weak-copyleft license compatible with fhirlint's Apache-2.0 license: MIT, Apache-2.0, BSD-2-Clause, BSD-3-Clause, ISC, MPL-2.0, PSF-2.0, Unlicense or 0BSD. Dependency review fails on any other license. An exception needs a maintainer decision with a reason, recorded in `security.yml` next to `allow-dependencies-licenses`.

## Code review

Every change to `main` goes through a pull request. Before it is merged, the reviewer checks the following. Today the reviewer is a maintainer ([GOVERNANCE.md](GOVERNANCE.md#roles)), who applies the same checks to their own pull requests.

- **Correctness.** The change does what the issue asks, and the tests cover it. A bug fix has a test that fails without the fix. CI is green, including the coverage floors.
- **Findings.** A new or changed built-in lint rule or reference check comes with the corpus diff and a precision sample ([docs/corpus-scan.md](docs/corpus-scan.md)). A change to how validator output is parsed, suppressed or reported has been checked against a real JAR run.
- **Security.** fhirlint reads untrusted resources:
  - input is never executed, and processes start with argument lists, never through a shell;
  - paths stay inside what the user named;
  - responses from the network are bounded;
  - output escapes what it quotes.

  [ARCHITECTURE.md](ARCHITECTURE.md#threat-model) has the threat model. A change that touches one of its boundaries says so in the pull request.
- **Dependencies.** A new or updated dependency meets the [dependency rules](#dependencies).
- **Docs.** User-visible behaviour is documented in the same pull request: the README, the pages in `docs/`, `fhirlint.yml.example` and the schemas.
- **Conventions.** Branch name, signed-off commits, and a pull request description that says what changed, why, what was ruled out, and how to test it.

The reviewer leaves comments on anything that needs to change, and a pull request is merged only when none are open. Branch protection doesn't enforce this; the reviewer does.

## Opening a PR

- One PR per feature or fix — keep it focused
- **All new flags must also be supported in `fhirlint.yml`** (config file)
- New functionality requires tests — unit tests at minimum, integration tests where the JAR is involved
- A bug fix comes with a test that fails without the fix
- A major change adds or updates tests in the automated suite in the same PR: a new command, flag, report format or rule, or a change to how validator output is parsed, suppressed or reported
- Update the docs if the flag table or usage examples are affected: the flag and key reference is in `docs/configuration.md`, usage in `docs/usage.md` and the pages next to it
- PRs go directly against `main`
- `go.mod` and `go.sum` must be tidy: CI runs `go mod tidy -diff`
- A change to the plugin (`hooks/`, `skills/`, `.claude-plugin/`), the Action, the GitLab component or pipeline, the workflows, the release build, the JAR signing key or agent config such as `CLAUDE.md` fails the **Sensitive paths** check until a maintainer reviews it and sets the `sensitive-paths-reviewed` label. A new push removes the label again, unless the pull request's changes to the sensitive paths are the same as before the push: "Update branch", a rebase without conflicts in those files, or a push that only touches other files keep it. Any doubt (a sensitive file without a patch, more than 300 changed files, a commit the API can no longer compare after a force push, or `main` changing the same file) removes it. The full list is in `.github/scripts/sensitive-paths.sh`; its tests are in `internal/sensitivepaths`.

## Engage with the problem, not just the ticket

An issue is a hypothesis about a problem, not a specification to fill in. The most useful contributions test that hypothesis before writing code, and say what they found. We would rather merge a smaller change with a clear argument behind it than a larger one that only does what the title said. This matters more now that most contributions involve an AI tool, because [confidence in a tool tends to come at the cost of your own scrutiny](https://advait.org/files/lee_2025_ai_critical_thinking_survey.pdf).

Before and while you work on an issue:

- **Check the premise against the primary source.** fhirlint's value is findings people can trust. Confirm the behaviour against the [FHIR specification](https://hl7.org/fhir/) (the element definition, the invariant, the datatype), the IG, or the HL7 validator's code and messages. It is not enough that the resource parses or the expression matches. A rule that matches "as documented" but not "as the specification says" is a bug, green tests and all: every `url` looks like a canonical until the element definition says that `Device.url` is a network address. Link what you checked.
- **Bring the edge cases yourself.** State the realistic valid resource or configuration that would trip your change, and show it stays clean. For a false-positive fix, add the cases that must stay reported to [`scripts/corpus/must-stay-reported.txt`](scripts/corpus/must-stay-reported.txt), so the fix doesn't quietly drop real findings. Make the scope boundary explicit: what the change deliberately does not do, and why.
- **Measure, don't assume.** Run a new or changed lint rule or reference check against the real-world corpus (`scripts/rule-diff.sh`, see [docs/corpus-scan.md](docs/corpus-scan.md)) and put the finding delta in the pull request. Say how many findings it adds and removes, and whether each is a true or a false positive. For a change to how validator output is parsed, suppressed or reported, show it on a real JAR run. Green unit tests do not mean corpus-clean.
- **Propose the better approach, and show your working.** Say so if you see any of these: a cleaner design, a helper that already exists, an adjacent gap the issue misses, or a scope that is too narrow or too broad. Name the alternative you ruled out and why, and the main drawback of the approach you picked. Argue the tradeoff in the open rather than deciding it silently.
- **Disagreeing is welcome.** If the issue's framing or scope seems wrong or incomplete, comment on the issue before you build it. A comment that sharpens the problem is worth as much as the code that follows, and we won't read it as overstepping. Bring the evidence and we'll talk it through.

You don't need an open issue to contribute an idea. If you hit a resource or an IG where fhirlint reports something misleading, misses something, or could explain a finding better, open an issue and make the case: show the resource, say what fhirlint should do, and check first whether an existing rule or option already covers it. A sharp proposal with a real example is worth more than a long list of maybes. We would rather hear a good idea and decide against it than never hear it.

This applies to your AI tool as much as to you. A tool's output reads as plausible whether or not it is right, so [verify it against the source yourself](https://arxiv.org/abs/2211.03622) rather than trusting it because it looks correct. If you used one, it should have done this thinking with you. A pull request that restates the issue title with no sign that the premise was tested reads as unreviewed, whoever or whatever wrote it.

## AI-assisted contributions

Contributions written with AI tools are welcome on the same terms as any other:

- **Say so** in the "AI assistance" field of the pull request template: which tool, and for what. Not as a `Co-Authored-By` line in the commit (see above).
- **Understand and stand behind every line.** You are the author. "The model wrote it" is not an answer in review.
- **Check FHIR and validator claims against the source:** the FHIR specification, the IG, or the HL7 validator's code and messages. Do not take a model's word for them. Message ids, element paths, terminology behaviour and version numbers are where generated text is most often confidently wrong.
- **One issue per pull request.** Generated changes tend to sprawl; keep them to what the issue asks for.

The sign-off works the same way: with it you certify that you have the right to submit the change, however it was written.

## Adding a new flag

The pattern is consistent across all flags — follow these steps:

1. Add the field to `Options` in `internal/validator/run.go`
2. Pass it through `buildArgs()` in the same file
3. Declare `flagXxx` and register the flag in `cmd/validate.go`
4. Add viper binding and merge logic (see existing flags for the pattern)
5. Add the key to `fhirlint.yml.example` with a comment
6. Add unit tests in `internal/validator/build_args_test.go` and `cmd/config_test.go`

## Reporting bugs

Open a [GitHub issue](https://github.com/fhirlint/fhirlint/issues) and include:

- fhirlint version: `fhirlint version`
- Java version: `java -version`
- OS and architecture
- The exact command that failed
- The FHIR resource if possible (anonymize sensitive data)

## Suggesting features

- See the [roadmap](ROADMAP.md) for what is planned and what is deliberately out of scope
- Open an issue first before starting implementation
- Describe the **use case**, not just the solution
- Check existing issues — the feature may already be planned

## Good first issues

Issues labelled [`good first issue`](https://github.com/fhirlint/fhirlint/issues?q=is%3Aopen+label%3A%22good+first+issue%22) are well-scoped for new contributors. Feel free to ask questions in the issue before starting.
