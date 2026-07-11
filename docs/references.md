# Referential integrity

Profile validation checks each resource on its own. It does not detect a
**dangling reference** — a `reference` that points at a resource which is not
present in the dataset. `--check-references` adds that cross-resource check.

```bash
fhirlint validate ./fhir/ --check-references
```

Or enable it in `fhirlint.yml`:

```yaml
check-references: true
```

## How it works

fhirlint builds an index of every resource's identity across the whole input
set — a directory, a Bundle, or an NDJSON file — and then resolves each literal
reference against it:

**Indexed as resolvable targets**

- `ResourceType/id` of every resource in the set.
- `fullUrl` of every Bundle entry (e.g. `urn:uuid:…` or an absolute URL).
- The `ResourceType/id` of each Bundle entry's `resource`.

**Reference resolution**

| Reference form | Resolves against | Unresolved severity |
|----------------|------------------|---------------------|
| Relative `Patient/123` (with optional `/_history/…`) | indexed `ResourceType/id` | error (`ref:unresolved`) |
| `urn:uuid:…` / `urn:oid:…` | Bundle entry `fullUrl` | error (`ref:unresolved`) |
| Contained `#id` | the `contained` array of the enclosing resource | error (`ref:unresolved`) |
| Absolute `https://host/fhir/Patient/1` | an indexed `fullUrl`, or the trailing `ResourceType/id` | information (`ref:external`) |

Absolute references to a server outside the validated set cannot be verified, so
they are reported at **information** severity rather than failing the build.

**Not reference-resolved**

- References by `identifier` only (no literal `.reference`) — logical references
  cannot be resolved by id.
- Canonical URLs (`StructureDefinition.baseDefinition`, `Questionnaire.item.definition`,
  …) — these are `url`-typed, not `Reference` datatypes.

## Findings

Findings carry the message ID `ref:unresolved` (error) or `ref:external`
(information) and a precise location, e.g.
`Encounter.participant[0].individual.reference`. They flow through every
reporter and respect `--severity`, `--fail-on`, suppression, and baseline:

```bash
# Silence external-reference notices
fhirlint validate ./fhir/ --check-references --suppress messageId:ref:external
```

## Opt-in semantics

The check is opt-in because it is only meaningful over a **complete** set. If you
validate a single file or a subset, references to resources you did not include
will correctly report as unresolved. Point `--check-references` at the full
dataset (directory / Bundle / NDJSON) that should be internally consistent.

XML resources are skipped with a notice — reference checking supports JSON input
only.

## Deferred

Orphan detection (resources in the set that nothing references) was considered
but left out of the initial version: at directory scale most resources are
legitimately unreferenced, so it would be noisy. It may be added later as an
opt-in, most usefully scoped to a single Bundle.
