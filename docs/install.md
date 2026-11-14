# Installing fhirlint

Every way to install fhirlint. All of them need Java 17+ for the validator, except the Docker image and the Homebrew cask, which bring a JRE. To check a download before you run it, see [Verifying a release](verifying-releases.md).

## Pre-built binary (recommended)

Download the latest binary for your platform from the [releases page](https://github.com/fhirlint/fhirlint/releases):

```bash
# Linux (amd64)
gh release download --repo fhirlint/fhirlint --pattern "*linux_amd64.tar.gz"
tar xzf fhirlint_*_linux_amd64.tar.gz && sudo mv fhirlint /usr/local/bin/

# macOS (Apple Silicon)
gh release download --repo fhirlint/fhirlint --pattern "*darwin_arm64.tar.gz"
tar xzf fhirlint_*_darwin_arm64.tar.gz && sudo mv fhirlint /usr/local/bin/

# macOS (Intel)
gh release download --repo fhirlint/fhirlint --pattern "*darwin_amd64.tar.gz"
tar xzf fhirlint_*_darwin_amd64.tar.gz && sudo mv fhirlint /usr/local/bin/
```

Windows: download the `.zip` from the [releases page](https://github.com/fhirlint/fhirlint/releases) and add `fhirlint.exe` to your `PATH`.

## Homebrew (macOS)

```bash
brew install fhirlint/tap/fhirlint
```

This installs a JRE (`openjdk@17`) as a dependency, so no separate Java setup is needed. Upgrade with `brew upgrade fhirlint`.

## go install

```bash
go install github.com/fhirlint/fhirlint@latest
```

## Docker

```bash
# Validate files in the current directory
docker run --rm -v $(pwd):/work ghcr.io/fhirlint/fhirlint validate /work/fhir/

# Pin to a specific version
docker run --rm -v $(pwd):/work ghcr.io/fhirlint/fhirlint:1.17.0 validate /work/fhir/
```

The image includes a JRE — no separate Java installation required.

### Running as non-root

The container runs as uid `65532`, not root. Reading mounted resources works as shown above, but **writing** into a mounted directory — a report via `--output`, or `--baseline` — fails unless the host directory is writable by that uid:

```text
Error: html report: open /work/report.html: permission denied
```

Run as your own user, with group `0`, when the container needs to write to the mount:

```bash
docker run --rm --user $(id -u):0 -v $(pwd):/work \
  ghcr.io/fhirlint/fhirlint validate /work/fhir/ --format html -o /work/report.html
```

Group `0` matters: the baked-in validator JAR and the cache directory are group-owned by `0`, so the validator still finds them under an arbitrary uid. A fully arbitrary `--user <uid>:<gid>` is not supported.
