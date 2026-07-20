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

## Release integrity

Every published release is signed and carries provenance, so you can establish that an artifact came from this repository's release workflow rather than from someone else. The commands are in the README, under [pre-built binary](README.md#pre-built-binary-recommended) and [verifying the image](README.md#verifying-the-image).

What is covered:

| Artifact | Provenance attestation | SBOM | SBOM attestation |
|---|---|---|---|
| Release archives (`.tar.gz`, `.zip`) | yes | yes, alongside each archive | no — see below |
| `checksums.txt` | yes | n/a | n/a |
| Archive SBOMs (`*.sbom.json`) | yes | n/a | n/a |
| Container image | yes, plus a cosign signature | yes | yes |

### Why the archive SBOMs are not SBOM-attested

An SBOM attestation binds one SBOM to one subject artifact. With five release archives, each with its own SBOM, that is five separate attestations produced at release time, which needs the artifacts carried into a matrix job purely to make the claim.

Instead the archives, their SBOMs and `checksums.txt` all carry provenance attestations, and the attested `checksums.txt` lists the archives and their SBOMs. An SBOM can therefore be tied back to its archive through an attested file, without per-archive SBOM attestations.

The container image does carry a real SBOM attestation: it is a single subject, so there is no equivalent cost.

This is a deliberate trade-off, not an oversight. If the archive set grows or the verification story needs to be provable without `checksums.txt`, it is worth revisiting.
