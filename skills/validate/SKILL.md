---
name: validate
description: Validate FHIR resources (JSON/XML) with the fhirlint CLI, then explain the findings and fix the offending resources in place. Use when the user asks to validate, lint, or check FHIR resources/bundles/profiles, when working in a repo of FHIR resources, or when a FHIR file has validation errors to diagnose. Supports German profiles (KBV, MII, ISiK, DiGA).
allowed-tools: Bash, Read, Edit, Write, Glob, Grep
---

# Validate FHIR resources with fhirlint

This skill runs the `fhirlint` CLI to validate FHIR resources, then has you
**interpret** the validator's findings and **fix** the offending resource —
the value fhirlint adds over the raw HL7 validator is a clean, machine-readable
report you can act on directly.

## Prerequisites — check first

1. **`fhirlint` must be on `PATH`.** Verify with `fhirlint version`. If it is
   not installed, tell the user to install it and stop:
   - Homebrew: `brew install fhirlint/tap/fhirlint`
   - Go: `go install github.com/fhirlint/fhirlint@latest`
   - or download a binary from https://github.com/fhirlint/fhirlint/releases
2. **First run downloads the validator JAR (~250 MB) and needs Java 17+.** Tell
   the user the first invocation may take a minute and is **not** a hang. If
   `fhirlint` reports Java is missing, point them to https://adoptium.net.

## How to run

Always request JSON output and parse it — it is the contract, not the terminal text:

```bash
fhirlint validate <path> --format json
```

- `<path>` is a file, a directory, or `.` for the whole project. With no path,
  fhirlint validates the current directory.
- Add `--profile <alias-or-url>` to validate against a profile. Built-in
  aliases include `kbv-basis`, `kbv-patient`, `mii`, `diga`; see
  `fhirlint profiles` for the full list.
- Add `--ig <pkg#version>` to load an implementation guide.
- Use `fhirlint explain <message-id>` (e.g. `fhirlint explain dom-6`) to get a
  plain-language explanation of any message ID — fully offline, no JAR needed.

The JSON report has a `files` array; each file has `valid` plus an `issues`
array of `{severity, message, location, messageId}`. **Exit code 1 means
validation found errors; that is expected — do not treat it as a tool failure.**

## What to do with the results

1. Run the validation and read the JSON report.
2. For each `error`/`fatal` issue, explain in plain language what the validator
   is complaining about (use `fhirlint explain <messageId>` when a message ID is
   present and you are unsure).
3. Locate the offending field via the issue's `location` (a FHIRPath
   expression), open the resource, and propose or apply a fix that resolves the
   issue without changing unrelated data.
4. Re-run `fhirlint validate` on the fixed file to confirm the issue is gone.
5. Summarise: what was wrong, what you changed, and what (if anything) remains
   (e.g. warnings the user may choose to accept or suppress with `--suppress`).

Treat `warning`/`information` issues as advisory — surface them, but only change
the resource for them if the user asks.
