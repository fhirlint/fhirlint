# fhirlint — Claude Code context

fhirlint is a Go CLI that wraps the HL7 FHIR Validator JAR and adds developer-friendly output (terminal, JSON, HTML, JUnit, SARIF), watch mode, suppression rules, and built-in aliases for German FHIR profiles (KBV, MII, ISiK, DiGA). The JAR is auto-downloaded on first use.

## Architecture

```text
cmd/           CLI commands (cobra + viper)
  validate.go  main validate command — flag parsing, orchestration
  audit.go     audit subcommand
  cache.go     cache management
internal/
  validator/   core: run.go (exec JAR), jar.go (download), java.go, audit.go
  reporter/    output formatters: terminal, json, html, junit, sarif
  suppress/    suppression rule parsing and application
  input/       input resolution (file, dir, stdin, URL)
  profiles/    built-in profile alias resolution
  localig/     bundle local CodeSystem/ValueSet files into a temp IG
  resultcache/ file-based result cache (keyed by content hash + options)
  cache/       JAR/validator version file paths
pkg/fhirlint/  public Go library API (stable, embeddable)
testdata/      fixtures for unit tests
```

## Key conventions

### Adding a new flag

1. Add field to `Options` in `internal/validator/run.go`
2. Pass through `buildArgs()` in the same file
3. Declare `flagXxx` and register flag in `cmd/validate.go`
4. Add viper binding + config-merge block (follow existing pattern)
5. Add field to `pkg/fhirlint/fhirlint.go` `Options` and pass through `toInternalOpts`
6. Add entry to `fhirlint.yml.example`
7. Decide whether the flag changes the *result*. It joins the `--cache` key
   automatically (the key covers all of `Options`), so there is nothing to do
   unless it must be **excluded** — see `keyedOptions` in
   `internal/resultcache/` — or it is a **file path**, which belongs in the key
   as a `txreplay.Fingerprint`, not as a path. `TestKeyedOptions_EveryFieldHasBeenConsidered`
   fails until the decision is recorded.
8. Tests: `internal/validator/build_args_test.go`, `cmd/config_test.go`, `pkg/fhirlint/fhirlint_test.go`

### Tests

TESTING.md describes every test layer, what it protects against and when it runs. A PR that adds or removes a layer updates it.

- `go test ./...` — unit tests, no Java required
- `go test ./... -tags integration` — integration tests, downloads JAR (~250 MB), requires Java 17+
- `go test -tags registry ./internal/profiles/ ./internal/validator/` — checks the built-in aliases and the FHIR version table against the package registry and tx.fhir.org; needs network, run it when changing a pin or a version row
- Skip long-running or subprocess tests with `testing.Short()` guard
- Fuzz targets (`Fuzz*`) cover Go code that reads input fhirlint did not write; a new parser of such input gets one and a row in `.github/workflows/fuzz.yml`

### Linting

```bash
golangci-lint run
```

`//nolint` annotations require a reason comment.

## Working on an issue

Treat the issue as a hypothesis to test, not a spec to fill. fhirlint's value is findings people can trust, so the thinking matters more than the diff.

- Before coding, restate the problem in your own words and check the premise against the primary source: the FHIR specification (element definitions, invariants, the datatype), the IG, or the HL7 validator's code and messages. If it doesn't hold, say so and propose the correction instead of implementing a change you've shown to be wrong.
- Hunt the counter-evidence: a realistic valid resource or configuration that trips the change (false positive), the findings a fix could silently drop (`scripts/corpus/must-stay-reported.txt`), a simpler or more general approach, an existing helper in `internal/lint`, `internal/refcheck` or `internal/validator` that already covers it.
- For any change to a built-in lint rule or the reference check, run `scripts/rule-diff.sh` over the corpus and put the finding delta (added/removed, true vs false positive) in the PR ([docs/corpus-scan.md](docs/corpus-scan.md)). For a change to how validator output is parsed or reported, check it against a real JAR run. Don't claim corpus-clean from unit tests.
- Put the reasoning in the PR description: what you considered and rejected, the scope boundary (what it deliberately doesn't do), the tradeoffs. "Implements #N" is not a description.
- Disagreeing with the issue, backed by evidence, is wanted. Raise it rather than quietly building the wrong thing.

## Branching & PRs

- Every issue gets its own branch: `issue-<number>-<short-slug>`
- Open a PR against `main` — details go in the PR description, not the commit message
- Merge directly to `main` after review

## Commit messages

- **One-liner only** — `feat(#42): add --foo flag` is enough; details belong in the PR description
- Follow [Conventional Commits](https://www.conventionalcommits.org/): `feat`, `fix`, `docs`, `test`, `refactor`, `chore`
- Reference the issue number where applicable: `fix(#12): ...`
- **No `Co-Authored-By` lines** — do not add AI co-author attribution to commits
- Sign off every commit (`git commit -s`): the DCO check blocks commits without `Signed-off-by:` (#578)
- A new `.go`, shell or Dockerfile starts with `// Copyright The fhirlint Authors` and `// SPDX-License-Identifier: Apache-2.0` (`#` outside Go); a file taken from elsewhere gets an entry in `REUSE.toml` instead (#602)
