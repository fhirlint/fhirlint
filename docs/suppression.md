# Suppression rules

FHIR validation is strict by design — but real projects sometimes make deliberate, accepted deviations from the spec. This guide explains when and how to use suppression rules to silence those specific issues without disabling validation broadly.

## Table of contents

- [When to suppress vs. when to fix](#when-to-suppress-vs-when-to-fix)
- [Selector types](#selector-types)
- [CLI usage](#cli-usage)
- [Committing rules to fhirlint.yml](#committing-rules-to-fhirlintymll)
- [Re-levelling instead of suppressing](#re-levelling-instead-of-suppressing)
- [--show-suppressed: making decisions visible](#--show-suppressed-making-decisions-visible)
- [Stale rule warnings](#stale-rule-warnings)
- [Sharing rules with the validator (advisor file)](#sharing-rules-with-the-validator-advisor-file)
- [Suppression in JSON output](#suppression-in-json-output)
- [Common suppressions](#common-suppressions)

---

## When to suppress vs. when to fix

Suppression is the right tool when:

- **The deviation is intentional and accepted** — for example, a required field that does not apply in your context (`MedicationRequest.intent` for a dispensing system that has no concept of intent)
- **The constraint comes from a base profile that your derived profile intentionally relaxes** — the validator flags it correctly, but your team has signed off on the deviation
- **You are validating against a terminology system that is not yet published** — `dom-6` narrative warnings that are known and acceptable while the IG is in development
- **You own the code and cannot change it right now** — suppression gives you a clean build while you track the real fix

Suppression is the wrong tool when:

- **You just want to make the build pass** — suppressing unknown errors to silence CI is technical debt in disguise
- **The issue points to a real data quality problem** — a missing required field is usually a bug, not a policy decision
- **You want to silence a whole category of issues** — use `--best-practice ignore` for best-practice constraints or `--severity warning` to raise the display threshold instead
- **You want the finding to stay visible, just not fatal** — that is [re-levelling](#re-levelling-instead-of-suppressing), not suppression

---

## Selector types

fhirlint supports three ways to identify an issue to suppress:

### `messageId` — by HL7 message ID (most precise)

Every issue the HL7 validator emits has a stable message ID. fhirlint shows it in JSON output as `messageId`. Use this when you want to suppress exactly one specific message type:

```bash
fhirlint validate patient.json --suppress messageId:Measure_M_POPULATIONIDENTIFIER
```

Find the message ID with `--format json`:

```bash
fhirlint validate patient.json --format json | jq '.files[].issues[].messageId'
```

### `constraint` — by FHIR constraint ID

FHIR constraint IDs (like `dom-6`, `bdl-1`, `obs-7`) are the identifiers defined in FHIR profiles. They are typically the same as the `messageId` for constraint violations, but using `constraint:` makes the intent clearer:

```bash
fhirlint validate patient.json --suppress constraint:dom-6
```

### `expression` — by field path

Suppresses all issues on a specific FHIR path. Useful when a whole field is intentionally non-conformant:

```bash
fhirlint validate prescription.json --suppress expression:MedicationRequest.intent
```

This matches any issue whose location starts with `MedicationRequest.intent` — including nested paths like `MedicationRequest.intent.value`.

### `pattern` — by regex match on message text

Suppresses any issue whose full message text matches a regular expression. Useful when a message varies slightly across resources or validator versions and cannot be pinned to a stable `messageId`:

```bash
fhirlint validate resources/ --suppress "pattern:.*example\\.org.*"
```

```yaml
suppress:
  - pattern: ".*example\\.org.*"         # suppress all messages mentioning example.org
  - pattern: "Unknown extension.*"
    severity: warning                     # optional: only at this severity
```

Invalid regex patterns are rejected at startup with a clear error — not silently at match time.

`messageId`, `constraint`, and `expression` are preferred when available — `pattern` is the right tool only when the other selectors cannot target the issue precisely enough.

### Optional severity filter

All four selectors accept an optional severity qualifier in `fhirlint.yml`:

```yaml
suppress:
  - expression: Patient.text
    severity: warning   # only suppress warnings, not errors, on this field
```

The CLI format does not support a severity qualifier — use `fhirlint.yml` for that.

---

## CLI usage

`--suppress` is repeatable. Combine multiple rules in a single invocation:

```bash
fhirlint validate patient.json \
  --suppress constraint:dom-6 \
  --suppress expression:MedicationRequest.intent \
  --suppress messageId:Terminology_TX_NoValid_17
```

---

## Committing rules to `fhirlint.yml`

The most important use of suppression is committing accepted deviations to version control, making them explicit and reviewable. A suppression rule in `fhirlint.yml` is a documented decision — it shows up in git history, in PRs, and in code review.

```yaml
# fhirlint.yml
suppress:
  # dom-6 narrative warnings accepted project-wide: our IG explicitly does not
  # require human-readable narrative for machine-to-machine interfaces.
  - constraint: dom-6

  # MedicationRequest.intent is not applicable in our dispensing context.
  # Tracked in: https://github.com/my-org/fhir-profiles/issues/42
  - expression: MedicationRequest.intent

  # Internal drug catalogue not yet published — suppress terminology warnings
  # until IG package is released (milestone: Q3 2026).
  - messageId: Terminology_TX_NoValid_17
    severity: warning
```

Adding a comment above each rule — with a rationale and ideally a tracking link — turns suppression from a black box into a living decision log.

---

## Re-levelling instead of suppressing

Suppression answers one question: should this finding be visible? Sometimes the honest answer is "yes, but it must not fail the build". The classic case is an error caused by a defect in a profile someone else publishes — `DAV-PR-ERP-AbgabedatenBundle|1.0.3` shipping mutually inconsistent constraints, for instance. You cannot fix it, you cannot wait for it, and if you suppress it you will never notice when it is finally corrected.

`severity-override:` re-levels the finding instead:

```yaml
# fhirlint.yml
severity-override:
  - messageId: Rule_bdl_1
    severity: warning
    reason: "constraint is wrong in DAV-PR-ERP-AbgabedatenBundle 1.0.3"
    expires: 2026-12-31
```

The selectors are the ones above — `messageId`, `constraint`, `expression`, `pattern` — but `severity` means the opposite of what it means under `suppress:`. There it narrows which findings a rule matches; here it is the level to apply, and it is required.

| | `suppress:` | `severity-override:` |
|---|---|---|
| Effect | hides the finding | changes its level |
| `severity:` key | which findings to match | the level to apply (required) |
| Appears in output | only with `--show-suppressed` | always, at its new level |
| CLI equivalent | `--suppress` | none — config only |
| Counts toward exit code | no | yes, at the new level |

Upgrades work the same way, for the finding your project treats as more serious than the spec does:

```yaml
severity-override:
  - pattern: "Unknown extension.*"
    severity: error
    reason: "extensions must be declared in our IG"
```

The new level is the real one everywhere downstream: terminal output, `--severity` filtering, `--fail-on`, `--max-warnings`, the baseline and the exit code. Overrides are applied **before** suppression, so a `suppress:` rule with a severity filter matches against the new level, not the reported one.

Nothing is lost in the process. The terminal marks a re-levelled finding, and JSON and SARIF carry both levels:

```
  ⚠ WARN   Rule bdl-1: A Bundle entry must have a fullUrl
           ↕ reported as error
```

```json
{ "severity": "warning", "originalSeverity": "error", "messageId": "Rule_bdl_1" }
```

In SARIF the original travels as `properties.originalSeverity` on the result, so a code-scanning dashboard can still tell the two apart.

`reason` and `expires` behave exactly as they do for suppressions, including the stale-rule and expiry warnings below, and `require-suppress-reason` covers these rules too.

---

## `--show-suppressed`: making decisions visible

By default, suppressed issues are completely hidden from terminal output. Use `--show-suppressed` to display them with a muted `↷ SUPP` label:

```bash
fhirlint validate patient.json --show-suppressed
```

```
▶ patient.json
  ✗ ERROR  Patient.name: minimum required = 1, but only found 0
  ↷ SUPP   Best Practice Recommendation: In general, all observations should
           have a performer
           @ Patient (line 1, col 2)

────────────────────────────────────────
Files: 1  Valid: 0  Errors: 1  Warnings: 0  Suppressed: 1
```

This is useful for:
- **PR reviews** — showing reviewers exactly what is being suppressed and why
- **Audits** — a quick check that suppressions are still relevant
- **Debugging** — confirming that a suppression rule is actually matching

`--show-suppressed` can also be set in `fhirlint.yml`:

```yaml
show-suppressed: true
```

---

## Stale rule warnings

When a suppression rule matches zero issues across all validated resources, fhirlint prints a warning to stderr:

```
warn: suppress rule "constraint:dom-6" matched 0 issues
```

This happens when:
- The underlying issue was fixed in the resource
- The resource type changed and the path no longer exists
- The message ID was renamed in a new validator version

Stale rules should be removed — a suppression that matches nothing is misleading noise. Treat this warning as a prompt to review whether the accepted deviation still applies.

---

## Sharing rules with the validator (advisor file)

Suppression rules are applied by fhirlint, after the JAR has produced its output. That covers every run that goes through fhirlint — and nothing else. The raw `java -jar validator_cli.jar` someone runs to reproduce a finding, and the IG Publisher build next to it, still report everything the project has already decided to accept.

The validator has its own mechanism for this: `-advisor-file`, which the IG Publisher accepts too. Export yours from `fhirlint.yml`:

```bash
fhirlint suppress export -o advisor.json

java -jar validator_cli.jar patient.json -advisor-file advisor.json
```

The file is small — the format has exactly one key:

```json
{
  "suppress": [
    "UNABLE_TO_INFER_CODESYSTEM",
    "*@Patient.name",
    "*@Patient.name.*"
  ]
}
```

### What survives the conversion

The advisor format filters messages by message ID and element path, and knows nothing else. So:

| Rule | Exported as | |
|------|-------------|---|
| `messageId: X` | `"X"` | ✅ |
| `expression: Patient.name` | `"*@Patient.name"` and `"*@Patient.name.*"` | ✅ |
| `constraint: dom-6` | — | ❌ |
| `pattern: <regex>` | — | ❌ |
| any rule with a `severity:` filter | — | ❌ |
| `severity-override:` rules | — | ❌ |
| rules under `overrides:` | — | ❌ |

An `expression` rule needs two entries because an advisor path matches only its own depth unless its last segment is `*` — one entry covers the element, the other its descendants. `*` as the message ID matches any message, which is what an expression rule means.

`constraint` cannot be expressed: fhirlint matches the short key against the `#dom-6` **suffix** of a URI message ID (`http://hl7.org/fhir/StructureDefinition/DomainResource#dom-6`), and the advisor matches IDs by equality or a trailing-`*` prefix. `pattern` matches the message text, while the advisor's regex applies to the path — same syntax, different subject.

`reason` and `expires` have nowhere to go in the format, so an exported rule loses them. An **expired** rule is not exported at all: writing it into a file with no expiry mechanism would quietly bring it back to life.

Every rule that is dropped is listed on stderr with the reason:

```
Exported 1 of 2 suppression rule(s) as 1 advisor entry.

Not exported — no advisor equivalent:
  constraint:dom-6
      the advisor matches message ids by prefix, not by the #constraint suffix fhirlint uses
```

Add `--strict` to exit non-zero when anything was dropped — useful as a CI check that the exported file and the config have not drifted apart. The file is still written; `--strict` reports that it is incomplete, it does not refuse to produce it.

### What it does not buy

Only shared rules. The JSON advisor implements one hook upstream — message filtering by ID and path. It does not change severities, and it does not skip validation work, so exporting will not make a run faster.

---

## Suppression in JSON output

In `--format json`, suppressed issues appear in a separate `suppressed` array per file alongside the active `issues`. This gives you a complete audit trail without cluttering the main issue list:

```json
{
  "files": [
    {
      "filename": "patient.json",
      "valid": true,
      "issues": [],
      "suppressed": [
        {
          "severity": "warning",
          "message": "Best Practice Recommendation: ...",
          "location": "Patient (line 1, col 2)",
          "messageId": "dom-6"
        }
      ]
    }
  ]
}
```

This is useful for compliance reporting: you can show that validation ran, that specific issues were found, and that they were explicitly suppressed rather than silently ignored.

---

## Common suppressions

### dom-6 — narrative warnings

`dom-6` fires when a FHIR resource has no human-readable `text` narrative. Many machine-to-machine interfaces legitimately omit narrative.

```yaml
suppress:
  - constraint: dom-6
```

Alternatively, use `--best-practice ignore` to silence *all* best-practice constraints at once — but prefer the targeted suppression if dom-6 is the only one you want to silence.

### Terminology warnings for internal code systems

While an internal code system is not yet published as an IG package, the validator will warn about unknown codes. Use `--codesystem` to load the definition locally (see [Profiles & implementation guides](../README.md#profiles--implementation-guides)), or suppress the warnings during the transition period:

```yaml
suppress:
  - messageId: Terminology_TX_NoValid_17
    severity: warning
```

### Required fields not applicable in context

When a base profile requires a field that your derived profile intentionally omits:

```yaml
suppress:
  - expression: MedicationRequest.intent
```

Document the rationale alongside the rule in `fhirlint.yml` — future maintainers will need to know why this was accepted.
