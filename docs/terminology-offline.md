# Offline terminology

`--tx-offline` replays terminology responses recorded earlier, so a validation run needs no terminology server. This makes a run reproducible — same inputs, same recording, same result — and independent of `tx.fhir.org`, which HL7 states has no SLA and is not provisioned for CI.

## Why `--tx-cache` is not enough

The validator JAR has a terminology cache of its own (`--tx-cache`, `-txCache`). It is a latency optimisation and nothing more:

- The JAR fetches the terminology server's **capability statement at startup**, before validating anything, and aborts when that fails. With a fully populated cache and an unreachable server you get no report at all, only `Error fetching the server's capability statement`.
- With a warm cache, requests still go out during the run.
- There is no fail-on-miss mode: the JAR either uses a cache directory or does not cache.

So making a run hermetic means standing in for the terminology server, not caching behind it. That is what `fhirlint tx warm` and `--tx-offline` do.

## Validator version requirements

Validator **6.10.0** added SSRF protection that refuses plain-HTTP and private-network destinations. The replay server is both — loopback, over HTTP — so fhirlint writes a `fhir-settings.json` per run that exempts exactly that one URL, and passes it with `-fhir-settings`. Nothing else is exempted, and SSRF protection stays on for every other request the run makes.

The exemption names the loopback URL including its port, because the validator's prefix matching was tightened when CVE-2026-34361 was fixed. That is why the file is generated per run rather than shipped.

This is handled automatically and works on 6.9.x as well. If you see `Refusing to fetch from non-https URL`, you are on a fhirlint older than the fix.

`--tx-offline` therefore cannot be combined with your own `-fhir-settings` via `--validator-arg`; the validator takes only one, and fhirlint says so rather than silently overriding yours.

## Recording

```bash
fhirlint tx warm ./fhir/
```

fhirlint starts a loopback HTTP server that proxies to the real terminology server, points the validator at it, and stores every request and response under `.fhirlint-tx/`.

Record with the same profiles and IGs you validate with — they determine which codes get checked:

```bash
fhirlint tx warm ./fhir/ --profile kbv-patient --ig kbv.basis#1.5.0
```

Record against a specific server with `--terminology-server`. Without it, fhirlint records from the endpoint the validator would have used itself (`https://tx.fhir.org/r4` for FHIR R4 and R4B, `/r5` for R5).

### Terminology routing

Run without `-tx`, the validator (6.2.8 and later) looks up each code system in the tx registry and sends some requests to the server the registry names for it, not to tx.fhir.org. tx.fhir.de, for example, is authoritative for the German SNOMED CT edition and a candidate for ATC. A recording has to capture those requests too, or its replay is not the live run.

**With validator 7.0.4 or later**, `tx warm` without `--terminology-server` records with the routing intact (#465). The validator gets the recorder as `-tx` and the recorder's `/tx-reg` as `-ecosystem`, the registry option 7.0.4 added. The recorder forwards registry requests to `https://ecosystem.hl7.org/tx-reg` and rewrites the servers in each answer, other than tx.fhir.org, to a route through itself. The validator itself substitutes `-tx` for tx.fhir.org. A route is forwarded only to a server a registry answer named in the same run, so the recorder is not an open proxy, and the terminology server's credentials are sent neither to the registry nor to routed servers. The registry's answers are recorded too, so a replay also pins the routing decisions, which change over time independently of your resources. `manifest.json` then says:

```json
"routing": "registry",
"ecosystem": "https://ecosystem.hl7.org/tx-reg",
"routed": ["tx.fhir.de"],
```

`routed` lists the servers requests were actually routed to. Measured with validator 7.0.5 on the `Medication` example `Medication-149e683b-…` from `kbv.basis#1.9.0`:

| run | issues | same as live? |
|---|---|---|
| live, no `--terminology-server` | 16 | — |
| replay of a recording with registry routing | 16 | yes: ids, locations and messages |
| replay of a single-server recording (`--terminology-server https://tx.fhir.org/r4`) | 10 | no |

Replaying a registry recording needs 7.0.4 or later as well, and `--tx-offline` and `fhirlint serve --tx-offline` refuse an older JAR.

**Before 7.0.4**, and whenever `--terminology-server` is given, the recording is single-server: every request goes to the recorder's upstream. A replay then reproduces a single-server run, not a live run without `--terminology-server`. `tx warm` warns, and `manifest.json` records `"routing": "single-server"`, so that `upstream` is not read as "the server the live run used". A live run with `--terminology-server <url>` is single-server too, so it matches a recording made from the same `<url>`. Do not mix the two across environments: a team that replays a single-server recording in CI and validates live locally can get different terminology findings on the same commit.

Measured with validator 7.0.1 on the same `Medication` before this existed: live, `Medication.code` and `Medication.form` were checked by tx.fhir.de and produced 12 terminology warnings without message ids. Replayed from a tx.fhir.org recording, the same file produced 6 issues with ids (`UNKNOWN_CODESYSTEM_VERSION` and others). Validator 6.10.3 routes too, but on this file it reports the same issues either way (see [Message IDs and validator 7.0](baseline.md#message-ids-and-validator-70)).

The JAR's own terminology cache is **disabled during recording**. Otherwise it would answer some requests from `~/.fhir` without them ever reaching the recorder, and the recording would be complete only on the machine that made it.

## Replaying

```bash
fhirlint validate ./fhir/ --tx-offline
```

The recording is served from disk; nothing leaves the machine. The JAR's own cache is disabled here too, so every request reaches the replay server — otherwise an incomplete recording would pass locally and fail on a clean CI runner.

## Misses are errors

A request that was not recorded fails the run:

```text
Error: 2 terminology request(s) were not in the recording in .fhirlint-tx/:
  POST /ValueSet/$validate-code (system http://loinc.org, code 8302-2)
  POST /CodeSystem/$validate-code (system http://snomed.info/sct, code 271649006)
Re-record with: fhirlint tx warm ./fhir/
```

This is deliberate and it is the point of the feature. The validator downgrades some terminology failures to warnings and carries on, so a silently incomplete recording could otherwise produce a green run that skipped real terminology checks. fhirlint tracks misses itself and fails the run regardless of how the validator reacted.

Long lists are truncated to the first ten with a count of the rest.

## What a recording looks like

One pretty-printed JSON file per interaction, plus a `manifest.json` recording where and when it came from:

```text
.fhirlint-tx/
  manifest.json
  0bee30eecb57e4406ff51283e623b1ee.json
  1a4f…json
```

`manifest.json` also records the **validator version** the recording was made with. Which terminology requests get made is a property of the validator, not only of the resources — 6.10.0 changed how code systems are resolved — so replaying against a different version warns:

```text
warn: recording was made with validator 6.10.1, this run uses 6.9.12 — re-record if requests come up missing
```

It is a warning, not an error: a version change does not necessarily invalidate a recording. It turns an otherwise inexplicable miss into one with an obvious first suspect.

Each interaction file holds the request (method, path, query, canonicalised body) and the response (status, content type, body). Files are named by a hash of the request, so the same logical request maps to the same file across runs regardless of JSON key ordering.

Recordings are meant to be reviewed: pretty-printed JSON diffs cleanly, so a change in what a terminology server answers is visible in a pull request.

The recording directory is excluded from validation automatically — without that, its JSON files would be picked up and validated as if they were FHIR resources.

## In CI

Either commit the recording, or rebuild it as an artifact. Committed is the stronger option: it pins terminology the way `fhirlint.lock` pins IG packages and `--validator-version` pins the JAR.

```yaml
- name: Validate FHIR resources
  run: fhirlint validate ./fhir/ --tx-offline
```

Refresh it deliberately, as its own pull request:

```bash
fhirlint tx warm ./fhir/
git add .fhirlint-tx && git commit -m "chore: refresh terminology recording"
```

## Server mode

`fhirlint serve --tx-offline` works the same way and keeps the replay server alive for the validator server's lifetime.

One difference: a long-lived server cannot fail a run on a miss, since the request has already been answered by then. Misses are reported at shutdown instead:

```text
warn: 3 terminology request(s) were not in the recording:
  POST /ValueSet/$validate-code (system http://loinc.org, code 8302-2)
Re-record with: fhirlint tx warm <path>
```

## Incompatible options

| Option | Why |
|--------|-----|
| `--no-terminology-server` | Skips terminology instead of replaying it |
| `--terminology-server` | `--tx-offline` replaces the server; record against yours instead |
| `--server` (client side) | The validator server's terminology is fixed when it starts |
