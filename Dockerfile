FROM golang:1.26.5-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /fhirlint .

# Download the validator JAR at image build time so containers start immediately
# without a ~250 MB network fetch on first use.
FROM eclipse-temurin:25-jre-alpine AS jar-downloader
# pipefail so a failing command inside the pipe below (e.g. curl erroring, or
# grep matching no redirect header) fails the build rather than being masked by
# the exit status of the last stage. It does not catch the current parsing bug —
# grep and sed both exit 0 while sed's pattern fails to match, so the version
# file still ends up holding the raw Location header. That is tracked in #222.
SHELL ["/bin/ash", "-o", "pipefail", "-c"]
# curl is only used in this throwaway stage and never ships; pinning an exact
# apk version would break the build on every base image refresh for no benefit
# to the final image.
# hadolint ignore=DL3018
RUN apk add --no-cache curl && \
    mkdir -p /out && \
    curl -sL --retry 3 \
      "https://github.com/hapifhir/org.hl7.fhir.core/releases/latest/download/validator_cli.jar" \
      -o /out/validator_cli.jar && \
    # Write version from the redirect URL so `fhirlint version` shows it
    # immediately. The version only appears in the *intermediate* redirect
    # (.../releases/download/<version>/validator_cli.jar); the final hop goes to
    # a CDN host that carries no version, so the matching line must be picked by
    # content, not by position.
    #
    # Headers are buffered to a file first, on purpose: a consumer that exits
    # early (grep -m1, head -1) makes curl take a SIGPIPE, which under pipefail
    # fails the build with 141. tr consumes the whole stream, and awk then reads
    # a file rather than a pipe, so its early exit is harmless.
    curl -sIL --retry 3 \
      "https://github.com/hapifhir/org.hl7.fhir.core/releases/latest/download/validator_cli.jar" \
      | tr -d '\r' > /tmp/headers.txt && \
    awk 'match($0, "/releases/download/[^/]+/") { split(substr($0, RSTART, RLENGTH), a, "/"); print a[4]; exit }' \
      /tmp/headers.txt > /out/validator_version.txt && \
    # Fail the build rather than shipping an unparsed version file.
    grep -qE '^[0-9]+\.[0-9]+' /out/validator_version.txt

FROM eclipse-temurin:25-jre-alpine
# Pull in base-image package fixes that are published upstream but not yet in a
# temurin release, so the shipped image does not carry known HIGH/CRITICAL CVEs.
# The reproducibility cost is accepted deliberately: shipping known-vulnerable
# packages is the worse trade, and the Trivy gate is what makes this visible.
# hadolint ignore=DL3017
RUN apk --no-cache upgrade

# Run as a non-root user (CIS-DI-0001). fhirlint resolves its cache directory
# via os.UserHomeDir(), which reads $HOME, so HOME must be set explicitly — it
# is not inherited for a USER that the base image does not know about.
ENV HOME=/home/nonroot
RUN addgroup -g 65532 nonroot && \
    adduser -u 65532 -G nonroot -h "$HOME" -D nonroot

COPY --from=builder /fhirlint /usr/local/bin/fhirlint
COPY --from=jar-downloader --chown=65532:65532 /out "$HOME/.fhirlint"

# Group 0 with g=u keeps the cache dir writable when the image is run under an
# arbitrary uid (docker run --user, OpenShift), where 65532 no longer applies.
RUN chgrp -R 0 "$HOME" && chmod -R g=u "$HOME"

USER 65532:65532
ENTRYPOINT ["fhirlint"]
