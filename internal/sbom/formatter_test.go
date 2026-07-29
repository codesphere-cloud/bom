package sbom

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestNewFormatterSupportsCSBOM(t *testing.T) {
	formatter, err := NewFormatter("csbom")
	if err != nil {
		t.Fatalf("NewFormatter returned error: %v", err)
	}

	if _, ok := formatter.(CSBOMJSONFormatter); !ok {
		t.Fatalf("expected CSBOMJSONFormatter, got %T", formatter)
	}
}

func TestCSBOMFormatterUsesInternalShape(t *testing.T) {
	document := Document{
		Metadata: Metadata{
			Source: SourceMetadata{
				ChartName: "e2e-chart",
			},
		},
		Components: []Component{
			{
				Repository: "ghcr.io/example/api",
				Reference:  "ghcr.io/example/api:1.2.3",
			},
			{
				Repository: "busybox",
				Reference:  "busybox:1.36.1",
			},
		},
	}

	var buffer bytes.Buffer
	formatter := CSBOMJSONFormatter{}
	if err := formatter.Format(&buffer, document); err != nil {
		t.Fatalf("Format returned error: %v", err)
	}

	var payload struct {
		Components map[string]struct {
			ContainerImages map[string]string `json:"containerImages"`
		} `json:"components"`
	}
	if err := json.Unmarshal(buffer.Bytes(), &payload); err != nil {
		t.Fatalf("decode csbom output: %v", err)
	}

	component, ok := payload.Components["e2e-chart"]
	if !ok {
		t.Fatalf("expected e2e-chart component, got %v", payload.Components)
	}

	if got := component.ContainerImages["ghcr.io/example/api"]; got != "ghcr.io/example/api:1.2.3" {
		t.Fatalf("unexpected api image: %q", got)
	}

	if got := component.ContainerImages["busybox"]; got != "busybox:1.36.1" {
		t.Fatalf("unexpected busybox image: %q", got)
	}
}
