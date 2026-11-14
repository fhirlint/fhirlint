# Integrations

fhirlint in a GitHub Actions workflow, as a pre-commit hook and as a Claude Code plugin. For GitLab CI and caching in pipelines, see [CI/CD integration](ci.md) and [Pipeline integration](pipeline.md).

## GitHub Actions

Use the action to validate FHIR resources in one step — it installs fhirlint and runs `validate`:

```yaml
- uses: fhirlint/fhirlint@v1.17.0
  with:
    path: ./fhir/
    fail-on: error
    severity: warning
```

Supported inputs: `path`, `url`, `profile`, `ig`, `severity`, `fail-on`, `format`, `output`, `since`, `tx-offline`, `tx-dir`, `no-repo-config`, and `version` (a release tag or `latest`). `profile` and `ig` accept multiple values, one per line. The action runs on Linux runners (Java 17+ is preinstalled on `ubuntu-latest`).

### Validating only what the pull request changed

`since` scopes the run to files changed against the base branch. It needs the full history, so `actions/checkout` must be told not to make a shallow clone:

```yaml
- uses: actions/checkout@v5
  with:
    fetch-depth: 0          # required: the default shallow clone has no merge base

- uses: fhirlint/fhirlint@v1.17.0
  with:
    path: ./fhir/
    since: origin/${{ github.base_ref }}
```

With `fetch-depth: 0` missing, the action stops with an error naming the fix rather than passing git's own message through.

### Running without a terminology server

`tx-offline` replays a recording made by `fhirlint tx warm`, so the run does not depend on `tx.fhir.org`:

```yaml
- uses: fhirlint/fhirlint@v1.17.0
  with:
    path: ./fhir/
    tx-offline: true
```

Point `tx-dir` at the recording if it is not in the default `.fhirlint-tx/`. See [offline terminology](terminology.md#offline-terminology).

### Inline PR annotations

`--format github` emits [workflow commands](https://docs.github.com/en/actions/reference/workflow-commands-for-github-actions), which the runner turns into annotations shown directly on the offending lines in the PR's "Files changed" view:

```yaml
- uses: fhirlint/fhirlint@v1.17.0
  with:
    path: ./fhir/
    format: github
```

No extra permissions and no upload step — unlike SARIF, which needs `security-events: write` and `upload-sarif` to reach the Security tab. Severities map to `error` (also for `fatal`), `warning` and `notice`; each annotation is titled with the HL7 message id and the FHIRPath expression.

Combine it with a human-readable format in the same run:

```bash
fhirlint validate ./fhir/ --format terminal --format github
```

Annotations must be written to the job's stdout for the runner to see them, so `--output` does not apply to this format.

<details>
<summary>Manual install (any runner / OS)</summary>

```yaml
- name: Install fhirlint
  env:
    GH_TOKEN: ${{ github.token }}
  run: |
    gh release download --repo fhirlint/fhirlint --pattern "*linux_amd64.tar.gz"
    tar xzf fhirlint_*_linux_amd64.tar.gz
    sudo mv fhirlint /usr/local/bin/
    rm fhirlint_*_linux_amd64.tar.gz
```

Or use the Docker image directly in a workflow step:

```yaml
- name: Validate FHIR resources
  uses: docker://ghcr.io/fhirlint/fhirlint:latest
  with:
    args: validate /github/workspace/fhir/ --fail-on error
```

</details>

## Pre-commit hook

fhirlint ships hook definitions for the [pre-commit](https://pre-commit.com/) framework, so resources are validated before they ever reach CI.

```yaml
# .pre-commit-config.yaml
repos:
  - repo: https://github.com/fhirlint/fhirlint
    rev: v1.17.0
    hooks:
      - id: fhirlint
        files: ^input/resources/.*\.json$
```

```bash
pre-commit install
```

Both hooks pass `--skip-non-fhir`, so unrelated files such as `package.json` are skipped instead of failing the commit. Narrowing `files:` to the directory holding your resources is still worthwhile — it keeps the hook from opening every JSON file in the repo on each commit — but it is no longer required to get a usable result. A malformed resource is still validated and still fails, so the hook cannot silently pass a broken file.

Two hook ids are available:

| id | Runs via | Notes |
|----|----------|-------|
| `fhirlint` | `language: golang` | Builds fhirlint from source on first install. Downloads the validator JAR (~250 MB) on the first validation run. |
| `fhirlint-docker` | `language: docker_image` | Uses the prebuilt image with the JAR baked in — no Go toolchain, no first-run JAR download. Requires Docker. The image tag floats, so `rev:` pins the hook definition but not the image contents. |

Both pass the staged files to a single `fhirlint validate` invocation (one JVM start for the whole commit) and default to `args: [--quiet, --skip-non-fhir]`. Setting `args:` yourself replaces that default entirely, so repeat the flags you want to keep:

```yaml
      - id: fhirlint
        files: ^input/resources/.*\.json$
        args: [--quiet, --skip-non-fhir, --profile, kbv-patient, --fail-on, error]
```

## Use as a Claude Code plugin

Validate FHIR resources from inside an agentic [Claude Code](https://claude.com/claude-code) session. The plugin is a thin wrapper around the `fhirlint` binary: Claude runs it, **interprets** the validator's findings, and fixes the offending resource in place.

```text
/plugin marketplace add fhirlint/fhirlint
/plugin install fhirlint@fhirlint
```

This installs two skills:

- **validate** — runs `fhirlint validate`, explains the issues, and proposes/applies fixes to your resources.
- **audit** — runs `fhirlint audit` to check the validator JAR and the IG packages pinned in `fhirlint.lock` for updates and known security advisories.

Just ask Claude to "validate the FHIR resources in this project" and the skill activates.

**Requirements:** the plugin calls the `fhirlint` binary, so it must be on your `PATH` (install via Homebrew or `go install`, above). The first validation downloads the validator JAR (~250 MB) and needs **Java 17+** — the first run may take a minute and is not a hang.

### Auto-validation hook (opt-in)

The plugin also ships a `PostToolUse` hook that validates a FHIR resource **automatically right after Claude edits it** and feeds any errors back into the session, so the model can fix the resource it just touched without you asking.

It is **opt-in** — installing the plugin does not change your editing behaviour. Enable it by setting an environment variable:

```bash
export FHIRLINT_AUTOVALIDATE=1   # in your shell profile
```

or per-project in `.claude/settings.json`:

```json
{
  "env": { "FHIRLINT_AUTOVALIDATE": "1" }
}
```

Unset the variable (or set it to `0`) to disable it again.

The hook is deliberately quiet:

- It runs only on the **file just edited** (`.json`/`.xml`) and only when that file is a FHIR resource (has a `resourceType` / the FHIR namespace), so unrelated files are skipped — no validation storms.
- It surfaces **errors only**. Advisory warnings (e.g. `dom-6`) would nag on every edit; use the **validate** skill or `fhirlint validate` to see those.
- It honours your `fhirlint.yml`, `.fhirlintignore`, and suppression rules, so its output matches the CLI.
- It **stays idle until the validator JAR is cached**, so an automatic edit never triggers a surprise ~250 MB download. Run `fhirlint validate` once (or let the validate skill run) to prime it. (`jq` is also required for the hook.)

### Team rollout

Commit the marketplace and plugin to your repo's `.claude/settings.json` so the whole team gets it on trust:

```json
{
  "extraKnownMarketplaces": {
    "fhirlint": { "source": { "source": "github", "repo": "fhirlint/fhirlint" } }
  },
  "enabledPlugins": { "fhirlint@fhirlint": true },
  "env": { "FHIRLINT_AUTOVALIDATE": "1" }
}
```

Drop the `env` line to ship the skills without the auto-validation hook.
