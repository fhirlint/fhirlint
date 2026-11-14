# Security Policy

## Reporting a vulnerability in fhirlint

Please open a [GitHub Security Advisory](https://github.com/fhirlint/fhirlint/security/advisories/new) rather than a public issue. We aim to respond within 5 business days.

A confirmed vulnerability is published as a [GitHub Security Advisory](https://github.com/fhirlint/fhirlint/security/advisories) of this repository, naming the release that fixes it.

## Supported versions

Fixes go into the latest release. There are no maintained release branches, so upgrading is the mitigation for anything reported here.

A release stops receiving security fixes as soon as a newer release is published. There is no longer support period for older releases.

## Vulnerabilities that don't affect fhirlint

Scanners sometimes report a vulnerability that fhirlint doesn't reach: in a dependency, the Go standard library, the base image or the bundled validator JAR. When such a report reaches users, for example through a CVE in a scanner's output for the container image, we publish an [OpenVEX](https://openvex.dev) statement saying that fhirlint is `not_affected`, with the justification (such as `vulnerable_code_not_in_execute_path`). The statement goes into `vex/fhirlint.openvex.json` in this repository and is attached to the next release, so that scanners such as Trivy can read it (`trivy image --vex …`). No release has needed one so far, so the file doesn't exist yet. A vulnerability that does affect fhirlint is fixed in a release instead.

## Trust boundary

fhirlint is usually pointed at data it has no reason to trust — resources from an API, a partner, or a contributor's pull request — and it runs in CI where that data reaches it automatically.

**Trusted:** the fhirlint binary and its configuration (`fhirlint.yml`, CLI flags, suppression rules), and the validator JAR *after* it has been verified. When the checked directory is someone else's, such as a pull request from a fork, its `fhirlint.yml` and `.fhirlintignore` are theirs too: run with `--no-repo-config` and pass your own `--config`. See [Limits for untrusted pull requests](docs/ci.md#limits-for-untrusted-pull-requests).

**Untrusted:** the FHIR resources being validated, responses from a terminology server, anything fetched via `--url`, and the JAR download until it is verified.

The security-relevant question is whether untrusted input can make fhirlint do something other than report findings about it.

[ARCHITECTURE.md](ARCHITECTURE.md#threat-model) has the threat model and the assurance case built on this boundary.

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

- **The JAR is verified before use**, strongest check first:
  1. **The detached PGP signature** (`validator_cli.jar.asc`) against the HL7 release signing key, which fhirlint pins in-tree. This is the only check that means anything if the release channel itself is compromised, because it binds the JAR to a maintainer's key rather than to something GitHub also serves.
  2. **The SHA-256 digest GitHub records for the release asset**, when no signature verifies. This travels the same channel as the download, so it proves the transfer was not corrupted or truncated — not that the release is genuine. It is recorded as the weaker method it is.

  A signature that is present and does not verify, or a digest that does not match, discards the download and fails. It is never used, and a matching digest never launders a failed signature. The error says what to do about it: keep the JAR you already have, or pin a release that does verify. It deliberately does not point at `--jar`, which would install the rejected file with no check at all.

  Note this fires for an upstream mistake as readily as for an attack. A release whose `.asc` was uploaded before the JAR it is meant to sign produces exactly the same result, and fhirlint cannot tell the two apart, so it refuses either way.
- **An unverified JAR is never silent.** Releases before 6.6.0 carry neither a signature nor a digest, and either request can fail, so being unable to verify is not fatal. But it prints a warning at download time, and `fhirlint version` marks the JAR `(NOT verified)` for as long as it stays cached. You can always tell which you have, and which check produced it.
- **`fhirlint update` and `fhirlint audit`** report advisories that affect the version you actually have, not the whole history of the project.
- **A pinned validator is still watched.** Advisories are usually published long after the release they describe, so pinning for reproducibility would otherwise mean never hearing about one. When a published advisory affects your installed version, every run says so and points at `fhirlint audit`. That line is shown in CI and in redirected logs, where the update notices are suppressed — a pipeline cannot upgrade itself, but the person reading the log can act.
- **Exit codes follow findings.** `--fail-on` controls the threshold; a run that finds errors at or above it exits non-zero.
- **Untrusted responses are bounded.** A `--url` response is streamed to a temporary file, not held in memory, and one over 512 MiB fails the run (#586). While recording terminology traffic, a terminology response over 128 MiB is refused and not recorded, so a later replay reports it as a miss (#588).
- **Your input files are not modified.** Preprocessing (`--extract`, `--ignore`, `--bundle-entries`) works on temp copies.
- **The result cache does not take entries from the repository.** A `--cache` directory inside the working directory or an input's directory is refused when it comes from `fhirlint.yml` or is the default, and so is one owned by another user. Entries are never read or written through a symbolic link. A `--cache-dir` on the command line or `FHIRLINT_CACHE_DIR` is used as given.
- **A directory does not reach beyond itself.** A symbolic link inside a directory you validate (or pass to `coverage` or `qualify`) that leads outside it is skipped with a warning that names the link, not its target. A report quotes what it reads, through parse errors and `--show-source`, so such a link could otherwise copy a file from elsewhere on the runner into the job log. A discovered `fhirlint.yml` or `.fhirlintignore` that links out of its directory is refused in the same way. A path you name yourself, as an argument or with `--config`, is used as given.
- **The published container image runs as a non-root user** and carries no known HIGH or CRITICAL vulnerabilities at build time — CI fails the build otherwise.

### Repository controls

Secret scanning and Dependabot security updates are enabled on this repository. Every pull request runs unit tests, `golangci-lint`, `govulncheck`, `zizmor` (GitHub Actions auditing), `cfgaudit` (CLAUDE.md, the Claude Code plugin, its hooks and skills, with a config the pull request cannot change), and Docker gates (hadolint, Trivy, dockle). Releases and image publishes are gated on those runs being green.

OpenSSF Scorecard runs weekly and on every push to `main`. [docs/osps-baseline.md](docs/osps-baseline.md) maps the [OSPS Baseline](https://baseline.openssf.org/) controls to the evidence in this repository and lists what is still open.

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

`fhirlint version` additionally reports how the cached JAR was verified:

```text
validator: 6.10.2 (verified: PGP signature)  (https://github.com/hapifhir/org.hl7.fhir.core/releases)
```

| Marker | Meaning |
|---|---|
| `(verified: PGP signature)` | Checked against the pinned HL7 release signing key. The strong case. |
| `(verified: GitHub release digest)` | No usable signature; the JAR matches the SHA-256 GitHub records for the asset. Rules out a corrupted transfer, not a compromised release. |
| `(verified: an earlier fhirlint)` | Cached by a version that recorded only that it had verified, not how. |
| `(NOT verified)` | In use, but neither check could be made — re-run `fhirlint update` on a working connection. |
| *no marker* | The JAR predates this being recorded. |

### The pinned signing key

fhirlint embeds the HL7 release signing key rather than fetching it at verification time: a key fetched over the same channel as the JAR proves nothing about the JAR. Pinning also makes a key rotation a reviewable commit instead of a silent change.

```text
85D1 C17C F115 2107 B272  386C 8FDF A682 8139 9B5D
David Otasek <dotasek.dev@gmail.com>
```

The same key signs the Maven Central releases of `ca.uhn.hapi.fhir:org.hl7.fhir.validation`, an upload path with separate credentials, which is how it was corroborated beyond the keyserver copy.

If upstream rotates the key, signatures stop matching the pin. fhirlint treats that as **unverified, not as tampering** — it cannot tell a rotation from a substitution, and refusing to run would turn an upstream change into an outage for everyone. The warning names the expected fingerprint so you can see what happened, and the digest check still runs. The pin's own expiry is checked by CI ahead of time and asserted by the test suite.

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

Releases after v1.17.0 also carry the archives' provenance as release assets, `fhirlint_<version>.sigstore.json` and `fhirlint_<version>.intoto.jsonl`, for verifying without the attestations API.

**SLSA Build Level.** From the release after v1.17.0 on, the archives and the image are built, attested and signed in the reusable workflows `build-release.yml` and `build-image.yml`. They, not the workflow that the tag push triggers, are the signers of the provenance. That isolation is what GitHub documents as [SLSA v1 Build Level 3](https://docs.github.com/actions/security-guides/using-artifact-attestations-and-reusable-workflows-to-achieve-slsa-v1-build-level-3) for artifact attestations (#601), and `gh attestation verify --signer-workflow` checks it. v1.17.0 and earlier were attested by `release.yml` and `docker.yml` directly, which is Build Level 2.

### Rebuilding a release

The release archives are reproducible from the release after v1.17.0 on (#549): the binaries are built with `-trimpath`, without a build ID and with the commit time, and the archive entries carry a fixed owner and the commit time. Two builds of the same tag give the same bytes, which `.github/workflows/reproducible-build.yml` checks on every push to `main`. To check a release yourself, with the Go version from its `go.mod` and the GoReleaser version pinned in `.github/workflows/release.yml` (`GORELEASER_VERSION`):

```bash
git checkout vX.Y.Z
HOMEBREW_TAP_GITHUB_TOKEN=unused goreleaser release --clean --skip=publish,sbom
diff <(grep -E '\.(tar\.gz|zip)$' dist/checksums.txt) \
     <(gh release download vX.Y.Z --repo fhirlint/fhirlint -p checksums.txt -O - | grep -E '\.(tar\.gz|zip)$')
```

Before a release is published, each archive is unpacked and run on its platform (Linux amd64/arm64, macOS arm64, Windows amd64): it must report the tag as its version and run `explain` and `config check`, and on Linux validate a fixture with a known error. The release job then checks that the archives it published are the ones that were tested.

### Why the archive SBOMs are not SBOM-attested

An SBOM attestation binds one SBOM to one subject artifact. With five release archives, each with its own SBOM, that is five separate attestations produced at release time, which needs the artifacts carried into a matrix job purely to make the claim.

Instead the archives, their SBOMs and `checksums.txt` all carry provenance attestations, and the attested `checksums.txt` lists the archives and their SBOMs. An SBOM can therefore be tied back to its archive through an attested file, without per-archive SBOM attestations.

The container image does carry a real SBOM attestation: it is a single subject, so there is no equivalent cost.

This is a deliberate trade-off, not an oversight. If the archive set grows or the verification story needs to be provable without `checksums.txt`, it is worth revisiting.
