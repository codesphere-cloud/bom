FROM golang:1.26.5-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/bom ./cmd/bom
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/bom-action ./cmd/bom-action

FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

RUN apk add --no-cache bash ca-certificates curl git openssl tar gzip
RUN curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

COPY --from=builder /out/bom /usr/local/bin/bom
COPY --from=builder /out/bom-action /usr/local/bin/bom-action

ENTRYPOINT ["/usr/local/bin/bom-action"]
