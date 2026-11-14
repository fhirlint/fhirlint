# The toolchain

The FHIR package cache, the validator JAR, update notices, and auditing the toolchain.

## The FHIR package cache

IG packages live in `~/.fhir/packages`, the cache the validator JAR populates and the whole FHIR toolchain shares — the IG publisher and sushi write there too. fhirlint only ever reads it: nothing in fhirlint removes or renames anything in that directory, because another tool may be mid-way through using it.

`FHIRLINT_CACHE_DIR` does **not** move it. That variable relocates fhirlint's own cache; the package cache is resolved by the validator JAR from its own `user.home`, and an override only fhirlint honoured would point the two at different directories.

### What is installed

```bash
fhirlint packages              # grouped by package, alphabetical
fhirlint packages --sort size  # largest first
fhirlint packages --json
```

```text
FHIR package cache — 61 package(s), 1.2 GB

hl7.terminology.r4
  6.2.0              64 MB  FHIR 4.0.1
  7.2.0              61 MB  FHIR 4.0.1
kbv.basis  (installed under more than one spelling)
  1.1.0             239 KB  FHIR 4.0.1     KBV.Basis
  1.9.0               1 MB  FHIR 4.0.1     kbv.basis
```

Versions of one package are grouped rather than scattered through an alphabetical listing. Packages published under more than one capitalisation are folded into one group and flagged — older KBV releases used `KBV.Basis`, newer ones `kbv.basis`, and both end up on disk as separate copies of the same thing. The directory names themselves are never rewritten.

### What a package depends on

```bash
fhirlint packages tree de.medizininformatikinitiative.kerndatensatz.diagnose#2025.0.1
```

```text
de.medizininformatikinitiative.kerndatensatz.diagnose#2025.0.1
├── de.basisprofil.r4#1.5.4   (declared 1.5.x)
│   └── hl7.fhir.r4.core#4.0.1
├── de.medizininformatikinitiative.kerndatensatz.meta#2025.0.3   (declared 2025.0.x)
│   └── hl7.fhir.r4.core#4.0.1   …
└── kbv.all.st#1.27.0
    └── hl7.fhir.r4.core#4.0.1   …

All dependencies are in the cache.
2 declared as a version range; the resolved version shown is what is installed now.
```

Resolved entirely from manifests on disk, so it works offline. Use `--depth N` to stop at N levels of dependencies, and `--json` for the whole tree as a structure.

Two markers matter:

| | meaning |
|---|---|
| `MISSING` | declared but not in the cache — a run with `--offline` would fail on it |
| `(declared 1.5.x)` | the dependency is a **range**, not a pin; the version shown is what happens to be installed now |

That second one is why [`fhirlint.lock`](pipeline.md#ig-lock-file) records a manifest SHA256 rather than trusting version strings: a manifest alone does not determine which version a run resolves to.

A `…` marks a package already expanded elsewhere in the tree, or one cut off by `--depth`.

## JAR management

`fhirlint` uses the official [HL7 FHIR Validator](https://github.com/hapifhir/org.hl7.fhir.core/releases) maintained by HAPI FHIR. The JAR (~250 MB) is downloaded automatically on first use and cached at `~/.fhirlint/validator_cli.jar`.

```bash
# Show fhirlint version and the cached JAR version
fhirlint version

# Update the cached JAR to the latest release
fhirlint update
```

To use a specific or pre-downloaded JAR:

```bash
# Via flag
fhirlint validate patient.json --jar /path/to/validator_cli.jar

# Via environment variable
export FHIRLINT_JAR=/path/to/validator_cli.jar
fhirlint validate patient.json
```

### Update notices

When a newer fhirlint release or validator JAR exists, `fhirlint version` and the end of a `validate` run say so:

```text
Updates available:
  fhirlint   v1.16.0 → v1.17.0  https://github.com/fhirlint/fhirlint/releases
  validator  6.9.0 → 6.10.2     run: fhirlint update
```

The lookup is cached for 24 hours, times out after 3 seconds, and fails silently — it never delays or breaks a run. The notice goes to stderr, so it never contaminates a report on stdout.

**Advisories are reported separately, and are harder to silence.** When a published security advisory affects the validator you have installed, a second line appears:

```text
Security: 2 published advisory/advisories affect validator 6.9.11 (high or worse). Run: fhirlint audit
```

It is its own block rather than a column on the validator row, because it has to appear whether or not a newer release exists, and because being behind and being exposed are different things.

This matters most for pinned projects. An advisory is usually published well after the release it describes, so a pin that was clean in July can be affected in September without anything changing on your side. The lookup is cached for 24 hours like the release check, and only counts advisories that affect **your** version.

It stays visible with `CI` set and in a redirected log, unlike the update rows. A pipeline cannot upgrade itself, but whoever reads the log can act, and a pinned project is exactly the one that misses the news. Only `--offline` and `FHIRLINT_NO_UPDATE_NOTIFIER` silence it.

The update rows stay quiet when there is nobody to act on them: with `CI` set, when stderr is not a terminal, under `--offline`, and whenever `FHIRLINT_NO_UPDATE_NOTIFIER` is set to anything:

```bash
export FHIRLINT_NO_UPDATE_NOTIFIER=1
```

Builds installed with `go install` or built from source are not compared against releases — there is no release version to compare them to, so no notice appears.

### Moving the cache

`FHIRLINT_CACHE_DIR` relocates everything fhirlint caches — the JAR, the version, verification-status and update-check files, and the `--cache` result cache:

```bash
export FHIRLINT_CACHE_DIR=/mnt/build-cache/fhirlint
```

Useful for containers whose home directory is read-only, CI runners that cache a mounted volume between jobs, and working on two projects pinned to different validator versions without re-downloading on every switch.

`--cache-dir` still overrides the result-cache location on its own, and takes precedence over `FHIRLINT_CACHE_DIR` for that one directory.

## Auditing the toolchain

Two things decide what a validation run reports: the validator JAR and the IG packages. `fhirlint audit` checks both.

```bash
fhirlint audit
```

```text
Validator JAR
  current:  6.10.1
  latest:   6.10.2
  ✗ outdated — run: fhirlint update

Security advisories (hapifhir/org.hl7.fhir.core)
  ✓ 15 advisory/advisories published, none affect your version (6.10.1)

IG packages (fhirlint.lock)
  ✓ de.gematik.isik-basismodul  4.0.3 — current
  ✗ hl7.fhir.de.core            1.0.0 — not found on any registry (packages2.fhir.org, packages.fhir.org)
  ✓ hl7.fhir.r4.core            4.0.1 — current
  ✗ kbv.basis                   1.4.0 → 1.9.0 available
  (2 of 4 package(s) need attention)
```

The JAR half checks the installed version against the latest release and against the published [security advisories](https://github.com/hapifhir/org.hl7.fhir.core/security/advisories) for `org.hl7.fhir.core`.

The IG half asks the FHIR package registries about each of your packages — `packages2.fhir.org` first and `packages.fhir.org` second, the same two hosts in the same order the validator itself uses, so that a package `validate` can load is never reported as missing here. "Not found" means both said so; a registry that could not be reached is reported as an error, not as a finding. The packages are taken from the first source it finds:

1. **[`fhirlint.lock`](pipeline.md#ig-lock-file)**, when there is one. Preferred, because it pins exactly what a run resolved to — including the transitive packages your config never names.
2. **the `ig:` list in your config**, otherwise. Most projects have no lock file, and auditing what they did declare beats skipping the half of the audit they ran the command for.

The header names which one was used, and the config fallback adds a note:

```text
IG packages (fhirlint.yml)
  ✓ de.medizininformatikinitiative.kerndatensatz.person  2025.0.1 — current
  ✗ kbv.basis                                            1.5.0 → 1.9.0 available
  ! ./local-ig                                           not pinned — no version to check
  (1 of 2 package(s) need attention)
  note: from the ig: list — run 'fhirlint validate --lock' to pin transitive packages too
```

Entries that name no registry version — a bare package name, a local directory, `#latest` — are listed as not pinned rather than dropped silently: an unpinned IG is itself worth seeing in an audit. With neither source the section is skipped with a hint, and the JAR check still runs.

The audit is scoped to your project, not to the machine. It never audits the whole [package cache](#the-fhir-package-cache), which accumulates across every project that machine has ever validated.

In `--format json`, `igSource` names the file the packages came from and `igUnpinned` lists the entries that could not be checked. `lockFile` keeps naming the lock file only, so consumers reading that field are unaffected. Each package entry carries `untaggedNewer`, the same list as the note described below, and `registry`, the base URL of the host that answered for it.

A package is reported as:

| | meaning |
|---|---|
| `→ X available` | a newer version exists upstream |
| `not found on any registry` | neither packages2.fhir.org nor packages.fhir.org has the package — the pin will fail on a cold cache |
| `deprecated upstream` | the publisher marked this version deprecated |
| `registry latest is X (versions not comparable)` | the versions differ but could not be ordered |
| `ahead of registry latest` | you pinned a version newer than the registry's `latest`, e.g. a pre-release |
| `current release; registry latest is a pre-release (X)` | the publisher's `latest` tag points at a ballot or release candidate, and your pin is the last final |
| `also on the registry, not tagged latest: X` | released versions newer than both your pin and `latest`, which the publisher has not made current |

The `versions not comparable`, `ahead` and `registry latest is a pre-release` rows are worth explaining. FHIR IG versions are usually semver, but the registry does not enforce it, so fhirlint only calls a package *outdated* when it could actually establish that your pin is older. When a publisher switches versioning scheme, you are told the versions differ rather than being given a confident but invented direction.

A `latest` tag can also point at a *pre-release*. Publishers move it during a ballot round: as of September 2026, all twelve `de.medizininformatikinitiative.kerndatensatz.*` modules tag a `2027.0.0-ballot*` version while their last final release is from 2026. A ballot is a draft for comment, not something to pin a pipeline to, so fhirlint does not call your final pin outdated and does not count it as a finding — it says the tag moved and leaves the decision to you. This applies only when your pin is itself a final; if you are tracking a ballot round deliberately, a newer pre-release is still reported as outdated, because that is the release you asked to follow.

The final row is a note rather than a finding, and it can appear under a package that is otherwise `current`. fhirlint pins and compares against `dist-tags.latest` — what the publisher says is current — not against the highest number on the registry, because publishing a tarball and blessing it are separate acts. The gap between the two is real and can last a long time: at the time of writing, `fhir.r4.ukcore.stu2` serves 2.1.0 while tagging 2.0.2, and `de.medizininformatikinitiative.kerndatensatz.icu` serves the finals 2026.0.3 and 2027.0.0 while tagging 2027.0.0-ballot.3. Nothing is wrong with a pin that follows the tag, but you should be able to see the difference without reading the packument yourself. Pre-releases are excluded from this list, as are versions that could not be ordered.

### Exit codes

| code | meaning |
|---|---|
| `0` | nothing to report |
| `1` | the JAR is outdated or missing, or an advisory affects the installed version |
| `2` | only IG package findings |

An outdated IG package is a maintenance signal and an advisory against the JAR is a security one, so they get separate codes. `1` keeps the meaning it always had, and a JAR problem takes precedence when both are present.

`--format json` prints the whole report and always exits `0`, so a pipeline can decide for itself:

```bash
fhirlint audit --format json | jq '.igPackages[] | select(.outdated)'
```
