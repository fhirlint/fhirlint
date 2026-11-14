## Summary

<!-- What does this PR do and why? Link the issue. -->

## Approach

<!-- Skip if trivial. What did you consider and rule out, and why this way? What does this change deliberately NOT do (the scope boundary)? If the issue's framing or scope seemed off, say what you'd change. -->

## How to test

<!-- Commands or steps a reviewer can run. -->

## AI assistance

<!-- None, or which tool, what it did, and what you checked yourself: the FHIR or validator behaviour you confirmed against the specification, the IG or the validator's code, and the finding delta you ran on the corpus or a real JAR run. "None" is a fine answer. See CONTRIBUTING.md#ai-assisted-contributions. -->

## Checklist

- [ ] Tests added (unit tests required, integration tests for JAR-related changes)
- [ ] New flags also configurable via `fhirlint.yml`
- [ ] `README.md` updated if flag table or usage examples are affected
- [ ] `go test ./...` passes locally
- [ ] `golangci-lint run` passes locally
- [ ] `go mod tidy -diff` reports nothing
- [ ] `TESTING.md` updated if a test layer was added, removed or changed
- [ ] Built-in lint rule or reference check changed: `scripts/rule-diff.sh` summary and precision sample in the description ([docs/corpus-scan.md](../docs/corpus-scan.md))
