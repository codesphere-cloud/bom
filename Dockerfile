FROM golang:1.26.5-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/helm-bom ./cmd/helm-bom
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/helm-bom-action ./cmd/helm-bom-action

FROM alpine:3.22

RUN apk add --no-cache bash ca-certificates curl git openssl tar gzip
RUN curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

COPY --from=builder /out/helm-bom /usr/local/bin/helm-bom
COPY --from=builder /out/helm-bom-action /usr/local/bin/helm-bom-action

ENTRYPOINT ["/usr/local/bin/helm-bom-action"]
