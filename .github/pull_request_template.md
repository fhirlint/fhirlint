## Summary

<!-- What does this PR do and why? Link the issue. -->

## How to test

<!-- Commands or steps a reviewer can run. -->

## AI assistance

<!-- None, or which tool and for what (see CONTRIBUTING.md). -->

## Checklist

- [ ] Tests added (unit tests required, integration tests for JAR-related changes)
- [ ] New flags also configurable via `fhirlint.yml`
- [ ] `README.md` updated if flag table or usage examples are affected
- [ ] `go test ./...` passes locally
- [ ] `golangci-lint run` passes locally
- [ ] `go mod tidy -diff` reports nothing
- [ ] `TESTING.md` updated if a test layer was added, removed or changed
- [ ] Built-in lint rule or reference check changed: `scripts/rule-diff.sh` summary and precision sample in the description ([docs/corpus-scan.md](../docs/corpus-scan.md))
