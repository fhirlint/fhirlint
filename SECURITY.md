# Security Policy

## Reporting a vulnerability in fhirlint

Please open a [GitHub Security Advisory](https://github.com/fhirlint/fhirlint/security/advisories/new) rather than a public issue. We aim to respond within 5 business days.

## Supported versions

Fixes go into the latest release. There are no maintained release branches, so upgrading is the mitigation for anything reported here.

## Trust boundary

fhirlint is usually pointed at data it has no reason to trust — resources from an API, a partner, or a contributor's pull request — and it runs in CI where that data reaches it automatically.

**Trusted:** the fhirlint binary and its configuration (`fhirlint.yml`, CLI flags, suppression rules), and the validator JAR *after* its checksum has been verified.

**Untrusted:** the FHIR resources being validated, responses from a terminology server, anything fetched via `--url`, and the JAR download until it is verified.

The security-relevant question is whether untrusted input can make fhirlint do something other than report findings about it.

### In scope

- A crafted resource that makes fhirlint itself misbehave — crash in a way that hides findings, consume unbounded memory, read or write outside the paths it was given.
- A flaw in how the validator JAR is downloaded, verified or cached.
- A suppression, baseline or override rule that hides a finding it should not, or an exit code that reports success when findings should have failed the build. Silently under-reporting is the failure mode that matters most for a validator.
- Leaking resource contents somewhere they were not meant to go — a report file, a log, a terminology request.

### Not a vulnerability

- **Findings the HL7 validator misses.** fhirlint reports what the JAR produces; gaps in FHIR validation itself belong upstream.
- **`--validator-arg` passing unchecked arguments to the JAR.** It is a documented escape hatch and is explicitly not validated. Arguments fhirlint's own output parsing depends on are rejected, but anything else is the caller's responsibility.
- **Configuration doing what it says.** A suppression rule that hides a finding you did not intend to hide is a configuration mistake. `require-suppress-reason` and suppression `expires` dates exist to make those visible.
- **The container running as uid 65532** rather than an arbitrary uid you would prefer, or needing `--user $(id -u):0` to write into a mounted directory.

### Properties you can rely on

- **The JAR is checksum-verified before use** against the `.sha256` published with the upstream release. A mismatch deletes the download and fails with an explicit error — it is never used.
- **An unverified JAR is never silent.** Upstream does not always publish a checksum, and the request can fail, so verification being skipped is not fatal. But it prints a warning at download time, and `fhirlint version` marks the JAR `(checksum NOT verified)` for as long as it stays cached. You can always tell which you have.
- **`fhirlint update` and `fhirlint audit`** report advisories that affect the version you actually have, not the whole history of the project.
- **Exit codes follow findings.** `--fail-on` controls the threshold; a run that finds errors at or above it exits non-zero.
- **Your input files are not modified.** Preprocessing (`--extract`, `--ignore`, `--bundle-entries`) works on temp copies.
- **The published container image runs as a non-root user** and carries no known HIGH or CRITICAL vulnerabilities at build time — CI fails the build otherwise.

### Repository controls

Secret scanning and Dependabot security updates are enabled on this repository. Every pull request runs unit tests, `golangci-lint`, `govulncheck`, `zizmor` (GitHub Actions auditing), and Docker gates (hadolint, Trivy, dockle). Releases and image publishes are gated on those runs being green.

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

`fhirlint version` additionally reports whether the cached JAR passed checksum verification:

```
validator: 6.9.12 (checksum verified)  (https://github.com/hapifhir/org.hl7.fhir.core/releases)
```

`(checksum NOT verified)` means the JAR is in use but its checksum could not be obtained — re-run `fhirlint update` on a working connection. No marker at all means the JAR predates this being recorded.

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

Signing and provenance attestation apply **from v1.5.0 onward**. v1.4.0 and earlier are unsigned and carry no attestation; there is no way to verify them retroactively.

**v1.5.0 is missing the image SBOM attestation.** The release workflow failed on that step (#266) after the image had been pushed, signed and provenance-attested, so v1.5.0 has everything in the table below except the image SBOM attestation. Re-cutting a published tag to add it would have been worse than the gap; it applies from the next release.

For releases that have it, the verification commands are in the README, under [pre-built binary](README.md#pre-built-binary-recommended) and [verifying the image](README.md#verifying-the-image).

What is covered:

| Artifact | Provenance attestation | SBOM | SBOM attestation |
|---|---|---|---|
| Release archives (`.tar.gz`, `.zip`) | yes | yes, alongside each archive | no — see below |
| `checksums.txt` | yes | n/a | n/a |
| Archive SBOMs (`*.sbom.json`) | yes | n/a | n/a |
| Container image | yes, plus a cosign signature | yes | yes (from the release after v1.5.0) |

### Why the archive SBOMs are not SBOM-attested

An SBOM attestation binds one SBOM to one subject artifact. With five release archives, each with its own SBOM, that is five separate attestations produced at release time, which needs the artifacts carried into a matrix job purely to make the claim.

Instead the archives, their SBOMs and `checksums.txt` all carry provenance attestations, and the attested `checksums.txt` lists the archives and their SBOMs. An SBOM can therefore be tied back to its archive through an attested file, without per-archive SBOM attestations.

The container image does carry a real SBOM attestation: it is a single subject, so there is no equivalent cost.

This is a deliberate trade-off, not an oversight. If the archive set grows or the verification story needs to be provable without `checksums.txt`, it is worth revisiting.
