# Server mode (warm validator)

Every ordinary `fhirlint validate` invocation starts a fresh JVM and reloads the
HL7 validator's packages and terminology. That startup — tens of seconds — has
to happen before the first resource is validated. For repeated runs (an editor
loop, a large dataset, a CI matrix, a pre-commit hook) it dominates wall-clock
time.

`fhirlint serve` runs the validator **once** and keeps it warm as an HTTP
service (using the validator's own built-in server mode). Subsequent validations
take milliseconds.

```bash
# Terminal 1 — start the warm validator (packages load once)
fhirlint serve --port 8080 --ig hl7.fhir.us.core#9.0.0

# Terminal 2 / CI — validate against it, repeatedly and fast
fhirlint validate ./fhir/ --server http://localhost:8080
```

## Starting the server

```bash
fhirlint serve [--port 8080]
               [--fhir-version 4.0.1]
               [--ig <package|alias> ...]
               [--no-terminology-server | --terminology-server <url>]
               [--tx-cache <dir>]
```

- `--ig` accepts package references (`hl7.fhir.us.core#9.0.0`) or built-in
  aliases (`us-core`, `kbv-basis`, …). Load every IG whose profiles you intend
  to validate against — they are loaded once, at startup.
- The server streams package-loading progress while it starts, then prints the
  URL once it is ready to accept requests.
- It runs until interrupted with `Ctrl-C`, then shuts down cleanly.

## Validating against the server

Add `--server <url>` to any `validate` invocation (or set `server:` in
`fhirlint.yml`):

```bash
fhirlint validate patient.json --server http://localhost:8080
fhirlint validate ./bundle/     --server http://localhost:8080 --fail-on error
```

Everything downstream of validation works exactly as in a normal run — reporters
(`--format`), `--severity`, `--fail-on`, suppression, baseline, custom FHIRPath
rules, style/naming lint rules, and `--check-references`.

## What the server fixes vs. what varies per request

The server holds one validator configuration for its lifetime. Only the resource
and its profile vary per request.

| Fixed at `serve` startup | Varies per `validate --server` request |
|--------------------------|----------------------------------------|
| FHIR version (`--fhir-version`) | the resource being validated |
| IG packages (`--ig`) | the profile(s) to validate against (`--profile`, `profile-map`) |
| Terminology server / cache | — |

So **validation-semantic client flags are ignored** when using `--server`:
`--fhir-version`, `--ig`, `--best-practice`, `--jurisdiction`,
`--allow-example-urls`, `--display-issues-are-warnings`, `--locale`. fhirlint
prints a note if you pass `--fhir-version`/`--ig` together with `--server`. To
change any of these, restart `fhirlint serve` with the configuration you want.

Profiles **must** be available from an IG loaded at startup: to validate against
`us-core-patient`, start the server with `--ig us-core` (or the full package).

## CI pattern

Start the server as a background service, wait for it to be ready, then validate:

```bash
fhirlint serve --port 8080 --ig us-core --no-terminology-server &
# wait until the server answers, then:
fhirlint validate ./fhir/ --server http://localhost:8080 --fail-on error
```

For a single CI job that validates one small set, the plain `fhirlint validate`
(cold JVM) is simpler. Server mode pays off when the *same* warm validator serves
**many** validations.

## Limitations

- Transport is HTTP only (the validator's server mode is HTTP). A Unix-socket
  transport and an auto-managed daemon (spawn/reuse/stop transparently) are
  possible future additions.
- `--server` is not compatible with `--watch`.
- The server binds to `localhost`. Do not expose it to untrusted networks; it
  validates arbitrary submitted content.
