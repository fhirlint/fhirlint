# Roadmap

This is where fhirlint is heading over the next 12 months, until November 2027. It is a direction, not a promise: priorities change with what users report. Work is tracked in [issues](https://github.com/fhirlint/fhirlint/issues), and anything here without an issue gets one before it is started.

## Planned

**Validation**

- **Keep up with the HL7 validator.** fhirlint supports new releases soon after they appear. `upgrade-check` and the release notes name the findings a new release adds, removes or renames. `jar-monitor.yml` watches the latest release for advisories, and the signing key for its expiry.
- **FHIR R6 support**, once 6.0.0 is published, with the FHIR version table kept in one place (#306).

**German profiles**

- **Keep the built-in aliases current** (KBV, MII, ISiK, DiGA) as the packages publish new versions. `alias-monitor.yml` checks the alias table against the package registry every week.

**fhirlint's own checks**

- **Run every new or changed built-in lint rule and reference check against the real-world corpus** before a release ([docs/corpus-scan.md](docs/corpus-scan.md)).
- **Decide whether `profile-name-pascalcase` allows underscores**, as HL7's own naming invariant does (#594).

**Quality**

- **Keep statement coverage at 80 percent or above.** It reached 80.6 percent in #595, and the CI floor is 80.

## Not planned

fhirlint wraps the official HL7 validator and adds output, CI integration and its own checks around it. These are deliberately out of scope:

- **Implementing FHIR validation itself.** Validation stays with the HL7 validator, and gaps in it belong upstream.
- **Running a terminology server or a hosted validation service.**
- **A graphical interface in the CLI.** The browser UI is a separate project, [fhirlint/web](https://github.com/fhirlint/web).
- **Formats other than FHIR**, such as HL7 v2 or CDA.

Suggestions are welcome as [issues](https://github.com/fhirlint/fhirlint/issues/new).
