FROM golang:1.26.5-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/bom ./cmd/bom
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/bom-action ./cmd/bom-action

FROM alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce

RUN apk add --no-cache bash ca-certificates curl git openssl tar gzip
RUN curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

COPY --from=builder /out/bom /usr/local/bin/bom
COPY --from=builder /out/bom-action /usr/local/bin/bom-action

ENTRYPOINT ["/usr/local/bin/bom-action"]
