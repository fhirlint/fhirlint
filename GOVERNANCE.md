# Governance

fhirlint has two maintainers. This document says who decides what, who can reach the project's sensitive resources, and how that changes when more people join.

## Roles

**Maintainer.** Decides what goes into fhirlint and when it is released. A maintainer:

- reviews and merges pull requests into `main`, and sets the `sensitive-paths-reviewed` label after reviewing a change to a [sensitive path](CONTRIBUTING.md#opening-a-pr);
- triages issues and answers reports of validation problems, false findings and bugs;
- handles vulnerability reports as described in [SECURITY.md](SECURITY.md), including advisories and fixes;
- cuts releases: the version tag, the release notes, the container image, the Homebrew cask, the Claude Code plugin and the GitLab CI/CD component, with the checks in [TESTING.md](TESTING.md#before-each-release);
- keeps the project's security settings in place: branch protection, required checks, two-factor authentication, signed releases.

**Contributor.** Anyone who opens an issue or a pull request. Contributors don't need any access; how to contribute is in [CONTRIBUTING.md](CONTRIBUTING.md).

Decisions are made by the maintainers, in the open: in issues and pull requests on GitHub.

## Members with access to sensitive resources

| Person | Access |
|---|---|
| Nicolai Ehrhardt ([@predictor2718](https://github.com/predictor2718)) | Owner of the GitHub organization `fhirlint`: repository admin for `fhirlint`, `homebrew-tap`, `web` and `example-fhir-medication-api`, branch protection, releases, GitHub Actions settings and secrets, the container image on `ghcr.io/fhirlint/fhirlint`, security advisories. Owner of the GitLab group `fhirlint`: the read-only mirror that hosts the CI/CD Catalog component. |
| [@luma434](https://github.com/luma434) | Owner of the GitHub organization `fhirlint`, with the same GitHub access as above. |

Nobody else has write access to any of these. The GitHub organization requires two-factor authentication for all members.

## Secrets and credentials

fhirlint is built so that there are as few secrets as possible:

- **Two stored secrets in CI.** The workflows use the `GITHUB_TOKEN` that GitHub issues per run, with the least permissions each job needs, and OIDC tokens for Sigstore; all of them expire when the run ends. The two stored secrets are:
  - `HOMEBREW_TAP_GITHUB_TOKEN`: write access to `fhirlint/homebrew-tap`, used only by the release job on a version tag, to publish the cask.
  - `GITLAB_MIRROR_TOKEN`: a GitLab token with `write_repository` scope for the mirror, used only by `gitlab-mirror.yml` on pushes to `main` and version tags.

  GitHub doesn't pass either secret to workflows of pull requests from forks.
- **No signing keys.** Releases and the container image are signed keylessly: GitHub Actions obtains a short-lived certificate from Sigstore for each run, so there is no key to hold, rotate or hand over. The PGP key in the repository is HL7's public key for checking the validator JAR, not a secret.
- **Personal tokens.** A maintainer's personal access tokens are created for a task with the narrowest scope and an expiry of at most 30 days, kept only on the maintainer's machine with owner-only file permissions, and revoked when the task is done. They are never put in a repository, an issue or a CI variable.
- **Accounts.** GitHub and GitLab are accessed with two-factor authentication; access is listed above.

A secret that CI needs is limited to what the job needs, given to that job only, and rotated at least once a year and whenever someone with access leaves. Any secret that may have leaked is revoked at once, and the leak is handled as a vulnerability under [SECURITY.md](SECURITY.md).

## Granting access

Write or admin access is given only to a person who has contributed to fhirlint over a period of time and whom the maintainers know to work carefully, and only the access the role needs. Every new member must have two-factor authentication on GitHub and GitLab; GitHub enforces it for the organization, on GitLab it is checked before access is given. Changes to this list are made in this file, in a pull request.
