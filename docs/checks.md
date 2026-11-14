# Suppressions and fhirlint's own checks

Suppressing known issues, custom FHIRPath rules, the built-in style and naming rules, the reference check, and baselines.

## Suppressing known issues

When a deviation from the FHIR spec is intentional and accepted, use `--suppress` to silence the issue without disabling all failure detection:

```bash
# Suppress by HL7 message ID (most precise)
fhirlint validate patient.json --suppress messageId:dom-6

# Suppress by FHIR constraint ID
fhirlint validate patient.json --suppress constraint:dom-6

# Suppress all issues on a specific field path
fhirlint validate prescription.json --suppress expression:MedicationRequest.intent

# Show suppressed issues with a muted label instead of hiding them
fhirlint validate patient.json --suppress messageId:dom-6 --show-suppressed
```

### Expiring suppressions

A suppression added to work around a temporary problem tends to outlive it, quietly hiding findings nobody revisits. Give it a deadline in `fhirlint.yml`:

```yaml
suppress:
  - constraint: dom-6
    reason: "narrative added in the next sprint"
    expires: 2026-12-31
```

The date is **inclusive**: the rule suppresses throughout 2026-12-31 and lapses on 2027-01-01.

Once it lapses the rule stops suppressing — the findings come back, and if they are errors the build fails again. That is the point of the feature, so fhirlint says exactly what happened rather than leaving a mysterious failure:

```text
warn: suppress rule "constraint:dom-6" expired on 2026-12-31 and no longer suppresses anything
```

For the 14 days before, it warns while still suppressing, so the lapse is not a surprise on the day:

```text
warn: suppress rule "constraint:dom-6" expires on 2026-12-31
```

An unparseable date is an error rather than a silently ignored field — a typo must not turn a temporary suppression into a permanent one:

```text
Error: invalid suppress expires "31.12.2026": use YYYY-MM-DD
```

Expiry is config-only. `--suppress` on the command line is for one-off runs, where a deadline has little meaning.

### Requiring a reason

`reason` is optional, so a rule can silence a finding with nothing recorded about why. For projects where an unexplained suppression is hard to defend later, make it mandatory:

```yaml
require-suppress-reason: true
```

A rule without a reason then fails the run, naming the offenders:

```text
Error: require-suppress-reason is set, but 1 suppress rule(s) have no reason: "constraint:dom-6"
```

It is opt-in, since turning it on by default would break every config with a bare suppression. There is a `--require-suppress-reason` flag for enabling it from the command line, e.g. to enforce the policy in CI without changing the committed config.

The check covers **every** place a suppression can be written — `--suppress` on the command line, the global `suppress:` block, `suppress:` nested under `overrides:`, and the `severity-override:` rules described below. A policy that only covered one of them could be sidestepped by moving the rule somewhere else. Whitespace does not count as a reason.

Suppressed issues are:

- Excluded from the exit-code calculation (won't trigger `--fail-on error`)
- Hidden from terminal output by default; shown with `↷ SUPP` when `--show-suppressed` is set
- Included in a separate `suppressed` array in JSON/HTML output for auditability

A rule that matches nothing emits a warning (`suppress rule "X" matched 0 issues`) to catch stale suppressions.

Suppression rules can also be committed to `fhirlint.yml` to make accepted deviations explicit and reviewable:

```yaml
suppress:
  - constraint: dom-6
  - expression: MedicationRequest.intent
  - expression: Patient.text
    severity: warning   # only suppress warnings on this field
```

### Re-levelling instead of hiding

Suppression is all-or-nothing. Some findings deserve the middle option: an error caused by a defect in someone else's published profile will not be fixed on your schedule, must not fail CI, and must not disappear either — silently dropping it means nobody notices when the profile is eventually corrected.

`severity-override:` changes what a finding *is*, using the same selectors as `suppress`:

```yaml
severity-override:
  - messageId: Rule_bdl_1
    severity: warning     # the level to apply — required
    reason: "constraint is wrong in DAV-PR-ERP-AbgabedatenBundle 1.0.3"
    expires: 2026-12-31

  - pattern: "Unknown extension.*"
    severity: error       # upgrades work too
    reason: "extensions must be declared in our IG"
```

Note the difference in what `severity` means: under `suppress:` it narrows *which* findings a rule matches, while under `severity-override:` it is the level to apply. That is why it is mandatory here — a rule without one says nothing.

The new level is the real one from that point on. It drives terminal output, `severity` filtering, `--fail-on`, `--max-warnings`, the baseline and the exit code, so a downgraded error genuinely stops failing the build instead of merely looking different. Rules run **before** suppression, so a `suppress` rule with a `severity` filter matches against the new level.

Reports do not lose the original. Terminal output marks a re-levelled finding, and JSON and SARIF carry both levels, so a report cannot mislead a reader who does not have the config to hand:

```text
  ⚠ WARN   Rule bdl-1: A Bundle entry must have a fullUrl
           ↕ reported as error
```

```json
{ "severity": "warning", "originalSeverity": "error", "messageId": "Rule_bdl_1", ... }
```

`reason` and `expires` behave exactly as they do for suppressions, including the warning when a rule lapses or matches nothing, and `require-suppress-reason` covers these rules too — a downgrade stops a finding failing the build just as effectively as hiding it does. Config only: a re-levelling carries a reason and usually a date, which is not something to retype on every invocation.

### Sharing the rules with the validator

Suppression rules only hold for runs that go through fhirlint. A raw `validator_cli.jar` run, or an IG Publisher build, still reports everything the project has accepted. Both read the validator's own advisor file, so export yours:

```bash
fhirlint suppress export -o advisor.json

java -jar validator_cli.jar patient.json -advisor-file advisor.json
```

The advisor format filters by message ID and element path only, so `messageId` and `expression` rules export while `constraint`, `pattern` and severity-filtered rules cannot. Every dropped rule is listed with the reason, and `--strict` turns that into a non-zero exit for CI. See the [suppression guide](suppression.md#sharing-rules-with-the-validator-advisor-file) for the full mapping.

## Custom lint rules

Profile validation checks conformance to a StructureDefinition, but it cannot express **project-specific** expectations such as "every `Patient` must carry an MRN identifier". Custom lint rules fill that gap: each rule asserts a [FHIRPath](https://hl7.org/fhirpath/) expression over every validated resource, and a resource that fails the assertion produces a finding that flows through the normal reporters, severity filter, suppression, and baseline machinery.

Define rules under a `rules:` key in `fhirlint.yml`, or in a separate file passed with `--rules-file`:

```yaml
rules:
  - id: patient-needs-mrn
    resource: Patient        # optional: only applies to this resourceType
    assert: "identifier.where(system='http://hospital.example/mrn').exists()"
    message: "Patient is missing an MRN identifier"
    severity: error          # error | warning | information (default: error)

  - id: no-example-refs
    assert: "descendants().reference.exists() implies reference.startsWith('http://example.org').not()"
    severity: warning
```

```bash
# Rules from fhirlint.yml are applied automatically
fhirlint validate ./fhir/

# Or load them from a dedicated file
fhirlint validate ./fhir/ --rules-file lint-rules.yml
```

Each rule failure is reported with the message ID `rule:<id>`, so it can be suppressed like any other issue:

```bash
fhirlint validate patient.json --suppress messageId:rule:patient-needs-mrn
```

Rules are evaluated in-process against FHIR **JSON** (R4/R4B/R5) — no JVM round-trip — so they add negligible overhead even across large directories. XML resources are skipped with a notice. A malformed or unsupported expression is reported when rules are loaded (and by `fhirlint config check`), not silently ignored.

See the [Custom lint rules guide](rules.md) for the full list of supported FHIRPath functions and operators.

## Style & naming rules

Profile validation checks conformance to the FHIR spec, but it does not enforce **authoring conventions** — lowercase-hyphen resource ids, a house canonical-URL base, PascalCase profile names. Built-in lint rules add those checks. They are **opt-in**: nothing runs unless you enable it under the `lint:` key in `fhirlint.yml`, and each rule carries its own severity.

```yaml
lint:
  id-kebab-case: warning              # resource id should be lowercase kebab-case
  profile-name-pascalcase: warning    # StructureDefinition.name should be PascalCase
  canonical-url-pattern:              # canonical url must start with a base
    severity: error
    base: "https://example.org/fhir/"
```

Findings carry the message ID `lint:<rule>`, so they behave like any other issue — shown in every report, filtered by `--severity`, gated by `--fail-on`, and suppressible or baselineable:

```bash
fhirlint validate ./fhir/ --suppress messageId:lint:id-kebab-case
```

These rules catch conventions the validator itself accepts. For example the id `ExamplePatient1` is a valid FHIR id (the JAR reports no error), but `id-kebab-case` flags it as not lowercase-hyphenated. Rules run in-process against FHIR **JSON**; XML resources are skipped with a notice.

See the [Style & naming rules guide](lint-rules.md) for the full rule list and options. For **project-specific** assertions beyond these built-ins, see custom FHIRPath rules (`rules:` / `--rules-file`).

## Referential integrity

Profile validation checks each resource in isolation — it does not catch a **dangling reference**, where a `reference` points at a resource that is not present in the dataset. Pass `--check-references` to validate the reference graph across the whole input set (a directory, a Bundle, or NDJSON):

```bash
fhirlint validate ./fhir/ --check-references
```

fhirlint indexes every resource's identity (`ResourceType/id`, and Bundle entry `fullUrl`s) and then resolves each literal reference against that index:

- **Local references** — a relative `Patient/123`, a `urn:uuid:…` matching a Bundle entry, or a contained `#id` — that do not resolve are reported as an **error** with message ID `ref:unresolved`.
- **Absolute references** to a server outside the set (`https://other.example/fhir/Patient/1`) cannot be checked and are reported as **information** with message ID `ref:external`.
- References by `identifier` only (no literal `.reference`) and canonical URLs are not reference-resolved.

The check runs in-process (no JVM round-trip) and findings behave like any other issue — filtered by `--severity`, gated by `--fail-on`, and suppressible or baselineable:

```bash
fhirlint validate ./fhir/ --check-references --suppress messageId:ref:external
```

It is opt-in: when you validate a subset of a dataset, references to resources you did not include will (correctly) report as unresolved. XML resources are skipped with a notice.

### Overlap with the validator's own warning

The validator reports one reference problem by itself, with or without this flag: a `urn:uuid:` reference that no entry of the enclosing Bundle carries, as a warning with message ID `Bundle_BUNDLE_Not_Local`. With `--check-references` that is the same finding twice, so fhirlint keeps the `ref:unresolved` error and drops the warning — it says what was searched, and it is an error rather than a warning.

The warning is dropped only for references this check itself flagged. `Bundle_BUNDLE_Not_Local` is about one Bundle, while the check spans the whole validated set, so a `urn:uuid:` that resolves against a different file in the run still produces the validator's warning and no error — "not local to this bundle" is then the only thing anyone is saying, and it stays.

Without `--check-references` nothing changes: the validator's warning is the only reference finding there is.

## Baseline mode

When adopting fhirlint on an existing codebase, there may be many pre-existing issues that you can't fix all at once. Baseline mode lets you capture the current state and only fail the build on new regressions.

```bash
# Generate a baseline from current issues (the build always succeeds here)
fhirlint validate ./fhir/ --generate-baseline fhirlint-baseline.json

# Use the baseline: only new issues (regressions) fail the build
fhirlint validate ./fhir/ --baseline fhirlint-baseline.json

# Regenerate when the codebase intentionally changes
fhirlint validate ./fhir/ --generate-baseline fhirlint-baseline.json
```

Issues recorded in the baseline are suppressed on subsequent runs. New issues that were not in the baseline still fail the build according to `--fail-on`. When the codebase is fixed and an issue no longer appears, fhirlint emits a warning (`warn: N baseline occurrence(s) no longer found`) — regenerate the baseline to remove the stale entries.

Commit `fhirlint-baseline.json` to version control so the suppressed issues are visible and reviewable by the team.

Baselines match findings by message ID. Under validator 7.0, many terminology issues arrive without an ID. They are answers from tx.fhir.de, which the validator's terminology routing uses for more code systems than in 6.10.x. fhirlint recovers it from the message text (marked `"messageIdInferred": true`) and warns when a finding misses the baseline only because its ID changed. See [Message IDs and validator 7.0](baseline.md#message-ids-and-validator-70).

You can also set the baseline file in `fhirlint.yml`:

```yaml
baseline: fhirlint-baseline.json
```

**Distinction from `--suppress`:** `--suppress` is for intentional, accepted deviations (permanent exceptions). Baseline mode is for managing technical debt — issues you plan to fix eventually but can't address right now.

See [Baseline mode guide](baseline.md) for a full CI workflow walkthrough.
