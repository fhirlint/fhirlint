# Terminology

The terminology server, what the validator checks against it, and how to run without one.

## Terminology server

By default, fhirlint sends code lookups to `https://tx.fhir.org`. This behaviour can be tuned:

> **Note for CI and production use:** `tx.fhir.org` is a public development server — HL7 explicitly states it is not provisioned for CI pipelines or production use, has no SLA, and can go down without notice. For CI, either disable it with `--no-terminology-server` (terminology checks are skipped) or cache responses with `--tx-cache` to minimize requests. For production, run your own terminology server.

```bash
# Disable the terminology server entirely (offline / privacy-sensitive environments)
fhirlint validate patient.json --no-terminology-server

# Use a custom terminology server
fhirlint validate patient.json --terminology-server https://tx.internal.example.com

# Cache terminology responses between runs (useful in CI)
fhirlint validate patient.json --tx-cache .fhirlint-tx-cache/

# Disable terminology caching
fhirlint validate patient.json --tx-cache n/a

# Validation messages in German
fhirlint validate patient.json --locale de

# Suppress warnings about example.org placeholder URLs in test fixtures
fhirlint validate patient.json --allow-example-urls

# Control how best-practice constraints (e.g. dom-6) are handled
fhirlint validate patient.json --best-practice ignore    # silence all dom-6 warnings
fhirlint validate patient.json --best-practice error     # escalate to errors
```

**Caching the terminology responses in CI** speeds up repeated runs. Add the cache directory to `actions/cache`:

```yaml
- uses: actions/cache@v4
  with:
    path: .fhirlint-tx-cache
    key: fhirlint-tx-${{ runner.os }}-${{ hashFiles('fhirlint.yml') }}

- name: Validate FHIR resources
  run: fhirlint validate ./fhir/ --tx-cache .fhirlint-tx-cache/
```

> **`--tx-cache` does not make a run independent of the terminology server.** It is a latency optimisation, not a reproducibility measure. The validator fetches the server's capability statement at startup, before validating anything, and aborts when that fails — warm cache or not. With a fully populated cache and an unreachable server you get no report at all:
>
> ```text
> FHIRException: Error fetching the server's capability statement: Failed to connect to ...
> ```
>
> For runs that must not depend on the server, see [offline terminology](#offline-terminology) below.

### When the validator stops checking codes

Checking a code costs a terminology round trip, so the validator caps how many it will check for a single ValueSet include, ConceptMap group or CodeSystem supplement. Past the cap it checks **none** of them and issues a hint:

```text
Checks skipped: 2 — too many codes to check against a code system. See --codesystem-size-limit.
```

The cap is 1000 by default. A German project validating against ICD-10-GM, OPS or ATC is over it immediately, so a green run there is not evidence that the codes are valid — it can mean nobody looked. fhirlint counts these hints separately in the summary for that reason: they arrive at hint severity, which most projects filter out, and the message saying a check did not run would otherwise be the first thing hidden.

Raising the cap will not help if the code system is not available in the first place, which for [German code systems is the default](#german-code-systems-are-not-checked-by-default). That case is counted and reported separately, because the remedy is different.

```bash
# Check them all, however many there are
fhirlint validate ./ig/ --codesystem-size-limit 0

# Or raise the cap
fhirlint validate ./ig/ --codesystem-size-limit 5000
```

Expect a remote terminology server to be slow about it — one round trip per code. Recording once with [`fhirlint tx warm`](#offline-terminology) and replaying makes repeat runs cheap.

`fhirlint explain VALUESET_INC_TOO_MANY_CODES` (also `CONCEPTMAP_VS_TOO_MANY_CODES`, `CODESYSTEM_CS_SUPP_TOO_MANY_CODES`) spells this out offline. The CodeSystem-supplement form is new in validator 6.10.2: a supplement that used to be checked can stop being checked after upgrading.

### German code systems are not checked by default

`de.basisprofil.r4` declares ICD-10-GM, OPS, ATC and Alpha-ID with `content=not-present`: it defines the code systems but ships none of their concepts. The content is published only as `bfarm.terminologien.*` packages on the [BfArM's own registry](https://terminologien.bfarm.de/), behind a token. It is not on `packages.fhir.org`, and neither `tx.fhir.org` nor `tx.fhir.de` serves those code systems, so there is no server to point at that has them.

A run therefore looks like this:

```text
Files: 1  Valid: 1  Errors: 0  Warnings: 0
Codes unchecked: 1 — a code system was not available. See: fhirlint explain UNKNOWN_CODESYSTEM
```

The finding arrives as a warning and the resource still counts as valid, so filtering to errors would hide it entirely. fhirlint counts it separately for that reason: **a green run is not evidence that your ICD-10-GM codes are valid** unless you arranged the content.

Two ways to arrange it. Install the terminology package into `~/.fhir/packages` yourself and name it like any other IG:

```bash
fhirlint validate ./fhir/ --ig bfarm.terminologien.icd10gm#<version>
```

Or run a terminology server loaded with the content. A local one over plain HTTP needs [`--allow-insecure-tx`](#using-your-own-terminology-server-over-http):

```bash
fhirlint validate ./fhir/ --terminology-server http://localhost:8080/fhir --allow-insecure-tx
```

`fhirlint explain UNKNOWN_CODESYSTEM` says the same thing at the terminal, and every wording the validator uses for it resolves to that explanation.

### Validating an IG's own examples (`--package-baseline`)

Point fhirlint at a package's examples and the error count is not interpretable on its own. Many IGs deliberately ship **invalid** instances to demonstrate what their invariants catch. `de.fhir.medication#2.0.0-ballot` has 563 examples and 327 of them fail — which reads as a broken package and is not.

Nothing in the resources says which is which. The `ImplementationGuide` lists all 563 with an empty `description`, and `exampleBoolean` only states *that* something is an example. Filenames do not carry it either: 368 of those examples are named `INV-…`, `Invalid-…` or `Warning-…`, but only 327 fail, because the 41 `Warning-…` cases legitimately produce warnings only.

The publisher's own QA run does carry it. Recent IG Publisher versions write `package/other/validation-summary.json` into the tarball, keyed by `ResourceType/id`:

```json
"MedicationDispense/Example-MD-Cov-bedarf-doseRange": { "errors": 0, "warnings": 4 }
```

fhirlint reads it automatically for any file validated straight out of the FHIR package cache, and reports the comparison:

```text
Files: 563  Valid: 236  Errors: 327  Warnings: 1108
Package baseline: 563 resource(s) checked against de.fhir.medication#2.0.0-ballot
  agrees with the published build (327 expected failure(s))
```

`--package-baseline` turns that into an exit code: resources the published build already recorded errors for stop failing the run, and anything beyond the baseline still does.

```bash
fhirlint validate ~/.fhir/packages/de.fhir.medication#2.0.0-ballot/package/example/ \
  --ig de.fhir.medication#2.0.0-ballot --package-baseline
```

Three things worth knowing:

- **It is off by default.** Not failing on errors because a third-party artefact said they were fine is a decision to make deliberately.
- **A large delta is usually about your environment, not the package.** Errors this run finds and the published build did not are often a missing `hl7.fhir.tools` dependency, a different terminology server, or absent CQL libraries — `Type_Specific_Checks_DT_URL_Resolve`, `UNKNOWN_CODESYSTEM_VERSION` and `MEASURE_M_CRITERIA_CQL_NO_LIB` dominate. The reverse case (`the build found and this run did not`) is reported too, and means this run checked *less*, since the package itself cannot have changed.
- **Not every package has one.** 113 of 198 packages in a representative cache ship the file, essentially newer IG Publisher output. `de.basisprofil.r4`, `de.gematik.isik#6.0.0` and `hl7.fhir.uv.ips#2.0.1` do not. Without a baseline fhirlint says nothing and reports exactly as before.

### Pinning code system versions (`--expansion-parameters`)

ICD-10-GM, OPS and Alpha-ID are annual editions. The German profiles bind to them without naming a version, so expansion follows whatever edition the terminology server currently holds. The same resources can therefore validate differently either side of a year change, with nothing in your repository having moved.

`--expansion-parameters` takes a FHIR `Parameters` resource and makes the edition an explicit, reviewable decision:

```json
{
  "resourceType": "Parameters",
  "parameter": [
    { "name": "system-version",
      "valueUri": "http://fhir.de/CodeSystem/bfarm/icd-10-gm|2026" },
    { "name": "system-version",
      "valueUri": "http://fhir.de/CodeSystem/bfarm/ops|2026" }
  ]
}
```

```bash
fhirlint validate ./fhir/ --expansion-parameters expansion-params.json
```

```yaml
# fhirlint.yml
expansion-parameters: expansion-params.json
```

Two things to know about it:

- **The file replaces the validator's default expansion parameters, it does not extend them.** Everything the run needs has to be in it; nothing is inherited.
- **It changes which terminology requests go out.** With [`--tx-offline`](#offline-terminology), record with the same file you validate with — `fhirlint tx warm` takes `--expansion-parameters` too. fhirlint fingerprints the file into the recording's manifest and warns when a replay was recorded with different parameters, which is usually the explanation for requests that suddenly come up missing.

### Air-gapped runs (`--offline`)

`--tx-offline` covers terminology. `--offline` covers the rest of the run:

```bash
fhirlint validate ./fhir/ --offline
```

- the validator JAR is taken from the cache and never downloaded
- the update check is skipped
- IG packages must already be in `~/.fhir/packages`, and a missing one is named before the JAR starts rather than surfacing as a package-resolution error
- terminology is skipped, unless a recording is replayed with `--tx-offline`
- the validator is told to block its own HTTP with `-no-http-access`

The last point is what makes it a guarantee rather than a promise about fhirlint's own code paths — the JAR refuses every request itself, including any URL that content asks it to follow.

**One combination gets a weaker guarantee, and says so.** `--tx-offline` replays terminology from a local server on loopback, and the validator's block has no loopback exemption — it refuses `http://127.0.0.1` exactly as it refuses `https://tx.fhir.org`. So `--offline --tx-offline` cannot use the block:

```text
--offline: terminology is replayed from a local server, so the validator's own
network block cannot be used. fhirlint downloads nothing; the JAR could still
follow a URL if content asks it to. For a hard block, drop --tx-offline or
isolate the network.
```

`--offline` needs validator 6.10.0 or newer (where `-no-http-access` was added) and refuses to run against an older JAR rather than failing obscurely. `--offline` and `--terminology-server` are mutually exclusive.

The same flag is on [`fhirlint serve`](usage.md#server-mode-warm-validator) and [`fhirlint lsp`](usage.md#editor-integration-language-server), where the decision is made once for the server's lifetime:

```bash
fhirlint serve --offline --ig kbv.basis#1.9.0
fhirlint lsp --offline
```

`lsp --offline --server <url>` is the one case where it does nothing: that server belongs to another process, started with whatever policy its owner chose, and the run says so instead of implying otherwise.

### Using your own terminology server over HTTP

A local Blaze, HAPI, Ontoserver or Snowstorm usually answers on plain HTTP:

```bash
fhirlint validate ./fhir/ --terminology-server http://localhost:8080/fhir --allow-insecure-tx
```

The flag is needed because validator 6.10.0 and later refuse plain-HTTP and private-network destinations outright, as protection against server-side request forgery. Without it the run stops before reading a resource:

```text
The validator refused to contact http://localhost:8080/fhir.
  Refusing to fetch from non-https URL: http://localhost:8080/fhir/metadata
This is not a connection failure. Validator 6.10.0 and later block plain HTTP
and private-network destinations, so a proxy or a different URL will not help.
```

`--allow-insecure-tx` exempts **exactly that URL** by generating a `fhir-settings.json` for the run. Every other request the validator makes keeps the protection. fhirlint never passes `-ssrf-protection-enabled false`, which would lift it for the whole run.

The exemption is announced on stderr, because a run that weakens a security control should not look like one that did not:

```text
Exempting http://localhost:8080/fhir from the validator's SSRF protection (--allow-insecure-tx).
```

Credentials are a separate matter: fhirlint refuses to send terminology credentials over plain HTTP at all, whatever this flag says.

### Authenticating to a terminology server

A terminology server that is not `tx.fhir.org` usually wants a credential. fhirlint reads it from the environment — one variable per scheme, and the one that is set decides the scheme:

| Variable | Sent as |
|---|---|
| `FHIRLINT_TX_TOKEN` | `Authorization: Bearer <token>` |
| `FHIRLINT_TX_APIKEY` | `Api-Key: <key>` |
| `FHIRLINT_TX_AUTH` | `Authorization: Basic <base64(user:password)>`, value is `username:password` |

```bash
export FHIRLINT_TX_TOKEN="$ONTOSERVER_TOKEN"
fhirlint validate ./fhir/ --terminology-server https://ontoserver.example.org/fhir
```

```text
Authenticating to https://ontoserver.example.org/fhir (bearer token from FHIRLINT_TX_TOKEN)
```

Environment only, deliberately: a flag lands in shell history and CI logs, and a config value lands in the repository. This is the same reasoning as `FHIRLINT_PROXY_AUTH`, which is proxy authentication and unrelated to terminology.

Rules worth knowing:

- **Only an explicitly named server is authenticated.** Without `--terminology-server` the validator talks to the public `tx.fhir.org`, and a credential that happens to be in the environment is not sent there.
- **Never over plain HTTP.** `http://` plus a credential is refused rather than leaked.
- **Setting two variables is an error.** Two credentials means one is stale, and picking a winner silently is how the wrong one gets used for months.
- `fhirlint serve` authenticates the same way, once for the server's lifetime.
- `fhirlint tx warm --terminology-server …` authenticates while recording. The credential is not stored in the recording, so replaying it in CI needs no secret at all — which is the point.

### Offline terminology

`--tx-offline` replays terminology responses recorded earlier, so a validation run needs no terminology server at all. This is what makes a run reproducible: same inputs, same recording, same result, no network.

Record once against the real server:

```bash
fhirlint tx warm ./fhir/
# Recording terminology traffic from https://tx.fhir.org into .fhirlint-tx/ (42 file(s))…
# Recorded 137 terminology interaction(s) (137 new) in .fhirlint-tx/
```

Then replay, offline, as often as you like:

```bash
fhirlint validate ./fhir/ --tx-offline
# Replaying 137 recorded terminology interaction(s) from .fhirlint-tx/
```

Under the hood fhirlint runs a loopback HTTP server that stands in for the terminology server and points the validator at it. Recording proxies to the real server and stores each request and response; replay serves them from disk. Recordings are stored as one pretty-printed JSON file per interaction, so a committed recording stays reviewable in a diff.

**A request that was not recorded is an error, never a silent fallback to the network:**

```text
Error: 2 terminology request(s) were not in the recording in .fhirlint-tx/:
  POST /r4/ValueSet/$validate-code (system http://loinc.org, code 8302-2)
  POST /r4/CodeSystem/$validate-code (system http://snomed.info/sct, code 271649006)
Re-record with: fhirlint tx warm ./fhir/
```

This matters more than it looks: the validator downgrades some terminology failures to warnings and carries on, so an incomplete recording could otherwise produce a green run that quietly skipped real terminology checks. fhirlint tracks the misses itself and fails the run regardless of how the validator reacted.

Record with the same profiles and IGs you validate with, since those determine which codes get checked:

```bash
fhirlint tx warm ./fhir/ --profile kbv-patient --ig kbv.basis#1.5.0
fhirlint validate ./fhir/ --profile kbv-patient --ig kbv.basis#1.5.0 --tx-offline
```

**A recording is a single-server run.** Every request goes to the one server it was recorded from. The validator, run live without `--terminology-server`, sends some requests elsewhere: the tx registry names other servers for some code systems, such as tx.fhir.de for the German SNOMED CT edition, which covers most KBV, MII and ISiK examples. Their answers can differ from tx.fhir.org's, so a replay reproduces the recording, not that live run. `tx warm` warns about this (the validator has routed since 6.2.8), and the recording's `manifest.json` says `"routing": "single-server"`. Compare replayed runs with replayed runs, and live runs with live runs.

Use `--tx-dir` to put the recording somewhere other than `.fhirlint-tx/`. In CI, either commit the recording or rebuild it as an artifact:

```yaml
- name: Validate FHIR resources
  run: fhirlint validate ./fhir/ --tx-offline
```

See the [Offline terminology guide](terminology-offline.md) for the recording format, CI patterns and server mode.

`--tx-offline` cannot be combined with `--no-terminology-server` (which skips terminology instead of replaying it), `--terminology-server` (which it replaces), or `--server` (whose terminology is fixed when the validator server starts).

### Behind a proxy

Two different programs make network calls here, and they read their configuration differently:

| Call | Made by | Reads |
|---|---|---|
| Validator JAR download, `--url` inputs, update check | fhirlint | `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY` |
| Terminology server (`tx.fhir.org`) | the validator JAR | nothing, unless told |

That asymmetry produces a confusing failure on a proxied network: the JAR downloads fine, then validation stalls against the terminology server. `--proxy` and `--https-proxy` close it:

```bash
fhirlint validate ./fhir/ --https-proxy proxy.example.org:3128
```

If unset, both default to the standard environment variables, so exporting them the way every other tool expects is usually enough:

```bash
export HTTPS_PROXY=http://proxy.example.org:3128
fhirlint validate ./fhir/
```

The JAR wants a bare `host:port` while the environment variables are URLs, so the scheme is stripped for you. Credentials embedded in the URL (`http://user:pass@proxy:3128`) are split out and passed separately, as the validator requires.

`NO_PROXY` applies to fhirlint's own requests but **not** to the validator's — it has no equivalent option. On a network where the terminology server is reachable directly but everything else goes through a proxy, set `--tx` to a local terminology server or `--no-tx` instead.

#### Proxy credentials

For a proxy requiring basic auth, use the environment:

```bash
export FHIRLINT_PROXY_AUTH='username:password'
```

There is deliberately no `--proxy-auth` flag and no `fhirlint.yml` key: the first would put the credential in shell history and CI job logs, the second in a file meant to be committed.

Be aware of what this does not fix. The validator takes the credential as a command-line argument, so it is visible in `ps` to other users on the same host for the duration of the run. fhirlint cannot change that. Where you have the choice, a proxy that does not require basic auth avoids the problem entirely.
