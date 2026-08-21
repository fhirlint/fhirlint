# Contributing to fhirlint

Thanks for your interest in contributing! This document covers everything from setting up your environment to getting a PR merged.

## Prerequisites

- **Go 1.25+** — [install](https://go.dev/dl/)
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
```

The integration tests live in `internal/validator/` (the JAR itself) and `cmd/` (the CLI end to end); both are behind the `integration` tag and skipped by default. CI runs the two packages as separate steps, because `go test` runs packages in parallel and on a cold cache both would download the same JAR at once.

CI runs them twice over: pushes to `main` test against the version pinned in
`.github/workflows/integration-test.yml`, and a weekly scheduled job tests
against the latest upstream release with no JAR cache, so an upstream
regression surfaces within a week. Bump the pin once that canary has been green
on the newer release.

## Linting

```bash
golangci-lint run
```

- Config is in `.golangci.yml`
- All linter warnings must be resolved before a PR is merged
- `//nolint` comments require a reason (e.g. `//nolint:gosec // intentional: ...`)

## Commit messages

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
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

## Opening a PR

- One PR per feature or fix — keep it focused
- **All new flags must also be supported in `fhirlint.yml`** (config file)
- New functionality requires tests — unit tests at minimum, integration tests where the JAR is involved
- Update `README.md` if the flag table or usage examples are affected
- PRs go directly against `main`

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

- Open an issue first before starting implementation
- Describe the **use case**, not just the solution
- Check existing issues — the feature may already be planned

## Good first issues

Issues labelled [`good first issue`](https://github.com/fhirlint/fhirlint/issues?q=is%3Aopen+label%3A%22good+first+issue%22) are well-scoped for new contributors. Feel free to ask questions in the issue before starting.
