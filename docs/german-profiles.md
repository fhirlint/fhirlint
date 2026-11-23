# German FHIR profiles (KBV, MII, ISiK, DiGA)

fhirlint ships with built-in aliases for the most common German FHIR profiles and implementation guides. This guide explains what each profile covers, how to use the aliases, and how to combine them for real-world German FHIR projects.

## Table of contents

- [Built-in aliases](#built-in-aliases)
- [KBV — Kassenärztliche Bundesvereinigung](#kbv--kassenärztliche-bundesvereinigung)
- [MII — Medizininformatik-Initiative](#mii--medizininformatik-initiative)
- [Terminology: ICD-10-GM, OPS and ATC](#terminology-icd-10-gm-ops-and-atc-are-not-checked-by-default)
- [Terminology: the German SNOMED CT edition](#terminology-the-german-snomed-ct-edition)
- [Display language: `--language`, not `--locale`](#display-language---language-not---locale)
- [ISiK — gematik hospital interoperability](#isik--gematik-hospital-interoperability)
- [DiGA — Digitale Gesundheitsanwendungen](#diga--digitale-gesundheitsanwendungen)
- [Combining profiles](#combining-profiles)
- [Version pinning](#version-pinning)
- [Using a specific profile URL](#using-a-specific-profile-url)
- [Large runs: the timeout](#large-runs-the-timeout)
- [fhirlint.yml for German projects](#fhirlintyml-for-german-projects)

---

## Built-in aliases

```bash
# List all available aliases
fhirlint profiles
```

| Alias | IG package | Covers |
|-------|-----------|--------|
| `kbv-basis` | `kbv.basis#1.10.0` | KBV base profiles (Patient, Practitioner, Organization, …) |
| `kbv-patient` | `kbv.basis#1.10.0` | KBV Patient profile specifically |
| `mii` | all 16 packages below | MII core dataset, complete |
| `mii-base` | `de.medizininformatikinitiative.kerndatensatz.base#2026.0.1` | Patient, Encounter, Condition, Procedure, Vitalstatus — replaces person/fall/diagnose/prozedur |
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
| `mii-dokument` | `de.medizininformatikinitiative.kerndatensatz.dokument#2026.0.1` | Clinical documents (DocumentReference) |
| `mii-mtb` | `de.medizininformatikinitiative.kerndatensatz.mtb#2026.0.1` | Molecular tumour board: findings, recommendations, therapy plans |
| `mii-pros` | `de.medizininformatikinitiative.kerndatensatz.pros#2026.7.0` | Patient-reported outcomes: questionnaires (PHQ, GAD-7, …) and scores |
| `mii-seltene` | `de.medizininformatikinitiative.kerndatensatz.seltene#2026.0.1` | Rare diseases: diagnoses, family history, care plans |
| `diga` | `kbv.mio.diga#1.1.0` | KBV MIO DiGA Toolkit profiles |
| `isik` | `de.gematik.isik#6.0.0` | gematik ISiK, one package: 178 profiles across 39 resource types |

Aliases resolve to the full IG package reference at runtime. They are a convenience shortcut — you can always use the full package reference directly if you need a specific version.

fhirlint also ships international aliases (`us-core`, `ips`, `ipa`, `uk-core`) — run `fhirlint profiles` or see the [Built-in profile aliases](usage.md#built-in-profile-aliases) table for the full list.

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

**`Could not find the code system http://loinc.org|2.83` from tx.fhir.de** (#712). The examples of `kbv.basis#1.10.0` give their LOINC codings `"version": "2.83"`. The validator sends LOINC, through the terminology registry, to tx.fhir.de, and on 2026-11-22 that server had LOINC 2.82 only. For each such code it answers with an error, which the validator reports as:

```text
error  Error from https://tx.fhir.de/fhir: Error: [<request id>]: Could not find the code system http://loinc.org|2.83
```

The finding has no message id, and its text carries a new request id on every run. So neither a `messageId` suppression nor a baseline holds it. 6.10.4, 7.0.5 and 7.0.6 behave the same: it is not a validator regression, and the resource is not wrong. Resources of your own hit it in the same way when they state LOINC 2.83.

You can do one of two things until tx.fhir.de serves 2.83:

- **Suppress exactly this message.** Give the rule a date, so that it lapses on its own once the server has caught up:

  ```yaml
  # fhirlint.yml
  suppress:
    - pattern: 'Could not find the code system http://loinc\.org\|2\.83'
      reason: "tx.fhir.de has no LOINC 2.83 yet (fhirlint#712)"
      expires: 2027-03-31
  ```

  Put it in `fhirlint.yml` rather than on the command line: `--suppress` reads whatever follows a `|` as the reason, which would cut this pattern in two.

- **Validate against tx.fhir.org alone** with `--terminology-server https://tx.fhir.org/r4`. An explicit server turns the registry off, so LOINC 2.83 resolves. In exchange, tx.fhir.org does not know BfArM's OPS, so OPS codes are no longer checked ("Code System unknown, so assuming value set expansion is correct").

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

```text
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

So a German project is in this state by default, and it is not a misconfiguration. See [German code systems are not checked by default](terminology.md#german-code-systems-are-not-checked-by-default) for what a run looks like and the two ways to get real checking.

---

## Terminology: the German SNOMED CT edition

German instances code SNOMED CT from the German edition, and say so in `Coding.version`:

```json
{
  "system": "http://snomed.info/sct",
  "version": "http://snomed.info/sct/11000274103/version/20260515",
  "code": "119376003",
  "display": "Tissue specimen"
}
```

The value sets they are bound to usually include SNOMED CT without a version. The validator then fills in the edition it was told to assume, and without `-sct` that is the international one (`900000000000207008`). The two disagree, and every such coding becomes an error:

```text
The code system 'http://snomed.info/sct' version 'http://snomed.info/sct/900000000000207008'
resulting from the version '' in the ValueSet include is different to the one in the value
('http://snomed.info/sct/11000274103/version/20260515')
```

Tell it which edition the instances use:

```bash
fhirlint validate ./fhir/ --snomed-edition de
```

or `snomed-edition: de` in `fhirlint.yml`. A run without it names the flag on these errors, once per file in the terminal:

```text
  ✗ ERROR  The code system 'http://snomed.info/sct' version 'http://snomed.info/sct/900000000000207008' resulting from …
           @ Condition.severity.coding[0].version (line 89, col 4)
           ↳ the validator assumed the international SNOMED CT edition, the coding states another: add --snomed-edition de
```

The flag takes the names the validator knows (`de`, `intl`, `at`, `ch`, `uk`, `nl`, ...; an unknown name lists them) or a module id such as `11000274103`, and passes the module id as `-sct`.

Measured with validator 7.0.6 on the 41 examples of `de.gematik.epa.laboratory#1.0.0-ballot.1` (2026-11-22):

| | errors | `VALUESET_VALUE_MISMATCH_CHANGED` |
|---|---|---|
| default (international edition) | 24 | 14 |
| `--snomed-edition de` | 10 | 0 |

The 14 errors go, and with them the "None of the codings provided are in the value set" messages they caused. In return the validator now gets far enough to report what it cannot check: two codings from a German edition release the terminology server does not have yet (`UNABLE_TO_CHECK_IF_THE_PROVIDED_CODES_ARE_IN_THE_VALUE_SET_CS`), and two NPU codes. No new error.

On the 81 examples of `kbv.basis#1.10.0` the same holds, with a second form of the error. Where the binding is required, the mismatch arrives as `Terminology_TX_Confirm_4a`, `The code provided (http://snomed.info/sct#364075005) was not found in the value set ...`, with the edition clash only at the end of the message:

| | errors | `VALUESET_VALUE_MISMATCH_CHANGED` | `Terminology_TX_Confirm_4a` |
|---|---|---|---|
| default (international edition) | 83 | 29 | 28 |
| `--snomed-edition de` | 27 | 0 | 0 |

One error is new, and it is real: an example's display `Clinical Finding` for 404684003, where SNOMED CT has `Clinical finding`. The edition clash had hidden it.

fhirlint does not set the edition for the German aliases. The edition belongs to the instances, not to the package: a project that codes the international edition against a KBV profile would get the same false errors the other way round, from a default it did not choose.

---

## Display language: `--language`, not `--locale`

The two options sound alike and do different things:

| Option | Validator flag | What it changes |
|---|---|---|
| `--locale de` | `-locale` | the language of the **messages** fhirlint shows you |
| `--language de-DE` | `-language` | the language **coding displays are checked against** |

Without `--language` the validator expects English displays. A German display that is the code system's own German designation is then an error, usually followed by a second one about value-set membership:

```text
Wrong Display Name 'Apfel' for http://example.org/fhir/CodeSystem/fruit#apple. Valid display is 'Apple' (for the language(s) 'en')
```

Use `de-DE`, not `de`. The validator accepts a designation when the requested language starts with the designation's tag (`LanguageUtils.langsMatch`), so:

| Designation tagged | `--language de` | `--language de-DE` |
|---|---|---|
| `de` (KBV) | accepted | accepted |
| `de-DE` (de.basisprofil, most of gematik) | **rejected** | accepted |

Both forms are common. In a local package cache on 2026-11-23, KBV tagged its 4,080 German designations `de`, de.basisprofil its 2,966 `de-DE`, gematik 657 `de-DE` and 152 `de`. Measured with validator 6.10.4 and 7.0.6, `-tx n/a`. An English display stays valid with `--language de-DE` when the code system is local.

A resource's own `language` element takes precedence over `--language`.

### What it does not fix

`--language` covers the code systems in your packages. For LOINC and SNOMED CT the validator passes the language to the terminology server, and the answer depends on what the server has. Measured on the 41 examples of `de.gematik.epa.laboratory#1.0.0-ballot.1` with 7.0.6:

| | errors | wrong display |
|---|---|---|
| no `--language` | 24 | 8, all German LOINC displays (`Harnsäure [Masse/Volumen] in Serum oder Plasma`) |
| `--language de` | 34 | 12 |
| `--language de-DE` | 31 | 9 |

With `de-DE`, the German LOINC displays pass, because tx.fhir.org has LOINC's `de-DE` designations. But other examples use English displays (`Laboratory report` for LOINC 11502-2), and the server now wants `Laborbericht`. SNOMED CT's German designations are tagged `de-x-sctlang-31000274-107`, which `de-DE` does not start with, so a German SNOMED display is not accepted either. These examples mix the two languages, so no single setting validates all of them. For your own instances, set `--language` to the language their displays are written in. If they are deliberately mixed, `--display-issues-are-warnings` keeps display mismatches out of the error count.

---

## ISiK — gematik hospital interoperability

ISiK ("Informationstechnische Systeme im Krankenhaus") is the gematik profile
set for hospital interoperability. Since Stufe 5 it ships as **one package**,
`de.gematik.isik`.

```bash
fhirlint validate patient.json --profile isik

# Equivalent
fhirlint validate patient.json --ig de.gematik.isik#6.0.0
```

### One package, not five

The per-module packages (`de.gematik.isik-basismodul`, `-medikation`,
`-terminplanung`, `-vitalparameter`, `-dokumentenaustausch`) are frozen at
`4.0.x`, which is ISiK Stufe 4. The spec tags make the mapping explicit:
[`v.5.0.0`](https://github.com/gematik/spec-ISiK-Basismodul/releases) is titled
"ISiK Stufe 5" and `v.6.0.0` is "ISiK Stufe 6".

`de.gematik.isik` 6.0.0 carries 178 profiles across 39 resource types, against
131 in the five old modules combined, and adds ICU (Device, DeviceMetric) and
laboratory content that never had its own published package.

There are no `isik-<module>` aliases. There is no separate package left to
narrow a load to, and an alias suggesting otherwise would resolve to the whole
package anyway.

fhirlint's aliases pinned the Stufe 4 modules until v1.12.0; if you were relying
on `isik-medikation` or similar, use `--profile isik` and select the profile you
need per resource.

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

## Large runs: the timeout

The German IGs on this page are large: ISiK carries 178 profiles across 39 resource types, and the `mii` alias loads all 16 MII packages. Validating a whole implementation guide — its examples, not one resource — therefore takes longer than the default `--timeout` of `5m` allows, and the timeout kills the Java process, so the run fails without a result.

Give such a run more time:

```bash
fhirlint validate ./fhir/ --profile isik --timeout 15m
```

Or set it for the project in `fhirlint.yml`:

```yaml
timeout: 15m
```

The default stays at `5m`: it fits single files and a CI job that validates only the changed files.

To keep repeated runs fast, record the terminology once and replay it, so the validator does not wait for the server on every run:

```bash
fhirlint tx warm ./fhir/ --profile isik
fhirlint validate ./fhir/ --profile isik --tx-offline
```

See [Offline terminology](terminology-offline.md) for the recording.

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

# Whole-IG runs take longer than the 5m default
timeout: 15m

# German validation messages (not display checking, see language)
locale: de

# Check coding displays against German designations, tagged de-DE or de
language: de-DE

# German instances code the German SNOMED CT edition
snomed-edition: de

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
