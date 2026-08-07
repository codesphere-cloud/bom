package sbom

import (
	"strings"
	"testing"
)

func TestParseCSBOMYAML(t *testing.T) {
	document, err := Parse(strings.NewReader(`
components:
  chart:
    containerImages:
      ghcr.io/example/api: ghcr.io/example/api:1.2.3
      quay.io/example/worker: quay.io/example/worker@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
`))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	got := ImageRefs(document)
	if len(got) != 2 {
		t.Fatalf("expected 2 image refs, got %d", len(got))
	}

	if got[0].Reference != "ghcr.io/example/api:1.2.3" {
		t.Fatalf("unexpected first ref: %q", got[0].Reference)
	}

	if got[1].Reference != "quay.io/example/worker@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("unexpected second ref: %q", got[1].Reference)
	}
}

func TestParseCSBOMV2YAML(t *testing.T) {
	document, err := Parse(strings.NewReader(`
version: "2"
name: chart
containerImages:
  ghcr.io/example/api:
    ref: ghcr.io/example/api:1.2.3
    sources:
      - Deployment/api spec.containers[0]
  quay.io/example/worker:
    ref: quay.io/example/worker@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
`))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	got := ImageRefs(document)
	if len(got) != 2 {
		t.Fatalf("expected 2 image refs, got %d", len(got))
	}

	if got[0].Reference != "ghcr.io/example/api:1.2.3" {
		t.Fatalf("unexpected first ref: %q", got[0].Reference)
	}

	if got[1].Reference != "quay.io/example/worker@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("unexpected second ref: %q", got[1].Reference)
	}

	if len(document.Components[0].Evidence) != 1 || document.Components[0].Evidence[0] != "Deployment/api spec.containers[0]" {
		t.Fatalf("unexpected evidence: %#v", document.Components[0].Evidence)
	}
}

func TestParseCSBOMV2JSON(t *testing.T) {
	document, err := Parse(strings.NewReader(`{
  "version": "2",
  "name": "chart",
  "containerImages": {
    "ghcr.io/example/api": {
      "ref": "ghcr.io/example/api:1.2.3",
      "sources": ["Deployment/api spec.containers[0]"]
    },
    "quay.io/example/worker": {
      "ref": "quay.io/example/worker@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
    }
  }
}`))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	got := ImageRefs(document)
	if len(got) != 2 {
		t.Fatalf("expected 2 image refs, got %d", len(got))
	}

	if got[0].Reference != "ghcr.io/example/api:1.2.3" {
		t.Fatalf("unexpected first ref: %q", got[0].Reference)
	}

	if got[1].Reference != "quay.io/example/worker@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("unexpected second ref: %q", got[1].Reference)
	}

	if len(document.Components[0].Evidence) != 1 || document.Components[0].Evidence[0] != "Deployment/api spec.containers[0]" {
		t.Fatalf("unexpected evidence: %#v", document.Components[0].Evidence)
	}
}

func TestParseCSBOMIncludesHelmChartOCIRefs(t *testing.T) {
	document, err := Parse(strings.NewReader(`
components:
  chart:
    containerImages:
      ghcr.io/example/api: ghcr.io/example/api:1.2.3
    files:
      dependency:
        ociRef: registry.example.com/charts/dependency:2.0.0
`))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	refs := OCIRefs(document)
	if len(refs) != 2 {
		t.Fatalf("expected image and Helm chart refs, got %#v", refs)
	}
	if document.Components[1].Type != ComponentTypeHelmChart {
		t.Fatalf("expected Helm chart component, got %#v", document.Components[1])
	}
}

func TestParseCSBOMV2IncludesHelmChartOCIRefs(t *testing.T) {
	document, err := Parse(strings.NewReader(`
version: "2"
name: chart
helmCharts:
  dependency:
    ref: registry.example.com/charts/dependency:2.0.0
containerImages:
  ghcr.io/example/api:
    ref: ghcr.io/example/api:1.2.3
`))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	refs := OCIRefs(document)
	if len(refs) != 2 {
		t.Fatalf("expected image and Helm chart refs, got %#v", refs)
	}
	if len(ImageRefs(document)) != 1 {
		t.Fatalf("expected only the container image from ImageRefs, got %#v", ImageRefs(document))
	}
}

func TestParseSPDXJSON(t *testing.T) {
	document, err := Parse(strings.NewReader(`{
  "spdxVersion": "SPDX-2.3",
  "dataLicense": "CC0-1.0",
  "SPDXID": "SPDXRef-DOCUMENT",
  "name": "chart",
  "documentNamespace": "https://example.com/spdx/test",
  "creationInfo": {
    "created": "2026-08-06T00:00:00Z",
    "creators": ["Tool: bom-dev"]
  },
  "packages": [
    {
      "name": "ghcr.io/example/api",
      "SPDXID": "SPDXRef-Package-001",
      "versionInfo": "1.2.3",
      "downloadLocation": "NOASSERTION",
      "filesAnalyzed": false,
      "summary": "ghcr.io/example/api:1.2.3"
    },
    {
      "name": "quay.io/example/worker",
      "SPDXID": "SPDXRef-Package-002",
      "versionInfo": "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
      "downloadLocation": "NOASSERTION",
      "filesAnalyzed": false,
      "summary": "quay.io/example/worker@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
    }
  ]
}`))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	got := ImageRefs(document)
	if len(got) != 2 {
		t.Fatalf("expected 2 image refs, got %d", len(got))
	}

	if got[0].Reference != "ghcr.io/example/api:1.2.3" {
		t.Fatalf("unexpected first ref: %q", got[0].Reference)
	}

	if got[1].Reference != "quay.io/example/worker@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("unexpected second ref: %q", got[1].Reference)
	}
}
