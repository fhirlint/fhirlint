# fhirlint — Claude Code context

fhirlint is a Go CLI that wraps the HL7 FHIR Validator JAR and adds developer-friendly output (terminal, JSON, HTML, JUnit, SARIF), watch mode, suppression rules, and built-in aliases for German FHIR profiles (KBV, MII, DiGA). The JAR is auto-downloaded on first use.

## Architecture

```
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
7. Tests: `internal/validator/build_args_test.go`, `cmd/config_test.go`, `pkg/fhirlint/fhirlint_test.go`

### Tests

- `go test ./...` — unit tests, no Java required
- `go test ./... -tags integration` — integration tests, downloads JAR (~250 MB), requires Java 17+
- Skip long-running or subprocess tests with `testing.Short()` guard

### Linting

```bash
golangci-lint run
```

`//nolint` annotations require a reason comment.

## Branching & PRs

- Every issue gets its own branch: `issue-<number>-<short-slug>`
- Open a PR against `main` — details go in the PR description, not the commit message
- Merge directly to `main` after review

## Commit messages

- **One-liner only** — `feat(#42): add --foo flag` is enough; details belong in the PR description
- Follow [Conventional Commits](https://www.conventionalcommits.org/): `feat`, `fix`, `docs`, `test`, `refactor`, `chore`
- Reference the issue number where applicable: `fix(#12): ...`
- **No `Co-Authored-By` lines** — do not add AI co-author attribution to commits
