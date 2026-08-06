# helm-bom

`helm-bom` templates a Helm chart, extracts OCI image references from supported Kubernetes workload primitives, and emits either SPDX JSON or the internal `csbom` format in JSON or YAML.

## Usage

```bash
go run ./cmd/helm-bom \
  ./chart \
  --values values.yaml \
  --set image.tag=1.2.3
```

The CLI is built with `cobra`, so `--help` and `--version` are handled through the standard Cobra command surface.

If `--release-name` is omitted, both the Helm release name and the SPDX document name default to the chart `name` declared in `Chart.yaml`.

## Development

Use the documented `Makefile` targets for the common workflows:

```bash
make help
make build
make test
make test-e2e
make fmt
make lint
```

## Output

Supported output formats:

- `spdx-json` (default)
- `csbom-json`
- `csbom-yaml`
- `spdx`
- `csbom` (alias of `csbom-yaml`)

### SPDX JSON

The default output format is `spdx-json`:

```json
{
  "spdxVersion": "SPDX-2.3",
  "dataLicense": "CC0-1.0",
  "SPDXID": "SPDXRef-DOCUMENT",
  "name": "helm-bom chart",
  "documentNamespace": "https://codesphere-cloud.github.io/helm-bom/spdx/...",
  "creationInfo": {
    "created": "2026-07-29T00:00:00Z",
    "creators": [
      "Tool: helm-bom-dev"
    ]
  },
  "packages": [
    {
      "name": "ghcr.io/acme/api",
      "SPDXID": "SPDXRef-Package-001",
      "versionInfo": "1.2.3",
      "downloadLocation": "NOASSERTION",
      "filesAnalyzed": false,
      "externalRefs": [
        {
          "referenceCategory": "PACKAGE-MANAGER",
          "referenceType": "purl",
          "referenceLocator": "pkg:oci/ghcr.io/acme/api@1.2.3"
        }
      ],
      "annotations": [
        {
          "annotationType": "OTHER",
          "annotator": "Tool: helm-bom-dev",
          "comment": "Manifest evidence: Deployment/api spec.containers[0]"
        }
      ]
    }
  ],
  "relationships": [
    {
      "spdxElementId": "SPDXRef-DOCUMENT",
      "relationshipType": "DESCRIBES",
      "relatedSpdxElement": "SPDXRef-Package-001"
    }
  ]
}
```

### CSBOM JSON

The `csbom-json` export uses the existing [internal/csbom/bom.go](/Users/schrodit/dev/cs/helm-bom/internal/csbom/bom.go) shape directly:

```json
{
  "components": {
    "chart": {
      "containerImages": {
        "ghcr.io/acme/api": "ghcr.io/acme/api:1.2.3"
      }
    }
  }
}
```

### CSBOM YAML

The `csbom` and `csbom-yaml` exports use the same shape encoded as YAML:

```yaml
components:
  chart:
    containerImages:
      ghcr.io/acme/api: ghcr.io/acme/api:1.2.3
```

Supported Kubernetes primitives are handled explicitly rather than via generic YAML walking:

- `Pod`
- `PodTemplate`
- `ReplicationController`
- `Deployment`
- `ReplicaSet`
- `DaemonSet`
- `StatefulSet`
- `Job`
- `CronJob`
- `List` containing any of the above

Image reference parsing uses Docker’s upstream `github.com/distribution/reference` package.

SPDX document generation and JSON serialization use the upstream `github.com/spdx/tools-golang` library rather than a local SPDX struct implementation. The `csbom` exports use the existing `internal/csbom` package directly.

The formatter remains isolated behind `--format`, so additional SPDX serializations can be added later without changing Helm rendering or Kubernetes extraction.
