# Recorded validator output

Real output of the HL7 FHIR Validator, one directory per release, for the parser's unit tests (#715). `TestRecordedValidatorOutput` reads every recording on each `go test`, puts it through `readRun`, the same code a live run uses, and compares the result with the case's `expected.json`. A release that changes its OperationOutcome, its console lines or its message ids shows up as soon as it is recorded, not first in the integration suite or the canary.

## Layout

```text
inputs/<case>/            what the case validates, as the recorder's working directory
  in/                     the resources
  ig/                     a local IG, where the case needs one
<version>/
  Messages.properties.gz  the JAR's English message bundle, for the message id inference (#449)
  <case>/
    run.json              the paths passed to the JAR, and whether it exited non-zero
    stdout.txt            what the JAR logged
    stderr.txt
    output.json           the report it wrote; missing when it wrote none
    expected.json         what fhirlint makes of it
```

The cases and their options are listed in `recordedCases` in `internal/validator/recorded_output_cases_test.go`. A case added there fails the test for every version until it is recorded.

## Re-recording

```bash
scripts/record-validator-output.sh 7.0.6
```

Run it when an integration pin in `.github/workflows/integration-test.yml` moves, and record the new version next to the others. It needs Java 17 or later and the network: the JAR, the packages and, for `tx-no-id`, the live terminology servers. It is a manual step, not a CI job.

The script records into `<version>/`, rewrites each `expected.json` from the parser, and runs the test again. Review the diff before committing it:

- A changed `stdout.txt` or `output.json` is what the validator does differently.
- A changed `expected.json` is what fhirlint now reports differently for it.
- A check that fails names what the case exists to show, for example a crash that is no longer pinned on the file. That is a finding for its own issue. Don't adapt the check to make the recording pass.

## Normalisation

The recorder takes out what belongs to the machine and the moment rather than to the validator:

| In the output | Becomes |
|---|---|
| the working directory | `/work` |
| the home directory | `/home/user` |
| fhirlint's temp report file | `out.json` |
| timings | `00:00.100` |
| the JAR's age | `(1 days old)` |
| the Java line and the available memory | fixed placeholders |

Each case also gets a fresh terminology cache, so no earlier run answers for the server. The live answers in `tx-no-id` can still change between two recordings of the same version, because the servers change. Such a diff is about the server, not the validator.
