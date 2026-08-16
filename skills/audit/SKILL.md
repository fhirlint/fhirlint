---
name: audit
description: Check the bundled HL7 FHIR validator JAR and the IG packages pinned in fhirlint.lock for updates and known security advisories using `fhirlint audit`. Use when the user asks whether their FHIR validator or implementation guides are up to date or secure, wants to audit the validator toolchain, or is reviewing supply-chain/security posture of their FHIR setup.
allowed-tools: Bash
---

# Audit the FHIR toolchain

This skill runs `fhirlint audit`, which checks the two inputs that decide what a
validation run reports:

- the HL7 FHIR Validator JAR — whether it is up to date, and whether any
  published security advisory affects the installed version
- the IG packages recorded in `fhirlint.lock` — whether newer versions exist in
  the FHIR package registry, and whether any pin no longer resolves

## Prerequisites

`fhirlint` must be on `PATH` (verify with `fhirlint version`). See the
`validate` skill for install instructions. `audit` needs network access to reach
the GitHub releases and advisory APIs and `packages.fhir.org`.

Run it from the project root: the IG half reads `fhirlint.lock` from the current
directory. Without a lock file that section is skipped and the JAR check still
runs.

## How to run

Use JSON output and parse it:

```bash
fhirlint audit --format json
```

JAR fields: `currentVersion`, `latestVersion`, `outdated` (bool),
`advisoryCount`, and an `affecting` array of advisories that apply to the
installed version (each with `ghsaId`, `severity`, `summary`, `url`).

IG fields: `lockFile` (omitted when there is none) and `igPackages`, an array
with one entry per pinned package. Each entry has `id`, `name`, `version`, and
`latest`, plus at most one status flag:

- `outdated` — a newer version was established to exist
- `notFound` — the registry has no such package
- `deprecated` (with an optional `deprecationNote`)
- `differs` — the pinned version is not `latest` but the two could not be
  ordered, because FHIR IG versioning is not reliably semver
- `ahead` — the pin is newer than the registry's `latest`, e.g. a pre-release
- `error` — this package could not be checked at all

Note: `audit --format json` always exits `0` so the result can be parsed; rely
on the JSON fields, not the exit code, to decide status. (In terminal format the
codes are `1` for a JAR problem or affecting advisory and `2` for IG findings
only.)

## What to do with the results

1. Report whether the JAR is current. If `outdated` is true, tell the user to
   run `fhirlint update`.
2. If `affecting` is non-empty, list each advisory (severity, summary, link) and
   recommend updating. Be clear about which advisories actually affect the
   installed version versus the total published count.
3. For `igPackages`, lead with `notFound` and `deprecated` entries — those break
   or will break — then list `outdated` ones with their available version.
4. Report `differs` as "these versions differ" and do **not** describe it as
   outdated. The tool declined to guess a direction; do not supply one.
5. Treat `error` entries as "not checked", never as "fine". Say which packages
   could not be reached.
6. If everything is current with no affecting advisories, say so plainly.
