# Baseline mode

Baseline mode allows incremental adoption of fhirlint on codebases that already have validation issues. Instead of fixing every pre-existing issue before you can enable CI enforcement, you capture the current state as a baseline and from that point forward only fail on **new** issues (regressions).

## Table of contents

- [When to use baseline mode](#when-to-use-baseline-mode)
- [Generating a baseline](#generating-a-baseline)
- [Running with a baseline](#running-with-a-baseline)
- [Stale baseline entries](#stale-baseline-entries)
- [CI workflow](#ci-workflow)
- [Configuration file](#configuration-file)
- [Seeing what is suppressed](#seeing-what-is-suppressed)
- [Baseline vs. suppress rules](#baseline-vs-suppress-rules)

---

## When to use baseline mode

- **Incremental adoption**: You have an existing codebase with many FHIR resources and pre-existing issues. You want CI enforcement from today without blocking the pipeline until all issues are resolved.
- **Technical debt tracking**: Baseline makes existing issues visible (they are committed to version control) while preventing regressions as you work through the backlog.
- **New IG versions**: When you upgrade an IG package, new issues may appear across the codebase. Generate a baseline to capture them, then address them incrementally.

If an issue is truly an accepted permanent deviation from the spec, use [`--suppress`](suppression.md) instead.

---

## Generating a baseline

Run `--generate-baseline` to capture all current issues to a JSON file. The build always exits `0` during baseline generation — it is a snapshot, not a validation gate.

```bash
fhirlint validate ./fhir/ --generate-baseline fhirlint-baseline.json
```

The baseline file records each issue by resource file, line/column, message ID, and expression path, so it can match the same issue across future runs even if other issues in the file change.

Commit `fhirlint-baseline.json` to version control. This makes the suppressed issues transparent and reviewable.

---

## Running with a baseline

Pass `--baseline` to suppress issues recorded in the baseline. Only issues that are **not** in the baseline are subject to `--fail-on`:

```bash
fhirlint validate ./fhir/ --baseline fhirlint-baseline.json
```

- Issues present in the baseline: suppressed (excluded from exit code, hidden from terminal output by default).
- Issues **not** in the baseline: reported and evaluated against `--fail-on` as usual.

The summary always shows the full counts, including suppressed issues.

---

## Stale baseline entries

When you fix an issue that was in the baseline, the baseline entry for that issue no longer matches anything. fhirlint detects this and emits a warning:

```
warn: 3 baseline occurrence(s) no longer found — regenerate the baseline to remove stale entries
```

This is expected and harmless. Regenerate the baseline once you have fixed a batch of issues to keep it up to date:

```bash
fhirlint validate ./fhir/ --generate-baseline fhirlint-baseline.json
```

Commit the updated baseline so the team can see what was resolved.

---

## CI workflow

A typical setup:

1. **Initial setup** — generate the baseline and commit it:

   ```bash
   fhirlint validate ./fhir/ --generate-baseline fhirlint-baseline.json
   git add fhirlint-baseline.json
   git commit -m "chore: add fhirlint baseline"
   ```

2. **CI pipeline** — run with the baseline on every push:

   ```yaml
   - name: Validate FHIR resources
     run: fhirlint validate ./fhir/ --baseline fhirlint-baseline.json --fail-on error
   ```

3. **When making intentional changes** (new IG, deliberate spec deviation) — regenerate:

   ```bash
   fhirlint validate ./fhir/ --generate-baseline fhirlint-baseline.json
   git add fhirlint-baseline.json
   git commit -m "chore: update fhirlint baseline after IG upgrade"
   ```

4. **As you fix issues** — the baseline shrinks. Regenerate periodically and commit the smaller baseline so progress is visible.

---

## Configuration file

Set the baseline path in `fhirlint.yml` so you don't have to pass `--baseline` on every run:

```yaml
# fhirlint.yml
baseline: fhirlint-baseline.json
fail-on: error
```

CLI flags always take precedence — you can override or disable the baseline for a single run:

```bash
# Temporarily ignore the baseline (e.g. to audit all current issues)
fhirlint validate ./fhir/ --baseline ""
```

---

## Seeing what is suppressed

By default, baseline-suppressed issues are hidden from terminal output. Use `--show-suppressed` to make them visible with a muted `↷ SUPP` label:

```bash
fhirlint validate ./fhir/ --baseline fhirlint-baseline.json --show-suppressed
```

In JSON and HTML output, suppressed issues are always included in a separate `suppressed` array regardless of `--show-suppressed`.

---

## Baseline vs. suppress rules

| | `--suppress` | Baseline (`--baseline`) |
|---|---|---|
| Purpose | Accepted, permanent deviation | Technical debt — plan to fix eventually |
| Scope | Specific message ID, constraint, or expression path | Any issue present when the baseline was generated |
| Committed to VCS | Yes (in `fhirlint.yml`) | Yes (`fhirlint-baseline.json`) |
| Stale detection | Warns if a rule matches 0 issues | Warns if baseline entries no longer match |
| Regeneration | Manual edit of `fhirlint.yml` | `--generate-baseline` |

Use `--suppress` when you have made a deliberate architectural decision (e.g. a IG deviation that your project intentionally does not follow). Use baseline mode when you have issues you intend to fix but cannot address immediately.
