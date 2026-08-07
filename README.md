# helm-bom

`helm-bom` templates a Helm chart, extracts OCI image references from supported Kubernetes workload primitives, emits either SPDX JSON or the internal `csbom` format in JSON or YAML, and can validate a generated BOM against upstream image registries.

If a chart contains a `.bomrc.yml` or `.bomrc.yaml` file, `helm-bom` also evaluates any configured `additionalImages` entries against the rendered manifest and merges those image references into the output.

## Usage

```bash
go run ./cmd/helm-bom \
  ./chart \
  --values values.yaml \
  --set image.tag=1.2.3
```

Explicit subcommands are also available:

```bash
go run ./cmd/helm-bom generate ./chart --output bom.json
go run ./cmd/helm-bom registry login ghcr.io -u "$USER" --password-stdin
go run ./cmd/helm-bom check bom.json
```

The CLI is built with `cobra`, so `--help` and `--version` are handled through the standard Cobra command surface.

The top-level `helm-bom ./chart ...` invocation remains supported as a compatibility alias for `helm-bom generate ./chart ...`.

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

## `.bomrc.yaml` / `.bomrc.yml`

Per-chart extra image discovery can be configured with an optional `.bomrc.yaml` or `.bomrc.yml` file in the chart root:

```yaml
dummyValues:
  image:
    repository: ghcr.io/acme/api
    tag: latest

additionalImages:
  - resource:
      apiVersion: v1
      kind: ConfigMap
      name: extra-images
    key: metrics
    image: .data.sidecars[] | select(.name == "metrics") | .image

imageKeyMappings:
  ghcr.io/acme/api: api
  docker.io/library/busybox: busybox
```

`dummyValues` is optional. When present, `helm-bom` writes it to a temporary Helm values file and passes it before any CLI-supplied `--values` files, so explicit user inputs still override these placeholders.

Each `additionalImages` entry:

- selects one rendered Kubernetes resource by `apiVersion`, `kind`, and `metadata.name`
- may optionally set `key` to override the BOM key used for that configured image
- evaluates the `image` field as a yq-style selector against that resource
- must resolve to exactly one string image reference

`imageKeyMappings` is optional. It remaps the BOM key used for auto-discovered image repositories in `csbom-json` and `csbom-yaml` output. Mapping keys are exact auto-discovered repository names, and mapping values are the keys that should be written into the BOM.

Image reference parsing uses Docker’s upstream `github.com/distribution/reference` package.

SPDX document generation and JSON serialization use the upstream `github.com/spdx/tools-golang` library rather than a local SPDX struct implementation. The `csbom` exports use the existing `internal/csbom` package directly.

The formatter remains isolated behind `--format`, so additional SPDX serializations can be added later without changing Helm rendering or Kubernetes extraction.

## Validation

`helm-bom check <bom>` reads either:

- `spdx-json` output generated by `helm-bom`
- `csbom-json`
- `csbom-yaml`
- `csbom`

For each image reference found in the BOM, `check` validates that the upstream registry serves a manifest for that reference using `github.com/google/go-containerregistry/pkg/crane.Get`, which delegates to `remote.Get`.

If a registry requires authentication first, log in with:

```bash
printf '%s\n' "$TOKEN" | go run ./cmd/helm-bom registry login ghcr.io -u "$USER" --password-stdin
```

Credentials are stored in the Docker config used by `crane` and the Docker CLI, honoring `DOCKER_CONFIG` when it is set.

## GitHub Action

This repository also ships a container-based GitHub Action defined by [action.yml](/Users/schrodit/dev/cs/helm-bom/action.yml) and built from [Dockerfile](/Users/schrodit/dev/cs/helm-bom/Dockerfile). The published Action pulls `ghcr.io/codesphere-cloud/helm-bom:latest`.

The Action supports two modes:

- `generate` to render charts and write BOMs
- `check` to validate existing BOM files against upstream registries

### Generate BOMs In CI

```yaml
jobs:
  bom:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: codesphere-cloud/helm-bom@main
        with:
          mode: generate
          paths: |
            charts/*
          changed-only: true
```

When `changed-only: true` is set in `generate` mode, the Action only runs charts whose directories contain files changed by the current push or pull request. Each generated BOM is written into the matching chart directory as `bom.json` or `bom.yaml`, depending on `format`. If `paths` is omitted, the Action discovers charts from the repository root automatically.

### Check BOMs In CI

```yaml
jobs:
  validate-boms:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: codesphere-cloud/helm-bom@main
        with:
          mode: check
          paths: |
            charts/*/bom.json
          changed-only: true
          registry-server: ghcr.io
          registry-username: ${{ github.actor }}
          registry-password: ${{ secrets.GITHUB_TOKEN }}
```

When `changed-only: true` is set in `check` mode, the Action only validates BOM files that were themselves changed by the current push or pull request. If `paths` is omitted, the Action discovers BOM files from the repository root automatically.

The Action writes these outputs:

- `changed-paths`
- `matched-paths`
- `processed-paths`
- `changed-output-paths`
- `changed-target-paths`
- `any-processed-changed`

## Image Release Flow

Two GitHub workflows manage the Action container image:

- [.github/workflows/action-image.yml](/Users/schrodit/dev/cs/helm-bom/.github/workflows/action-image.yml) builds the image on pull requests and pushes to `main` without publishing it.
- [.github/workflows/action-release.yml](/Users/schrodit/dev/cs/helm-bom/.github/workflows/action-release.yml) publishes the image to GHCR when a tag matching `v*` is pushed.

The tag release workflow publishes:

- the exact tag version
- major/minor semver aliases
- `latest`
