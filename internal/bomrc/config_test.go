package bomrc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingConfigReturnsEmpty(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(cfg.AdditionalImages) != 0 {
		t.Fatalf("expected no additional images, got %d", len(cfg.AdditionalImages))
	}
}

func TestLoadConfigParsesAdditionalImages(t *testing.T) {
	chartPath := t.TempDir()
	content := []byte(`
additionalImages:
  - key: external
    image: ghcr.io/example/external:2.0.0
  - resource:
      apiVersion: v1
      kind: ConfigMap
      name: extra-images
    key: sidecar
    image: .data.sidecar
imageKeyMappings:
  ghcr.io/example/api: api
bomGenerationValues:
  image:
    repository: ghcr.io/example/api
    tag: latest
`)
	if err := os.WriteFile(filepath.Join(chartPath, fileNames[0]), content, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(chartPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if len(cfg.AdditionalImages) != 2 {
		t.Fatalf("expected 2 additional images, got %d", len(cfg.AdditionalImages))
	}

	direct := cfg.AdditionalImages[0]
	if direct.Key != "external" || direct.Image.Literal != "ghcr.io/example/external:2.0.0" {
		t.Fatalf("unexpected direct image: %#v", direct)
	}
	if direct.Resource != (ResourceRef{}) {
		t.Fatalf("expected direct image resource to be empty, got %#v", direct.Resource)
	}

	got := cfg.AdditionalImages[1]
	if got.Resource.APIVersion != "v1" || got.Resource.Kind != "ConfigMap" || got.Resource.Name != "extra-images" {
		t.Fatalf("unexpected resource: %#v", got.Resource)
	}
	if got.Key != "sidecar" {
		t.Fatalf("unexpected configured key: %q", got.Key)
	}
	if got.Image.Literal != ".data.sidecar" {
		t.Fatalf("unexpected image selector: %q", got.Image.Literal)
	}
	if cfg.ImageKeyMappings["ghcr.io/example/api"] != "api" {
		t.Fatalf("unexpected image key mapping: %#v", cfg.ImageKeyMappings)
	}

	imageValues, ok := cfg.BOMGenerationValues["image"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested image generation values, got %#v", cfg.BOMGenerationValues["image"])
	}
	if imageValues["repository"] != "ghcr.io/example/api" {
		t.Fatalf("unexpected dummy repository: %#v", imageValues["repository"])
	}
	if imageValues["tag"] != "latest" {
		t.Fatalf("unexpected dummy tag: %#v", imageValues["tag"])
	}
}

func TestLoadConfigParsesStructuredAdditionalImage(t *testing.T) {
	chartPath := t.TempDir()
	content := []byte(`
additionalImages:
  - key: external
    image:
      repository: ghcr.io/example/external
      tag: "2.0.0"
      digest: sha256:1234567890123456789012345678901234567890123456789012345678901234
`)
	if err := os.WriteFile(filepath.Join(chartPath, fileNames[0]), content, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(chartPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if len(cfg.AdditionalImages) != 1 {
		t.Fatalf("expected 1 additional image, got %d", len(cfg.AdditionalImages))
	}

	got := cfg.AdditionalImages[0].Image
	if got.Repository != "ghcr.io/example/external" || got.Tag != "2.0.0" || got.Digest != "sha256:1234567890123456789012345678901234567890123456789012345678901234" {
		t.Fatalf("unexpected structured image: %#v", got)
	}

	ref, ok := got.Ref()
	if !ok {
		t.Fatal("expected Ref() to succeed")
	}
	want := "ghcr.io/example/external:2.0.0@sha256:1234567890123456789012345678901234567890123456789012345678901234"
	if ref != want {
		t.Fatalf("unexpected ref: got %q, want %q", ref, want)
	}
}

func TestImageValueRejectsMissingTagAndDigest(t *testing.T) {
	var v ImageValue
	err := v.UnmarshalJSON([]byte(`{"repository":"ghcr.io/example/external"}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestImageValueRejectsMissingRepository(t *testing.T) {
	var v ImageValue
	err := v.UnmarshalJSON([]byte(`{"tag":"2.0.0"}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadSupportsYAMLConfigName(t *testing.T) {
	chartPath := t.TempDir()
	content := []byte(`
additionalImages:
  - resource:
      apiVersion: v1
      kind: ConfigMap
      name: extra-images
    image: .data.sidecar
`)
	if err := os.WriteFile(filepath.Join(chartPath, fileNames[1]), content, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(chartPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if len(cfg.AdditionalImages) != 1 {
		t.Fatalf("expected 1 additional image, got %d", len(cfg.AdditionalImages))
	}
}

func TestLoadPrefersYMLWhenBothFilesExist(t *testing.T) {
	chartPath := t.TempDir()
	ymlContent := []byte(`
bomGenerationValues:
  marker: yml
`)
	yamlContent := []byte(`
bomGenerationValues:
  marker: yaml
`)
	if err := os.WriteFile(filepath.Join(chartPath, fileNames[0]), ymlContent, 0o644); err != nil {
		t.Fatalf("write yml config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(chartPath, fileNames[1]), yamlContent, 0o644); err != nil {
		t.Fatalf("write yaml config: %v", err)
	}

	cfg, err := Load(chartPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.BOMGenerationValues["marker"] != "yml" {
		t.Fatalf("expected .bomrc.yml to win, got %#v", cfg.BOMGenerationValues["marker"])
	}
}
