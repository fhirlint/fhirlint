# Pipeline integration

Everything that keeps a pipeline run reproducible and bounded: the lock file, pinning the validator, bounds, crashes, missing profiles, the result cache and validating only what changed. For the CI setups themselves, see [CI/CD integration](ci.md).

## Pipeline integration

```bash
# Exit non-zero if any errors are found (default)
fhirlint validate patient.json --fail-on error

# Exit non-zero for warnings too
fhirlint validate patient.json --fail-on warning

# Never fail on validation issues (reporting-only runs)
fhirlint validate patient.json --fail-on never
```

### IG lock file

`--lock` writes a `fhirlint.lock` file containing SHA256 hashes of all resolved IG packages **and the validator version they were validated against**. On subsequent runs (without `--lock`), fhirlint verifies that cached packages match the recorded hashes and that the validator in use is the recorded one.

```bash
# Generate or update the lock file
fhirlint validate ./fhir/ --lock

# Subsequent runs verify the lock automatically (no flag needed)
fhirlint validate ./fhir/
```

Commit `fhirlint.lock` to version control. This prevents silent package changes from affecting CI results.

#### Packages the validator loads on its own

The validator also loads packages nobody asked for. When the input names a canonical, such as a US Core profile in `meta.profile`, it loads the package that defines it, in the **newest version, pre-releases included**. One US Core Patient validated without `--ig` makes 6.10.3 and 7.0.3 load 15 more packages, among them `hl7.fhir.us.core#9.0.0` and the pre-release `hl7.fhir.uv.extensions.r4#5.3.0-ballot-tc1`. When a new version is published, the same run can report different findings.

fhirlint names them once per run on stderr, and in the JSON report as `meta.implicitPackages`:

```text
note: the validator loaded 15 package(s) that no --ig, fhirlint.yml or fhirlint.lock named, and the findings depend on them: hl7.fhir.r4.examples#4.0.1, hl7.fhir.us.core#9.0.0, …
warn: 2 of them are pre-releases: hl7.fhir.uv.extensions#5.1.0-snapshot1, hl7.fhir.uv.extensions.r4#5.3.0-ballot-tc1 — pin the version you mean with --ig <id>#<version>
```

It also warns when one of them has the id of a requested package in another version. The notes do not affect the exit code. The lock file is what turns a change into a failure: `--lock` records these packages in an `implicit` section, and from then on a package outside that section, or with changed content, fails the run. The message names the fix: pin the version with `--ig <id>#<version>`, or accept it with `--lock`. A lock file written before this section existed only gets a warning. [Server mode](daemon.md) does not report implicit packages.

Pinning keeps a run reproducible, but it does not tell you whether the pins are still the right ones. [`fhirlint audit`](toolchain.md#auditing-the-toolchain) reads the lock file and reports which packages have moved on upstream.

### Pinning the validator

The IG packages are only half the input. By default fhirlint downloads whatever HL7 published most recently, so a fresh CI runner can pick up a new validator and report different findings from the same sources and the same lock file.

`--validator-version` pins it:

```yaml
# fhirlint.yml
validator-version: "6.9.12"
```

```bash
fhirlint validate ./fhir/ --validator-version 6.9.12
```

The cache holds one JAR at a time, so switching between pinned versions re-downloads it. Move a pin deliberately with:

```bash
fhirlint update --validator-version 6.9.13
```

The lock file records the version in use, and a later run against a different validator fails rather than quietly producing different results:

```text
Error: lock file expects validator 6.9.12, running 6.9.13 — pin it with
--validator-version 6.9.12, or run with --lock to accept 6.9.13
```

#### Before you bump the pin

`fhirlint upgrade-check` shows what a new validator changes on your data. It validates the same input with the validator you use now and with `--to`, then compares the findings:

```bash
fhirlint upgrade-check --to 7.0.3 ./fhir/ --ig kbv.basis#1.10.0
```

The output looks like this (an illustration built from the changes seen between 6.10.3 and 7.0.3, not one recorded run):

```text
Validator upgrade check
  from: 6.10.3
  to:   FHIR Validation tool Version 7.0.3 (Git# 28e9fac6d79d). Built 2026-10-21T16:46:58.228Z

Per file:
  fhir/Patient-1.json  +1 new  -0 resolved  ~1 renamed

New (1):
  warning fhir/Patient-1.json  UNKNOWN_CODESYSTEM_VERSION @ Patient.identifier[0].type
          A definition for CodeSystem … version '2026' could not be found …

Renamed ids (1):
  MSG_DRAFT → MSG_DRAFT_SRC_STATUS  fhir/Patient-1.json @ Patient.meta.profile[0]

Per message id (changed counts only):
  MSG_DRAFT                                             1 → 0
  MSG_DRAFT_SRC_STATUS                                  0 → 1
  UNKNOWN_CODESYSTEM_VERSION                            0 → 1
```

- **Arguments:** everything after `--to` (and an optional `--from <version>`) goes to `validate` unchanged, `fhirlint.yml` included, so both runs use the same options.
- **The comparison JAR** is downloaded and verified into its own directory in the cache. It does not replace the default JAR or your pin.
- **Matching** uses the same key as `fhirlint diff`, inferred message ids included. A resolved and a new finding at the same place with similar text are reported as a rename.
- **`--json`** gives the same as JSON. The exit code is 1 when the new validator reports new errors, so the command can run as a scheduled CI job.
- **Cost:** two full validation runs.
- **Terminology:** against a live server, the two runs can get different answers. Record them with `fhirlint tx warm` and pass `--tx-offline` for an exact comparison; otherwise the command warns.

Lock files written before this existed carry no validator version; those runs warn instead of failing, so nothing breaks on upgrade. `--jar` takes precedence over a pin — you are pointing at a specific file — and warns if that file is a different version than the pin.

### Bounding a run

A pathological or very large input can stall a pipeline or produce a report nobody reads. Two bounds cap it:

```bash
fhirlint validate ./fhir/ --validation-timeout 2m --max-messages 500
```

Both ask the validator to stop and hand back the issues it found so far, so the run still reports something useful. That makes them different from `--timeout`, which kills the Java process outright and yields nothing:

| | Stops | Result |
|---|---|---|
| `--timeout` | the JVM | nothing — the run fails |
| `--validation-timeout` | validation | partial findings, run reported as inconclusive |
| `--max-messages` | validation | partial findings, run reported as inconclusive |

**Hitting a bound fails the run.** This is not caution for its own sake: when the validator stops early it returns only the messages gathered so far, so files with real errors come back with none and are counted as valid. The same input that reports `Files: 2  Valid: 0  Errors: 5` unbounded reports `Files: 2  Valid: 2  Errors: 0` under `--max-messages 1`. Exiting 0 there would turn a bound into a way to make a red pipeline green.

```text
Error: validation stopped early because --max-messages was reached, so the results
are partial and files with errors may be reported as valid — raise the bound, or
set --fail-on never to accept partial results
```

Set `--fail-on never` if you genuinely want partial results accepted.

Memory is bounded by the JVM, not by fhirlint. Without a cap the JVM takes a quarter of the machine's RAM. Set the heap through `JDK_JAVA_OPTIONS`, which the `java` launcher has read since Java 9; `JAVA_OPTS` is only a convention of start scripts and has no effect here:

```bash
JDK_JAVA_OPTIONS="-Xmx2g" fhirlint validate ./fhir/
```

For CI that validates pull requests from other people, see [Limits for untrusted pull requests](ci.md#limits-for-untrusted-pull-requests).

### When the validator crashes

Some inputs make the validator throw instead of reporting, and when it does it writes no output for any file in the run. fhirlint contains that: it reports the file the validator was on as `fatal` with message id `fhirlint-validator-crash` and the exception's first line, then validates the other files again in a new JVM, so one such file does not hide every other finding.

```text
warn: the validator crashed on in/BodyStructure-0c5948ba-….json; re-running the other 12 file(s) in a new JVM
```

Each re-run costs a JVM start. After three crashes in one run, or when `--timeout` runs out during a re-run, the files still waiting are reported as `fatal` with `fhirlint-not-validated` rather than counted as valid. Neither kind of result goes into the `--cache`. If fhirlint cannot tell which file the validator was on, for example because it failed while loading packages, the run fails with `validator produced no result` as before.

### When a profile cannot be found

"Profile reference … has not been checked because it could not be found" almost always means that the package defining the profile was not loaded. fhirlint looks the canonical up in the `.index.json` files of your local package cache (`~/.fhir/packages`) and names the package:

```text
  ⚠ WARN   Profile reference 'https://fhir.kbv.de/StructureDefinition/KBV_PR_Base_Patient|1.10.0' has not been checked because it could not be found, …
           @ Patient (line 1, col 2)
           ↳ defined in kbv.basis#1.10.0 (also kbv.basis#1.9.0): add --ig kbv.basis#1.10.0, or the built-in alias --profile kbv-basis
```

It suggests the highest final release, lists every installed version, and never picks one silently. When no installed package defines the canonical, the hint says so and points to the [package registry](https://registry.fhir.org). The same applies to extensions and value sets that could not be found. JSON carries the hint as `"hint": {"canonical", "package", "packages", "alias"}`, the other reports add a `Hint:` line, and the language server shows it on the diagnostic. The index is read only when a run has such a finding. `--redact` removes it.

### When a profile has no snapshot

When the validator cannot generate a profile's snapshot, its report says only that the profile "has no snapshot". The reason is printed on the validator's console. fhirlint reads it from there and attaches it to the finding:

```text
  ✗ ERROR  StructureDefinition http://example.org/fhir/StructureDefinition/Slice-nonrepeating|1.0.0 has no snapshot - validation is against the snapshot, so it must be provided
           @ Patient (line 1, col 2)
           ↳ cause: Attempt to a slice an element that does not repeat: Patient.maritalStatus/Patient.maritalStatus … at element married (slice = {5})
```

JSON carries it as `"cause"` on the issue, and the other reports add a `Cause:` line to the message. If no finding names the profile, the failure is reported as an `error` of its own, with message id `fhirlint-snapshot-failed`. `--redact` removes the cause along with the message. [Server mode](daemon.md) does not read the console yet, so its findings have no cause.

### Result caching

`--cache` caches validation results per file content hash. Unchanged files are not re-validated, which significantly speeds up repeated runs in CI.

The key covers the file's content and every option that changes what the validator reports — the FHIR version, profiles and IGs, but equally `--best-practice`, `--locale`, `--jurisdiction`, terminology settings, and the effective validator version, so an entry is never shared across runs that would legitimately disagree. Files passed by path (`--expansion-parameters`, `--fhir-settings`, `--po`) enter the key by content, so editing one invalidates the entry and moving it does not.

```bash
fhirlint validate ./fhir/ --cache
fhirlint validate ./fhir/ --cache --cache-dir .fhirlint-cache/
```

Cache the result directory in CI with `actions/cache`. Keep it outside the checkout:

```yaml
- uses: actions/cache@v4
  with:
    path: ${{ runner.temp }}/fhirlint-cache
    key: fhirlint-results-${{ runner.os }}-${{ hashFiles('fhirlint.yml', 'fhir/**') }}
    restore-keys: fhirlint-results-${{ runner.os }}-

- name: Validate FHIR resources
  run: fhirlint validate ./fhir/ --cache --cache-dir "${{ runner.temp }}/fhirlint-cache"
```

A cache entry is found by a hash of the file and the options, which anyone can compute. A cache directory inside the checkout can therefore be filled by the pull request being checked, with entries that report its files as valid. fhirlint refuses such a directory when it comes from `fhirlint.yml`, or is the default under a `HOME` that points into the workspace, and then runs without the cache. It also refuses a cache directory owned by another user. It never reads or writes an entry through a symbolic link. A `--cache-dir` on the command line, or `FHIRLINT_CACHE_DIR`, is your own choice and is used as given, so point it outside the checkout.

### Validating only what changed

On a large repository, validating every resource on every pull request costs minutes to confirm that three files are still fine. `--since` scopes the run to what changed against a git ref:

```bash
fhirlint validate ./fhir/ --since main
fhirlint validate ./fhir/ --since HEAD~1
```

Three sets are validated:

- **Committed changes**, compared as `main...HEAD` — changes since the merge base, i.e. what the branch contributes, not whatever else landed on `main` meanwhile.
- **Uncommitted changes** against `HEAD`, staged or not, so a local run sees the file you are editing.
- **Untracked files** that are not gitignored, so a resource you just created is not silently skipped.

Deleted files are skipped; a renamed file is validated at its new path. Files that did not change are reported and then left alone:

```text
Skipped 214 unchanged file(s) (--since main)
```

An empty change set exits 0 and says so rather than passing silently:

```text
No changed files to validate (--since main)
```

`--since` needs a git working tree, so it cannot be combined with `--url`, stdin input or `--watch`. An unresolvable ref, or running outside a repository, is an error — quietly validating everything instead would hide a broken CI configuration.

**With `--check-references`,** the unchanged files are still indexed for reference resolution even though they are not validated. Reference integrity is a property of the whole dataset, so a changed resource pointing at an untouched one resolves normally instead of being reported as a dangling reference.

This composes with `--cache` rather than replacing it: `--since` needs no state at all and works on a cold runner, while the result cache also helps for repeated local runs and for content that changed and changed back.

### Example GitHub Actions workflow

```yaml
- name: Validate FHIR resources
  run: |
    fhirlint validate ./fhir/ \
      --format json --output fhir-report.json \
      --tx-cache .fhirlint-tx-cache/ \
      --cache --cache-dir "${{ runner.temp }}/fhirlint-cache" \
      --fail-on error

- uses: actions/upload-artifact@v4
  if: always()
  with:
    name: fhir-validation-report
    path: fhir-report.json
```
