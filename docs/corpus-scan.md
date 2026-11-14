# Corpus false-positive scan

fhirlint's own checks, the [built-in lint rules](lint-rules.md) and the [reference check](references.md), are heuristics over resource structure. Their unit tests show that they do what their fixtures expect. They can't show that a rule stays quiet on resources nobody chose for it. That is what this scan is for (#577). It runs the rules over about 13,000 real-world resources written by other people, and compares two builds over the same data, so a change to a rule shows every finding it adds or removes.

The HL7 validator isn't involved. These checks run in-process on JSON, so the scan needs neither Java nor the JAR, and one scan of the corpus takes about a minute.

## When to run it

- For every pull request that adds or changes a built-in lint rule (`internal/lint`) or the reference check (`internal/refcheck`). Paste the diff summary into the pull request.
- Before each release, against the previous release.

## The corpus

[`scripts/corpus/sources.tsv`](../scripts/corpus/sources.tsv) pins every source to a version and a sha256. [`scripts/corpus-fetch.sh`](../scripts/corpus-fetch.sh) downloads them, rejects a download whose hash differs, and unpacks them to `~/.cache/fhirlint-corpus` (or `$FHIRLINT_CORPUS`). The corpus stays out of the repository. It is about 1.5 GB unpacked, and each source keeps its own licence.

| Dataset | Source | Resources |
|---|---|---|
| `hl7.fhir.r4.examples`, `r4b`, `r5` | the examples of the FHIR specification, R4, R4B and R5 | 5,306 / 2,840 / 2,822 |
| `hl7.fhir.us.core`, `uv.ips`, `uv.sdc`, `us.mcode` | HL7 implementation guides, conformance resources and examples | 435 / 119 / 202 / 351 |
| `de.basisprofil.r4`, `de.gematik.isik-basismodul`, `de.mii.laborbefund` | German IGs, as fhirlint's aliases use them | 218 / 126 / 20 |
| `synthea-r4-sep2019` | Synthea synthetic patients, one transaction Bundle each | 1,180 |

Each dataset is one set for the reference check, the way `fhirlint validate DIR --check-references` treats a directory. A dataset's `.rulescan-base` file holds the base for `canonical-url-pattern`, which is the canonical its own conformance resources use. Synthea has no canonical resources, so it has no base, and that rule is skipped there.

To add or bump a source: download it, record its sha256 in `sources.tsv`, and run the diff once on the old corpus and once on the new one, so that new findings come from the data and not from a rule.

## Running it

```bash
scripts/corpus-fetch.sh                 # once; later runs only verify
scripts/rule-diff.sh                    # latest release tag vs. the working tree
scripts/rule-diff.sh v1.17.0 my-branch  # any two revisions
RULE_DIFF_KEEP=/tmp/scan scripts/rule-diff.sh   # also keep both scans
```

The script builds both revisions and scans each from an empty directory. For each rule it prints the counts and the findings only A has (`-`) or only B has (`+`). Values that look like generated ids are masked, so the output can go into a pull request. Both builds use the same copy of the scan tool ([`internal/tools/rulescan`](../internal/tools/rulescan/main.go)): it is the measuring device, so only the rules differ between A and B.

To go through one rule's findings, scan and draw a sample:

```bash
go run ./internal/tools/rulescan scan -o scan.jsonl ~/.cache/fhirlint-corpus/data
go run ./internal/tools/rulescan sample -n 20 -rule ref:unresolved scan.jsonl
```

## Conventions

### What to read in the diff

A change meant to raise precision should only remove findings: read every one it adds. A false-positive fix should remove only the false positives: read the ones it removes. Look at a sample if there are many, and check that none of them is a true positive.

### The precision step

For a new or changed rule:

1. Draw about 20 findings from distinct resources (`rulescan sample`; one resource with many hits must not stand in for the corpus).
2. Judge each in context: open the resource and decide true or false positive.
3. Check the rule's premise against the primary source, the FHIR specification or the IG's own guidance, not only that the expression matched. "Matches as documented" is not the same as correct. The `url` of every resource looks like a canonical until the element definition says that `Device.url` is a network address.

Write the result into the pull request: how many of the sample were true positives, and which primary source the premise was checked against.

### Must stay reported

[`scripts/corpus/must-stay-reported.txt`](../scripts/corpus/must-stay-reported.txt) lists true positives in the corpus, each with a line saying why it is one. `rule-diff.sh` fails when build B no longer reports one of them, so a false-positive fix can't silently take true positives with it. When you fix a false positive, add the closest true positive next to it. For example, the fix for references between contained resources sits next to an entry for a reference that really has no target.

## Results

### First run (2026-11-14, v1.17.0)

The rules produced 8,703 findings. Four classes were false positives, all fixed in #577:

| Rule | Cause | Findings |
|---|---|---|
| `ref:unresolved` | A `#id` inside a contained resource points at a sibling in the container ([references](https://hl7.org/fhir/R4/references.html#contained)), but the check looked it up in the contained resource's own, empty, `contained` list | 173 |
| `ref:unresolved` | The entries of a Bundle inside a Bundle entry weren't indexed (the core examples' Bundle of Bundles) | 1,095 |
| `ref:unresolved` | The entries of a Bundle contained in another resource weren't indexed (SDC's extraction templates) | 4 |
| `ref:external` | Same cause as the Bundle of Bundles: a nested entry's absolute `fullUrl` wasn't indexed | 6 |
| `ref:external` | `DetectedIssue.reference` is a `uri` element of the resource, not the `reference` of a Reference datatype | 3 |
| `lint:canonical-url-pattern` | `Device.url` (R4, R4B, R5) and `DeviceDefinition.url` (R4, R4B) are the "network address to contact device", not a canonical. `Contract.url` was checked too, and its definition says canonical identifier, so it stays | 1 |

All remaining findings were justified as follows:

- **`ref:unresolved` (763):** each target is absent from the whole dataset, nested and contained resources included. A sample of 20 from distinct resources was all true positives: example resources point at a `Patient/example` or an `Organization/example` the package doesn't ship. That is exactly what the check reports, and the reason it is opt-in and meant for complete sets.
- **`ref:external` (155):** absolute references to servers outside the set, such as `http://www.BenefitsInc.com/fhir/...`. Information severity, as documented.
- **`lint:id-kebab-case` (3,855):** every hit has upper-case letters, dots or both, such as `Abrechnungsart`, OIDs or upper-case UUIDs. The rule is a house convention that HL7 doesn't share, and the FHIR `id` type allows all of these. The hits match the documented rule.
- **`lint:canonical-url-pattern` (1,756):** canonicals outside the dataset's base. Most are `terminology.hl7.org` code systems and value sets in the core examples, plus example canonicals on `example.org`. With the base chosen per dataset, these are what the rule exists to report.
- **`lint:profile-name-pascalcase` (892):** names that start lower-case (R4 core extensions, primitive types such as `boolean`) or contain spaces or hyphens. HL7's own invariant (`sdf-0` in R4, `cnl-0` in R5: `[A-Z]([A-Za-z0-9_]){0,254}`) warns about these as well. One difference is deliberate: the rule also rejects underscores, which HL7 allows. That accounts for 16 hits in de.basisprofil (`VitalSignDE_Blutdruck`) and MII (`MII_LM_Labor`), where underscores are an established naming style. The underscore question is open (see the pull request for #577).
