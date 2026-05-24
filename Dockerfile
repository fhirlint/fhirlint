FROM golang:1.26.3-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /fhirlint .

# Download the validator JAR at image build time so containers start immediately
# without a ~250 MB network fetch on first use.
FROM eclipse-temurin:25-jre-alpine AS jar-downloader
RUN apk add --no-cache curl && \
    mkdir -p /root/.fhirlint && \
    curl -sL --retry 3 \
      "https://github.com/hapifhir/org.hl7.fhir.core/releases/latest/download/validator_cli.jar" \
      -o /root/.fhirlint/validator_cli.jar && \
    # Write version from the redirect URL so `fhirlint version` shows it immediately.
    curl -sIL --retry 3 \
      "https://github.com/hapifhir/org.hl7.fhir.core/releases/latest/download/validator_cli.jar" \
      | grep -i '^location:' | tail -1 | tr -d '\r' \
      | sed 's|.*/releases/download/\([^/]*\)/.*|\1|' \
      > /root/.fhirlint/validator_version.txt

FROM eclipse-temurin:25-jre-alpine
COPY --from=builder /fhirlint /usr/local/bin/fhirlint
COPY --from=jar-downloader /root/.fhirlint /root/.fhirlint
ENTRYPOINT ["fhirlint"]
