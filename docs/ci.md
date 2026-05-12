# CI/CD Integration

fhirlint is designed to slot into any CI pipeline with minimal configuration. This guide covers the recommended setup for GitHub Actions and GitLab CI, including caching strategies that keep validation fast on repeated runs.

## Table of contents

- [Basic setup](#basic-setup)
- [Caching the validator JAR](#caching-the-validator-jar)
- [Caching terminology responses](#caching-terminology-responses)
- [Uploading reports as artifacts](#uploading-reports-as-artifacts)
- [JUnit XML test results](#junit-xml-test-results)
- [Project-level config with fhirlint.yml](#project-level-config-with-fhirlintymll)
- [GitLab CI](#gitlab-ci)
- [Exit codes](#exit-codes)
- [Troubleshooting](#troubleshooting)

---

## Basic setup

### GitHub Actions

```yaml
# .github/workflows/fhir-validation.yml
name: FHIR Validation

on:
  push:
    branches: [main]
  pull_request:

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install Java
        uses: actions/setup-java@v4
        with:
          distribution: temurin
          java-version: 17

      - name: Install fhirlint
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          gh release download --repo fhirlint/fhirlint --pattern "*linux_amd64.tar.gz"
          tar xzf fhirlint_*_linux_amd64.tar.gz
          sudo mv fhirlint /usr/local/bin/
          rm fhirlint_*_linux_amd64.tar.gz

      - name: Validate FHIR resources
        run: fhirlint validate ./fhir/ --fail-on error
```

This is the minimal working setup. Read on for caching strategies that make repeated runs significantly faster.

---

## Caching the validator JAR

The HL7 FHIR Validator JAR is ~250 MB. Without caching it is re-downloaded on every run. Cache it by pinning the validator version and storing `~/.fhirlint/`:

```yaml
      - name: Cache validator JAR
        uses: actions/cache@v4
        with:
          path: ~/.fhirlint
          key: fhirlint-jar-${{ runner.os }}-${{ hashFiles('fhirlint.yml') }}
          restore-keys: |
            fhirlint-jar-${{ runner.os }}-

      - name: Validate FHIR resources
        run: fhirlint validate ./fhir/ --fail-on error
```

The cache key includes `fhirlint.yml` so a version bump there invalidates the JAR cache.

Alternatively, check in a pre-downloaded JAR and point fhirlint at it:

```yaml
      - name: Validate FHIR resources
        env:
          FHIRLINT_JAR: ./tools/validator_cli.jar
        run: fhirlint validate ./fhir/ --fail-on error
```

---

## Caching terminology responses

The validator resolves code system lookups against `https://tx.fhir.org`. These responses can be cached between runs with `--tx-cache`, which is often the biggest source of per-run latency after the JAR itself.

> **Important:** `tx.fhir.org` is a public development server that HL7 explicitly states is not provisioned for CI or production use. It has no SLA and can go down without notice. For CI, either cache responses with `--tx-cache` (recommended) or disable the terminology server entirely with `--no-terminology-server`. For production systems, run your own terminology server.

```yaml
      - name: Cache terminology responses
        uses: actions/cache@v4
        with:
          path: .fhirlint-tx-cache
          key: fhirlint-tx-${{ runner.os }}-${{ hashFiles('fhirlint.yml') }}
          restore-keys: |
            fhirlint-tx-${{ runner.os }}-

      - name: Validate FHIR resources
        run: fhirlint validate ./fhir/ --tx-cache .fhirlint-tx-cache/ --fail-on error
```

Add `.fhirlint-tx-cache/` to `.gitignore` to keep the cache out of the repository.

For air-gapped environments or pipelines where no external calls are acceptable, disable the terminology server entirely:

```yaml
      - name: Validate FHIR resources
        run: fhirlint validate ./fhir/ --no-terminology-server --fail-on error
```

Note: `--no-terminology-server` skips all code system validation, which may hide real issues. Prefer `--tx-cache` when possible.

### Full optimised workflow

Combining both caches gives the fastest possible runs:

```yaml
name: FHIR Validation

on: [push, pull_request]

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-java@v4
        with:
          distribution: temurin
          java-version: 17

      - name: Install fhirlint
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          gh release download --repo fhirlint/fhirlint --pattern "*linux_amd64.tar.gz"
          tar xzf fhirlint_*_linux_amd64.tar.gz && sudo mv fhirlint /usr/local/bin/
          rm fhirlint_*_linux_amd64.tar.gz

      - name: Cache validator JAR
        uses: actions/cache@v4
        with:
          path: ~/.fhirlint
          key: fhirlint-jar-${{ runner.os }}-${{ hashFiles('fhirlint.yml') }}
          restore-keys: fhirlint-jar-${{ runner.os }}-

      - name: Cache terminology responses
        uses: actions/cache@v4
        with:
          path: .fhirlint-tx-cache
          key: fhirlint-tx-${{ runner.os }}-${{ hashFiles('fhirlint.yml') }}
          restore-keys: fhirlint-tx-${{ runner.os }}-

      - name: Validate FHIR resources
        run: |
          fhirlint validate ./fhir/ \
            --tx-cache .fhirlint-tx-cache/ \
            --format json --output fhir-report.json \
            --fail-on error

      - name: Upload validation report
        uses: actions/upload-artifact@v4
        if: always()
        with:
          name: fhir-validation-report
          path: fhir-report.json
```

---

## Uploading reports as artifacts

fhirlint can produce JSON and HTML reports alongside the terminal output. Use `if: always()` so the report is uploaded even when validation fails — that is exactly when you need it most.

```yaml
      - name: Validate FHIR resources
        run: |
          fhirlint validate ./fhir/ \
            --format terminal \
            --format html --output fhir-report.html \
            --fail-on error

      - name: Upload HTML report
        uses: actions/upload-artifact@v4
        if: always()
        with:
          name: fhir-validation-report
          path: fhir-report.html
          retention-days: 30
```

The HTML report can be downloaded from the Actions run summary and opened in any browser — no server required.

---

## JUnit XML test results

`--format junit` outputs results in JUnit XML format. GitHub Actions, Jenkins, Azure DevOps, and GitLab CI can all consume this format natively to display per-file pass/fail results in their test dashboards — no custom plugin needed.

Each validated file becomes a `<testcase>`. Issues at or above `--severity` become `<failure>` elements. Files with no issues are recorded as passing test cases.

```yaml
- name: Validate FHIR resources
  run: fhirlint validate ./fhir/ --format junit --output fhir-results.xml

- name: Publish test results
  uses: mikepenz/action-junit-report@v4
  if: always()
  with:
    report_paths: fhir-results.xml
```

Combine with terminal output to see results in the log and in the test dashboard at the same time:

```yaml
- name: Validate FHIR resources
  run: |
    fhirlint validate ./fhir/ \
      --format terminal \
      --format junit --output fhir-results.xml \
      --fail-on error
```

---

## Project-level config with `fhirlint.yml`

Instead of repeating flags on every CI step, commit a `fhirlint.yml` to your repository root. All team members and CI runs share the same defaults:

```yaml
# fhirlint.yml
fhir-version: 4.0.1
severity: warning
fail-on: error

ig:
  - kbv.basis#1.5.0

tx-cache: .fhirlint-tx-cache/

suppress:
  - constraint: dom-6

format:
  - terminal
```

With this file in place the CI step simplifies to:

```yaml
      - name: Validate FHIR resources
        run: fhirlint validate ./fhir/
```

CLI flags always override `fhirlint.yml` values, so you can still pass `--fail-on never` locally while the config enforces `fail-on: error` in CI.

---

## GitLab CI

```yaml
# .gitlab-ci.yml
fhir-validation:
  image: eclipse-temurin:17
  stage: test
  cache:
    key: fhirlint-$CI_COMMIT_REF_SLUG
    paths:
      - .fhirlint-jar/
      - .fhirlint-tx-cache/
  variables:
    FHIRLINT_JAR: .fhirlint-jar/validator_cli.jar
  before_script:
    - apt-get update -qq && apt-get install -qq curl
    - |
      FHIRLINT_VERSION=$(curl -s https://api.github.com/repos/fhirlint/fhirlint/releases/latest \
        | grep '"tag_name"' | cut -d'"' -f4)
      curl -sL "https://github.com/fhirlint/fhirlint/releases/download/${FHIRLINT_VERSION}/fhirlint_${FHIRLINT_VERSION#v}_linux_amd64.tar.gz" \
        | tar xz && mv fhirlint /usr/local/bin/
    - mkdir -p .fhirlint-jar .fhirlint-tx-cache
  script:
    - fhirlint validate ./fhir/ --tx-cache .fhirlint-tx-cache/ --fail-on error
  artifacts:
    when: always
    paths:
      - fhir-report.json
    expire_in: 30 days
```

---

## Exit codes

| Exit code | Meaning |
|-----------|---------|
| `0` | Validation passed (or `--fail-on never`) |
| `1` | Validation found issues at or above the `--fail-on` threshold |
| `2` | fhirlint itself failed (missing Java, JAR download error, invalid flag, etc.) |

Use `--fail-on warning` to catch warnings in CI as well. Use `--fail-on never` for reporting-only runs that should never break the build.

---

## Troubleshooting

**Validation is slow on every run**
Cache both `~/.fhirlint` (JAR) and `.fhirlint-tx-cache/` (terminology). Without the tx-cache, every run re-fetches code system lookups from `tx.fhir.org`.

**"Could not download JAR" in CI**
The runner may not have outbound HTTPS access. Either cache the JAR via `actions/cache`, commit it to the repository, or use `FHIRLINT_JAR` to point to a pre-staged file.

**"java: command not found"**
fhirlint requires Java 11+. Add `actions/setup-java` (GitHub Actions) or use a JDK base image (GitLab CI).

**Different results between local and CI**
The most common cause is a different JAR version or a cold terminology cache. Pin the JAR version in `fhirlint.yml` and use `--tx-cache` with a persistent cache store.

**Terminology server errors or flaky validation in CI**
`tx.fhir.org` is not provisioned for CI use and can be unavailable. Use `--tx-cache` to reuse cached responses across runs, or switch to `--no-terminology-server` if terminology validation is not required. For reliable CI, consider running your own terminology server.
