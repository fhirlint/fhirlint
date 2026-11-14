# Verifying a release

Release archives, `checksums.txt`, the SBOMs and the container image are signed through Sigstore and carry SLSA provenance. This page shows how to check them. How to rebuild a release and compare it byte for byte is in [SECURITY.md](../SECURITY.md#rebuilding-a-release).

## Release archives

Release archives carry a build-provenance attestation, so you can check an archive really came from this repo's release workflow before unpacking it. Releases after 1.17.0 are built in the reusable workflow `build-release.yml`, which makes their provenance SLSA Build Level 3 ([#601](https://github.com/fhirlint/fhirlint/issues/601)). `--signer-workflow` checks that this workflow signed it:

```bash
gh attestation verify fhirlint_*_linux_amd64.tar.gz --repo fhirlint/fhirlint \
  --signer-workflow fhirlint/fhirlint/.github/workflows/build-release.yml
```

For 1.17.0 and earlier, whose provenance `release.yml` signed (Build Level 2), drop `--signer-workflow`.

`checksums.txt` is attested too, so the checksum route is trustworthy end to end — verify the file itself, then use it to check anything it lists:

```bash
gh attestation verify checksums.txt --repo fhirlint/fhirlint
sha256sum --check --ignore-missing checksums.txt
```

It covers the archives and their SBOMs, so an attested `checksums.txt` makes those tamper-evident without each needing its own attestation.

Releases after 1.17.0 also carry the attestation itself, as `fhirlint_<version>.sigstore.json` (the Sigstore bundle) and `fhirlint_<version>.intoto.jsonl` (its DSSE envelope). One bundle covers every archive, SBOM and `checksums.txt`, so you can verify against the file instead of the attestations API, for example from a mirror:

```bash
gh attestation verify fhirlint_*_linux_amd64.tar.gz --repo fhirlint/fhirlint \
  --bundle fhirlint_*.sigstore.json
```

Each archive also ships an SPDX SBOM next to it (`<archive>.sbom.json`) listing the Go modules it was built from. The SBOM files carry a provenance attestation of their own, so you can confirm one came from the release build before trusting its contents:

```bash
gh release download --repo fhirlint/fhirlint --pattern "*linux_amd64.tar.gz.sbom.json"
gh attestation verify fhirlint_*_linux_amd64.tar.gz.sbom.json --repo fhirlint/fhirlint
```

Note what that does and does not say. It establishes that the release build produced this SBOM file — not, cryptographically, that this SBOM describes that particular archive. The link between the two is the filename convention plus `checksums.txt`, which is itself attested and lists both.

A true SBOM attestation binds one SBOM to one subject artifact, so covering five archives means five separate attestations at release time. Given that the archives, their SBOMs and the checksums file are all provenance-attested and mutually covered by the attested checksums, that extra release-pipeline machinery buys little here. The [container image](#verifying-the-image) does carry a real SBOM attestation, because there it is a single subject and costs nothing.

Attestations and SBOMs apply from **v1.5.0** onward; v1.4.0 and earlier have neither.

## Verifying the image

From **v1.5.0** onward, images are signed with [cosign](https://docs.sigstore.dev/) (keyless, via Sigstore) and carry a build-provenance attestation. Images after 1.17.0 are built and signed in the reusable workflow `build-image.yml` (SLSA Build Level 3, [#601](https://github.com/fhirlint/fhirlint/issues/601)); earlier ones in `docker.yml`. The SBOM attestation applies from **v1.6.0** onward — the v1.5.0 image is signed and provenance-attested but has no SBOM attestation ([#266](https://github.com/fhirlint/fhirlint/issues/266)).

Check the signature:

```bash
cosign verify ghcr.io/fhirlint/fhirlint:<version> \
  --certificate-identity-regexp '^https://github\.com/fhirlint/fhirlint/\.github/workflows/(build-image|docker)\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

The regex accepts both workflows. To require Build Level 3, accept only `build-image`. Check that the image was built by this repo's build workflow, and inspect its SBOM:

```bash
gh attestation verify oci://ghcr.io/fhirlint/fhirlint:<version> --repo fhirlint/fhirlint \
  --signer-workflow fhirlint/fhirlint/.github/workflows/build-image.yml
cosign download attestation ghcr.io/fhirlint/fhirlint:<version>
```

Signature and attestations are bound to the image **digest**, so verifying a tag checks whatever that tag currently points at. Pin the digest (`ghcr.io/fhirlint/fhirlint@sha256:...`) when you need the stronger guarantee.

Images published before v1.5.0 are unsigned.
