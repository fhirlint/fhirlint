# German FHIR profiles (KBV, MII, ISiK, DiGA)

fhirlint ships with built-in aliases for the most common German FHIR profiles and implementation guides. This guide explains what each profile covers, how to use the aliases, and how to combine them for real-world German FHIR projects.

## Table of contents

- [Built-in aliases](#built-in-aliases)
- [KBV — Kassenärztliche Bundesvereinigung](#kbv--kassenärztliche-bundesvereinigung)
- [MII — Medizininformatik-Initiative](#mii--medizininformatik-initiative)
- [Terminology: ICD-10-GM, OPS and ATC](#terminology-icd-10-gm-ops-and-atc-are-not-checked-by-default)
- [ISiK — gematik hospital interoperability](#isik--gematik-hospital-interoperability)
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
| `kbv-basis` | `kbv.basis#1.9.0` | KBV base profiles (Patient, Practitioner, Organization, …) |
| `kbv-patient` | `kbv.basis#1.9.0` | KBV Patient profile specifically |
| `mii` | all 15 modules below | MII core dataset, complete |
| `mii-person` | `de.medizininformatikinitiative.kerndatensatz.person#2025.0.1` | Patient, Practitioner, Organization |
| `mii-fall` | `de.medizininformatikinitiative.kerndatensatz.fall#2025.0.1` | Encounter |
| `mii-diagnose` | `de.medizininformatikinitiative.kerndatensatz.diagnose#2025.0.1` | Condition |
| `mii-prozedur` | `de.medizininformatikinitiative.kerndatensatz.prozedur#2025.0.1` | Procedure |
| `mii-laborbefund` | `de.medizininformatikinitiative.kerndatensatz.laborbefund#2026.0.3` | Observation, DiagnosticReport |
| `mii-medikation` | `de.medizininformatikinitiative.kerndatensatz.medikation#2026.0.1` | Medication, MedicationStatement, … |
| `mii-icu` | `de.medizininformatikinitiative.kerndatensatz.icu#2026.0.2` | Intensive care: ventilation, ECMO, device metrics |
| `mii-onkologie` | `de.medizininformatikinitiative.kerndatensatz.onkologie#2026.0.3` | Oncology: staging, therapy, tumour documentation |
| `mii-biobank` | `de.medizininformatikinitiative.kerndatensatz.biobank#2026.0.1` | Specimen and biobank data |
| `mii-consent` | `de.medizininformatikinitiative.kerndatensatz.consent#2026.0.0` | Broad consent |
| `mii-molgen` | `de.medizininformatikinitiative.kerndatensatz.molgen#2026.0.4` | Molecular genetics |
| `mii-patho` | `de.medizininformatikinitiative.kerndatensatz.patho#2026.0.2` | Pathology findings |
| `mii-studie` | `de.medizininformatikinitiative.kerndatensatz.studie#2026.0.2` | Clinical study metadata |
| `mii-mikrobiologie` | `de.medizininformatikinitiative.kerndatensatz.mikrobiologie#2025.0.2` | Microbiology |
| `mii-bildgebung` | `de.medizininformatikinitiative.kerndatensatz.bildgebung#2026.0.0` | Imaging |
| `diga` | `kbv.mio.diga#1.1.0` | KBV MIO DiGA Toolkit profiles |
| `isik` | all five modules below | gematik ISiK, complete |
| `isik-basis` | `de.gematik.isik-basismodul#4.0.3` | Patient, Encounter, Practitioner, Organization, Coverage, Condition, Procedure, … (18 resource types) |
| `isik-medikation` | `de.gematik.isik-medikation#4.0.3` | Medication, MedicationRequest, MedicationStatement, MedicationAdministration |
| `isik-terminplanung` | `de.gematik.isik-terminplanung#4.0.3` | Appointment, Schedule, Slot, HealthcareService |
| `isik-vitalparameter` | `de.gematik.isik-vitalparameter#4.0.2` | Observation (66 vital-sign profiles) |
| `isik-dokumentenaustausch` | `de.gematik.isik-dokumentenaustausch#4.0.1` | DocumentReference, document Bundle |

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

The Kerndatensatz has no umbrella package: it is published module by module. The
`mii` alias therefore stands for all six modules at once, and each module also
has its own alias.

### Validate against the MII core dataset

```bash
fhirlint validate observation.json --profile mii

# Equivalent
fhirlint validate observation.json \
  --ig de.medizininformatikinitiative.kerndatensatz.person#2025.0.1 \
  --ig de.medizininformatikinitiative.kerndatensatz.fall#2025.0.1 \
  --ig de.medizininformatikinitiative.kerndatensatz.diagnose#2025.0.1 \
  --ig de.medizininformatikinitiative.kerndatensatz.prozedur#2025.0.1 \
  --ig de.medizininformatikinitiative.kerndatensatz.laborbefund#2026.0.3 \
  --ig de.medizininformatikinitiative.kerndatensatz.medikation#2026.0.1
```

The modules are not released together: person, fall, diagnose and prozedur are
on the 2025 train and depend on `kerndatensatz.meta` 2025.0.x, while
laborbefund and medikation are on the 2026 train and depend on meta 2026.0.x.
Loading `mii` therefore pulls both versions of the meta package. The validator
accepts that, but if your project needs one coherent train, name the modules you
need rather than using the aggregate.

### Validate a specific MII module

```bash
# MII Laboratory module
fhirlint validate lab-observation.json --profile mii-laborbefund

# MII Diagnose module
fhirlint validate condition.json --profile mii-diagnose

# Or by package reference, to pin a different version
fhirlint validate condition.json \
  --ig de.medizininformatikinitiative.kerndatensatz.diagnose#2025.0.1
```

### MII + KBV combined

Some projects need to comply with both MII and KBV profiles — for example, a hospital that participates in MII research and also generates KBV eRezept prescriptions:

```bash
fhirlint validate patient.json \
  --ig kbv.basis#1.5.0 \
  --ig de.medizininformatikinitiative.kerndatensatz.person#2025.0.1
```

---

## Terminology: ICD-10-GM, OPS and ATC are not checked by default

The German base profiles bind to code systems whose content nobody publishes freely. `de.basisprofil.r4` declares them with `content=not-present`:

```
http://fhir.de/CodeSystem/bfarm/alpha-id      content=not-present
http://fhir.de/CodeSystem/bfarm/atc           content=not-present
http://fhir.de/CodeSystem/bfarm/icd-10-gm     content=not-present
http://fhir.de/CodeSystem/bfarm/ops           content=not-present
```

The concepts live in `bfarm.terminologien.*` packages on the BfArM's registry, behind a token. Checked against the alternatives:

| Source | Has ICD-10-GM, OPS, ATC, Alpha-ID |
|---|---|
| `packages.fhir.org` | the packages are not published there |
| `tx.fhir.org` | code systems not present |
| `tx.fhir.de` (Ontoserver, HL7 Germany) | 837 code systems, none German |
| `terminologien.bfarm.de/packages` | yes, behind a token |

So a German project is in this state by default, and it is not a misconfiguration. See the [README section](../README.md#german-code-systems-are-not-checked-by-default) for what a run looks like and the two ways to get real checking.

---

## ISiK — gematik hospital interoperability

ISiK ("Informationstechnische Systeme im Krankenhaus") is the gematik profile
set for hospital interoperability, published module by module as
`de.gematik.isik-*`. All five modules are R4 and share a 4.0.x train.

### Validate against ISiK

```bash
# The whole set
fhirlint validate patient.json --profile isik

# One module
fhirlint validate observation.json --profile isik-vitalparameter

# Equivalent with the full package reference
fhirlint validate patient.json --ig de.gematik.isik-basismodul#4.0.3
```

### Two version overlaps to know about

The modules do not agree on their base-profile pin. `isik-vitalparameter`
depends on `de.basisprofil.r4` 1.5.3 while the other modules depend on 1.5.2, so
`--profile isik` loads two versions of the German base profiles. The validator
accepts that; pin `de.basisprofil.r4` explicitly if your project needs one
version throughout.

`isik-medikation` depends on `hl7.fhir.uv.ips` 1.1.0, which is not the version
the `ips` alias pins (2.0.1). Combining `--profile isik --profile ips` therefore
loads both IPS versions.

`fhirlint packages tree` shows what a module actually pulls in, resolved against
what is in your cache:

```bash
fhirlint packages tree de.gematik.isik-medikation#4.0.3
```

### isik-labor is not aliased

The registry serves only a `4.0.0-rc` for `de.gematik.isik-labor`, with no
`dist-tags.latest` — there is no published release to pin, so it gets no alias.
Name it explicitly if you need the release candidate:

```bash
fhirlint validate labor.json --ig de.gematik.isik-labor#4.0.0-rc
```

---

## DiGA — Digitale Gesundheitsanwendungen

DiGAs (digital health applications) export patient data through the **MIO DiGA
Toolkit**, published by the KBV as `kbv.mio.diga` — 48 profiles covering the
export Bundle, Observation, CarePlan, Questionnaire and the rest. The BfArM
defines the regulatory framework but publishes no FHIR package of its own, which
is what the `diga` alias pointed at until #335.

### Validate against DiGA profiles

```bash
fhirlint validate diga-export.json --profile diga

# Equivalent
fhirlint validate diga-export.json --ig kbv.mio.diga#1.1.0
```

`kbv.mio.diga` depends on `kbv.basis` 1.3.0. Combining `diga` with `kbv-basis`
therefore loads two versions of `kbv.basis`; the validator accepts that, but pin
`kbv.basis#1.3.0` explicitly if you need one version for the whole run.

### DiGA in CI

```yaml
# fhirlint.yml for a DiGA project
fhir-version: 4.0.1
ig:
  - kbv.mio.diga#1.1.0
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
  --ig de.medizininformatikinitiative.kerndatensatz.person#2025.0.1 \
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
  - de.medizininformatikinitiative.kerndatensatz.person#2025.0.1
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
