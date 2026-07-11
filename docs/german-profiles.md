# German FHIR profiles (KBV, MII, DiGA)

fhirlint ships with built-in aliases for the most common German FHIR profiles and implementation guides. This guide explains what each profile covers, how to use the aliases, and how to combine them for real-world German FHIR projects.

## Table of contents

- [Built-in aliases](#built-in-aliases)
- [KBV — Kassenärztliche Bundesvereinigung](#kbv--kassenärztliche-bundesvereinigung)
- [MII — Medizininformatik-Initiative](#mii--medizininformatik-initiative)
- [DiGA — Digitale Gesundheitsanwendungen](#diga--digitale-gesundheitsanwendungen)
- [Combining profiles](#combining-profiles)
- [Version pinning](#version-pinning)
- [Using a specific profile URL](#using-a-specific-profile-url)
- [fhirlint.yml for German projects](#fhirlintymll-for-german-projects)

---

## Built-in aliases

```bash
# List all available aliases
fhirlint profiles
```

| Alias | IG package | Covers |
|-------|-----------|--------|
| `kbv-basis` | `kbv.basis#1.5.0` | KBV base profiles (Patient, Practitioner, Organization, …) |
| `kbv-patient` | `kbv.basis#1.5.0` | KBV Patient profile specifically |
| `mii` | `de.medizininformatikinitiative.kerndatensatz#2024.0.0` | MII core dataset modules |
| `diga` | `de.bfarm.diga#1.2.0` | DiGA framework profiles |

Aliases resolve to the full IG package reference at runtime. They are a convenience shortcut — you can always use the full package reference directly if you need a specific version.

fhirlint also ships international aliases (`us-core`, `ips`, `ipa`, `uk-core`) — run `fhirlint profiles` or see the [Built-in profile aliases](../README.md#built-in-profile-aliases) table for the full list.

---

## KBV — Kassenärztliche Bundesvereinigung

The KBV publishes FHIR profiles for statutory health insurance workflows in Germany — prescriptions (eRezept), referrals, and medical documents.

### Validate against KBV base profiles

```bash
# Using the alias
fhirlint validate patient.json --profile kbv-basis

# Equivalent with full package reference
fhirlint validate patient.json --ig kbv.basis#1.5.0
```

### eRezept (e-prescription)

The KBV eRezept profiles are in a separate package:

```bash
fhirlint validate prescription-bundle.json --ig kbv.ita.erp#1.3.0
```

### Validate a KBV Patient resource

```bash
fhirlint validate patient.json \
  --profile kbv-patient \
  --fhir-version 4.0.1
```

### Common KBV issues

**`dom-6` narrative warnings** — KBV resources are used in machine-to-machine interfaces and typically omit the `text` narrative element. Suppress this project-wide:

```yaml
# fhirlint.yml
suppress:
  - constraint: dom-6
```

**Terminology warnings for KBV code systems** — KBV uses German-specific code systems (e.g. `https://fhir.kbv.de/CodeSystem/KBV_CS_SFHIR_BAR2_WBO`) that may not be fully resolvable via `tx.fhir.org`. If you see repeated `Unknown code system` warnings, use `--no-terminology-server` during development or add the KBV terminology server:

```bash
fhirlint validate patient.json \
  --profile kbv-basis \
  --terminology-server https://fhir.kbv.de/terminology
```

---

## MII — Medizininformatik-Initiative

The MII defines a core dataset (`Kerndatensatz`) for clinical research data interoperability across German university hospitals.

### Validate against MII core dataset

```bash
fhirlint validate observation.json --profile mii

# Equivalent
fhirlint validate observation.json \
  --ig de.medizininformatikinitiative.kerndatensatz#2024.0.0
```

### Validate a specific MII module

The MII core dataset is split into modules. You can load individual module packages instead of the full core dataset:

```bash
# MII Laboratory module
fhirlint validate lab-observation.json \
  --ig de.medizininformatikinitiative.kerndatensatz.laborbefund#2024.0.0

# MII Diagnose module
fhirlint validate condition.json \
  --ig de.medizininformatikinitiative.kerndatensatz.diagnose#2024.0.0
```

### MII + KBV combined

Some projects need to comply with both MII and KBV profiles — for example, a hospital that participates in MII research and also generates KBV eRezept prescriptions:

```bash
fhirlint validate patient.json \
  --ig kbv.basis#1.5.0 \
  --ig de.medizininformatikinitiative.kerndatensatz#2024.0.0
```

---

## DiGA — Digitale Gesundheitsanwendungen

DiGAs (digital health applications) must comply with the BfArM DiGA framework, which defines FHIR profiles for DiGA prescription codes and supporting resources.

### Validate against DiGA profiles

```bash
fhirlint validate diga-codierung.json --profile diga

# Equivalent
fhirlint validate diga-codierung.json --ig de.bfarm.diga#1.2.0
```

### DiGA in CI

```yaml
# fhirlint.yml for a DiGA project
fhir-version: 4.0.1
ig:
  - de.bfarm.diga#1.2.0
fail-on: error
suppress:
  - constraint: dom-6
```

---

## Combining profiles

A resource can be validated against multiple profiles simultaneously. fhirlint passes all `--profile` and `--ig` flags to the HL7 validator in a single invocation:

```bash
fhirlint validate patient.json \
  --profile kbv-basis \
  --ig de.medizininformatikinitiative.kerndatensatz#2024.0.0 \
  --profile https://fhir.my-hospital.de/StructureDefinition/HospitalPatient
```

All profiles must be satisfied — any violation from any profile is reported.

---

## Version pinning

The built-in aliases pin to specific IG versions that were current at the time fhirlint was released. To use a newer or different version, pass the package reference explicitly:

```bash
# Use a newer KBV basis version
fhirlint validate patient.json --ig kbv.basis#1.6.0

# Check what version the alias resolves to
fhirlint profiles
```

Pin the version in `fhirlint.yml` to ensure every team member and CI run uses the same IG:

```yaml
ig:
  - kbv.basis#1.5.0
  - de.medizininformatikinitiative.kerndatensatz#2024.0.0
```

---

## Using a specific profile URL

When a resource's `meta.profile` declares a specific profile URL, you can validate against that exact profile:

```bash
fhirlint validate patient.json \
  --ig kbv.basis#1.5.0 \
  --profile https://fhir.kbv.de/StructureDefinition/KBV_PR_Base_Patient
```

If the resource already has the correct `meta.profile` set, the HL7 validator will validate against that profile automatically — passing `--ig` is enough to make the package available.

---

## `fhirlint.yml` for German projects

A typical `fhirlint.yml` for a German FHIR project:

```yaml
# fhirlint.yml
fhir-version: 4.0.1
severity: warning
fail-on: error

ig:
  - kbv.basis#1.5.0

# Cache terminology responses — KBV code systems can be slow to resolve
tx-cache: .fhirlint-tx-cache/

# German locale for validation messages
locale: de

# dom-6 narrative warnings are accepted project-wide for M2M interfaces
suppress:
  - constraint: dom-6

format:
  - terminal
```

With this file committed to the repository, the CI step and all local runs share the same validation configuration:

```bash
fhirlint validate ./fhir/
```
