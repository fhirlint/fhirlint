---
name: audit
description: Check the bundled HL7 FHIR validator JAR for updates and known security advisories using `fhirlint audit`. Use when the user asks whether their FHIR validator is up to date or secure, wants to audit the validator toolchain, or is reviewing supply-chain/security posture of their FHIR setup.
allowed-tools: Bash
---

# Audit the FHIR validator JAR

This skill runs `fhirlint audit` to check whether the bundled HL7 FHIR
Validator JAR is up to date and whether any published security advisories
affect the installed version.

## Prerequisites

`fhirlint` must be on `PATH` (verify with `fhirlint version`). See the
`validate` skill for install instructions. `audit` needs network access to
reach the GitHub releases and advisory APIs.

## How to run

Use JSON output and parse it:

```bash
fhirlint audit --format json
```

The report contains `currentVersion`, `latestVersion`, `outdated` (bool),
`advisoryCount`, and an `affecting` array of advisories that apply to the
installed version (each with `ghsaId`, `severity`, `summary`, `url`).

Note: `audit --format json` always exits `0` so the result can be parsed; rely
on the JSON fields, not the exit code, to decide status.

## What to do with the results

1. Report whether the JAR is current. If `outdated` is true, tell the user to
   run `fhirlint update`.
2. If `affecting` is non-empty, list each advisory (severity, summary, link) and
   recommend updating. Be clear about which advisories actually affect the
   installed version versus the total published count.
3. If everything is current with no affecting advisories, say so plainly.
