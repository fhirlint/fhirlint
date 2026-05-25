# Security Policy

## Reporting a vulnerability in fhirlint

Please open a [GitHub Security Advisory](https://github.com/fhirlint/fhirlint/security/advisories/new) rather than a public issue. We aim to respond within 5 business days.

## Third-party dependency: HL7 FHIR Validator JAR

fhirlint downloads and runs the official **[HL7 FHIR Validator](https://github.com/hapifhir/org.hl7.fhir.core)** JAR from GitHub Releases. This JAR is a substantial Java application that bundles its own dependencies and is **not** covered by Go's module system or `govulncheck`.

| Property | Value |
|---|---|
| Upstream repo | [hapifhir/org.hl7.fhir.core](https://github.com/hapifhir/org.hl7.fhir.core) |
| License | Apache 2.0 |
| Download source | GitHub Releases (`latest`) |
| Cached at | `~/.fhirlint/validator_cli.jar` |

### Checking your installation

```bash
fhirlint audit
```

This checks whether your local JAR is outdated and queries the GitHub Security Advisory database, reporting only the advisories that **affect your installed version**. Add `--format json` for machine-readable output (used by automation and dashboards).

### Updating the JAR

```bash
fhirlint update
```

fhirlint always downloads the **latest** release of the JAR. There is no pinned version — updating always moves you to the most recent upstream release, which is the recommended mitigation for any JAR-level CVE.

### If a CVE is found in the JAR

1. Check whether the upstream project has released a fix: [hapifhir releases](https://github.com/hapifhir/org.hl7.fhir.core/releases)
2. Run `fhirlint update` to install the latest version
3. Run `fhirlint audit` to confirm the advisory is resolved
4. If no fix is available upstream, consider pausing use of fhirlint until one is released

### Monitoring

fhirlint checks for new JAR versions automatically (once per 24 hours) and notifies you after `fhirlint validate` or `fhirlint version`. The maintainers run a weekly automated check (`fhirlint audit`) against the **latest** JAR release and open a GitHub issue only when a published advisory actually affects that release — not merely because historical advisories exist. The issue is closed automatically once a fixed release lands.
