FROM golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/bom ./cmd/bom
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/bom-action ./cmd/bom-action

FROM alpine:3.24@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a

RUN apk add --no-cache bash ca-certificates curl git openssl tar gzip
RUN curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

COPY --from=builder /out/bom /usr/local/bin/bom
COPY --from=builder /out/bom-action /usr/local/bin/bom-action

ENTRYPOINT ["/usr/local/bin/bom-action"]
