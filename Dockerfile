FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /fhirlint .

FROM eclipse-temurin:21-jre-alpine
COPY --from=builder /fhirlint /usr/local/bin/fhirlint
ENTRYPOINT ["fhirlint"]
