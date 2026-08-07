# bom

`bom` renders Helm charts, extracts OCI image references from supported Kubernetes workload resources, writes BOM output in SPDX JSON or internal `csbom` / `csbom-v2` formats, and can validate referenced images against upstream registries.

This repository primarily ships GitHub Actions for CI usage, plus the CLI those Actions wrap.

## Start Here

Choose the section that matches how you use this repository:

1. [Users of the GitHub Action](#users-of-the-github-action)
2. [Users of the CLI](#users-of-the-cli)
3. [Developers](#developers)

## Users Of The GitHub Action

This repository ships three composite Actions:

- [generate/action.yml](/Users/schrodit/dev/cs/bom/generate/action.yml) generates BOM files for Helm charts
- [registry-login/action.yml](/Users/schrodit/dev/cs/bom/registry-login/action.yml) authenticates registry access for subsequent checks
- [check/action.yml](/Users/schrodit/dev/cs/bom/check/action.yml) validates BOM files against upstream registries

All Actions:

- run a prebuilt binary from `dist/linux-*` directly on the runner
- support Linux runners only

### Generate BOMs In CI

```yaml
jobs:
  bom:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
        with:
          fetch-depth: 0

      - uses: codesphere-cloud/bom/generate@main
        with:
          include-paths: |
            charts/*
          exclude-paths: |
            charts/legacy
          changed-only: true
          format: csbom-v2-json
```

Generate-specific inputs:

- `include-paths`: newline-separated chart selectors to include. Selectors can be exact paths, path prefixes, or glob patterns. If omitted, the Action auto-discovers chart directories from the repository root.
- `exclude-paths`: newline-separated chart selectors to exclude after discovery. Selectors can be exact paths, path prefixes, or glob patterns.
- `changed-only`: only process charts whose directories contain files changed in the current push or pull request
- `debug`: enable extra action logs and pass `--debug` through to the CLI
- `format`: output format. Default: `csbom-v2-json`
- `release-name`: optional Helm release name override
- `namespace`: Helm namespace passed to `helm template`. Default: `default`
- `validate-configured-image-exists`: fail when configured extra images do not resolve from the rendered manifest
- `fail-on-no-matches`: fail instead of succeeding when no chart paths remain after filtering

Generated files are written into each chart directory as:

- `bom.json` for JSON formats
- `bom.yaml` for YAML formats

When `changed-only: true` is set, the Action only processes charts whose directories contain files changed in the current push or pull request.

Generate outputs:

- `changed-paths`
- `matched-paths`
- `processed-paths`
- `changed-output-paths`
- `changed-target-paths`
- `any-processed-changed`

### How Generation Works

For each selected chart, the generate Action:

1. runs `helm template`
2. scans the rendered manifest for supported Kubernetes workload resources
3. extracts OCI image references from those workloads
4. writes the result as `bom.json` or `bom.yaml` in the chart directory, depending on the selected format

Supported Kubernetes workload primitives:

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

Image reference parsing uses `github.com/distribution/reference`.

If a chart root contains `.bomrc.yml` or `.bomrc.yaml`, `bom` loads it automatically during generation.

Example:

```yaml
bomGenerationValues:
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

`.bomrc.yml` behavior:

- `bomGenerationValues` provides default Helm values used only for BOM generation
- CLI- or Action-supplied chart inputs still override those defaults
- `additionalImages` lets a chart declare extra image references from rendered resources outside the standard workload image fields
- each `additionalImages` entry selects one rendered resource by `apiVersion`, `kind`, and `metadata.name`
- `image` is evaluated as a yq-style selector and must resolve to exactly one string image reference
- `key` optionally overrides the BOM key for that configured image
- `imageKeyMappings` remaps auto-discovered repository keys in `csbom-json` and `csbom-yaml`

### Output Formats

The generate Action accepts these formats:

- `csbom-v2-json` default Action format
- `csbom-v2-yaml`
- `csbom-v2`
- `csbom-json`
- `csbom-yaml`
- `csbom`
- `spdx-json`
- `spdx`

Example `csbom-v2-json` output:

```json
{
  "version": "2",
  "name": "chart",
  "containerImages": {
    "ghcr.io/acme/api": {
      "ref": "ghcr.io/acme/api:1.2.3",
      "sources": [
        "Deployment/api spec.containers[0]"
      ]
    }
  }
}
```

Example `csbom-yaml` output:

```yaml
components:
  chart:
    containerImages:
      ghcr.io/acme/api: ghcr.io/acme/api:1.2.3
```

Example `spdx-json` output:

```json
{
  "spdxVersion": "SPDX-2.3",
  "dataLicense": "CC0-1.0",
  "SPDXID": "SPDXRef-DOCUMENT",
  "name": "bom chart",
  "documentNamespace": "https://codesphere-cloud.github.io/bom/spdx/...",
  "creationInfo": {
    "created": "2026-07-29T00:00:00Z",
    "creators": [
      "Tool: bom-dev"
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
      ]
    }
  ]
}
```

### Check BOMs In CI

```yaml
jobs:
  validate-boms:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
        with:
          fetch-depth: 0

      - uses: codesphere-cloud/bom/registry-login@main
        with:
          registry-server: ghcr.io
          registry-username: ${{ github.actor }}
          registry-password: ${{ secrets.GITHUB_TOKEN }}

      - uses: codesphere-cloud/bom/check@main
        with:
          include-paths: |
            charts/*/bom.json
          exclude-paths: |
            charts/legacy/*
          changed-only: true
```

Registry-login inputs:

- `registry-server`: registry host to authenticate with, such as `ghcr.io`
- `registry-username`: registry username
- `registry-password`: registry password or access token; the Action passes it to the CLI through standard input rather than a command-line argument
- `debug`: enable extra login logs

Check-specific inputs:

- `include-paths`: newline-separated BOM selectors to include. Selectors can be exact paths, path prefixes, or glob patterns. Explicit selectors may target supported JSON or YAML BOM files. If omitted, the Action auto-discovers only files named `bom.json` from the repository root.
- `bom-paths`: alias for `include-paths`; `include-paths` takes precedence when both are set
- `exclude-paths`: newline-separated BOM selectors to exclude after discovery. Selectors can be exact paths, path prefixes, or glob patterns.
- `changed-only`: only validate BOM files that were changed in the current push or pull request
- `debug`: enable extra action logs and pass `--debug` through to the CLI
- `format`: stdout check-summary format, either `table` (default) or `yaml`; the GitHub step summary always uses a Markdown table
- `fail-on-no-matches`: fail instead of succeeding when no BOM paths remain after filtering

When `changed-only: true` is set, the Action only validates BOM files that were themselves changed in the current push or pull request.

Check outputs:

- `changed-boms`
- `matched-boms`
- `processed-boms`
- `failed-boms`

All selected BOMs are checked even when some fail. The Action exits unsuccessfully after processing every BOM and writes a step-summary table containing each BOM and its validation status or error.

## Users Of The CLI

Use the CLI for local debugging, reproducing GitHub Action behavior, or developing chart/BOM logic outside CI.

### Run The CLI

```bash
go run ./cmd/bom ./chart \
  --values values.yaml \
  --set image.tag=1.2.3
```

The top-level `bom ./chart ...` invocation is a compatibility alias for `bom generate ./chart ...`.

Explicit subcommands:

```bash
go run ./cmd/bom generate ./chart --output bom.json
go run ./cmd/bom check bom.json
printf '%s\n' "$TOKEN" | go run ./cmd/bom registry login ghcr.io -u "$USER" --password-stdin
```

The CLI is built with `cobra`, so `--help`, `--version`, `completion`, and subcommand help are available.

If `--release-name` is omitted, both the Helm release name and the SPDX document name default to the chart `name` from `Chart.yaml`.

### Useful Flags For Local Debugging

```bash
go run ./cmd/bom generate ./chart \
  --format csbom-v2-json \
  --namespace default \
  --values values.yaml \
  --set image.tag=1.2.3 \
  --helm-arg=--include-crds \
  --debug
```

Main generate flags:

- `--format`: `spdx-json`, `spdx`, `csbom-json`, `csbom-yaml`, `csbom`, `csbom-v2-json`, `csbom-v2-yaml`, or `csbom-v2`
- `--output`: write to a file instead of stdout
- `--values`: pass Helm values files
- `--set`: pass Helm `--set` overrides
- `--set-string`: pass Helm `--set-string` overrides
- `--helm-arg`: append raw extra arguments to `helm template`
- `--namespace`: namespace passed to `helm template`
- `--release-name`: Helm release name override
- `--validate-configured-image-exists`: fail when configured extra image selectors do not resolve
- `--debug`: enable debug logging

The default CLI output format is `spdx-json`.

### Validate A BOM Locally

```bash
go run ./cmd/bom check bom.json
```

Supported input BOM formats for `check`:

- `spdx-json`
- `csbom-json`
- `csbom-yaml`
- `csbom`
- `csbom-v2-json`
- `csbom-v2-yaml`
- `csbom-v2`

For each container image and Helm chart OCI reference found in the BOM, `check` validates that the upstream registry serves a manifest for that reference.

### Configure BOM Checks

Add a `.bomlint.yml` file at the repository root to share checker settings between local CLI runs and the check Action:

```yaml
excludePaths:
  - charts/legacy
  - charts/experimental/*/bom.yaml
allowedRegistries:
  - docker.io
  - ghcr.io
  - registry.example.com:5000
```

- `excludePaths` uses the same exact-path, directory-prefix, and glob matching as the check Action's `exclude-paths` input. Configured and Action-supplied exclusions are combined.
- `allowedRegistries` is an exact allowlist of registry hosts, with optional ports. Do not include a URL scheme or repository path. Unqualified image references such as `busybox:latest` resolve to `docker.io`.
- If `allowedRegistries` is omitted or empty, registry allowlist enforcement is disabled for backward compatibility.

The CLI searches for `.bomlint.yml` from the BOM file's directory upward. Registry restrictions apply to every container image and Helm chart OCI reference in supported BOMs. The checker validates the existence of both kinds of reference in their upstream registries.

If a registry requires authentication first:

```bash
printf '%s\n' "$TOKEN" | go run ./cmd/bom registry login ghcr.io -u "$USER" --password-stdin
go run ./cmd/bom check bom.json
```

Credentials are stored in the Docker config used by `crane` and the Docker CLI, honoring `DOCKER_CONFIG` when it is set.

## Developers

### Common Workflows

Use the `Makefile` targets for standard development tasks:

```bash
make help
make check-tools
make build
make build-action
make test
make test-e2e
make fmt
make fmt-check
make lint
make dist
```

`make check-tools` verifies that `helm` is installed before build and test workflows that need it.

### Repository Layout

- [cmd/bom](/Users/schrodit/dev/cs/bom/cmd/bom): CLI entrypoint
- [cmd/bom-action](/Users/schrodit/dev/cs/bom/cmd/bom-action): GitHub Action wrapper entrypoint
- [internal/helm](/Users/schrodit/dev/cs/bom/internal/helm): Helm templating
- [internal/images](/Users/schrodit/dev/cs/bom/internal/images): image extraction, configured images, validation
- [internal/sbom](/Users/schrodit/dev/cs/bom/internal/sbom): SPDX and `csbom` / `csbom-v2` formatting and parsing
- [internal/ghaction](/Users/schrodit/dev/cs/bom/internal/ghaction): shared action filtering, path resolution, and output handling
- [generate/action.yml](/Users/schrodit/dev/cs/bom/generate/action.yml) and [check/action.yml](/Users/schrodit/dev/cs/bom/check/action.yml): composite Action entrypoints
- [scripts/build-dist.sh](/Users/schrodit/dev/cs/bom/scripts/build-dist.sh): dist artifact builder

### Action Development

The composite Actions execute the prebuilt `bom-action` binaries from `dist/`, so action changes usually need updated dist artifacts:

```bash
make build-action
make dist
```

The current dist matrix in this repository is:

- `dist/linux-amd64/bom`
- `dist/linux-amd64/bom-action`
- `dist/linux-arm64/bom`
- `dist/linux-arm64/bom-action`

### Output Formats

Available output formats:

- `spdx-json` default CLI format
- `csbom-v2-json` default generate Action format
- `csbom-json`
- `csbom-yaml`
- `csbom`
- `csbom-v2-yaml`
- `csbom-v2`
- `spdx`

SPDX generation uses `github.com/spdx/tools-golang`. Registry validation uses `github.com/google/go-containerregistry/pkg/crane`. The internal BOM formats are defined in [internal/csbom/bom.go](/Users/schrodit/dev/cs/bom/internal/csbom/bom.go) and [internal/csbom/v2/bom.go](/Users/schrodit/dev/cs/bom/internal/csbom/v2/bom.go).
