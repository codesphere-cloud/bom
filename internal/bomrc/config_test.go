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
  - resource:
      apiVersion: v1
      kind: ConfigMap
      name: extra-images
    key: sidecar
    image: .data.sidecar
imageKeyMappings:
  ghcr.io/example/api: api
dummyValues:
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

	if len(cfg.AdditionalImages) != 1 {
		t.Fatalf("expected 1 additional image, got %d", len(cfg.AdditionalImages))
	}

	got := cfg.AdditionalImages[0]
	if got.Resource.APIVersion != "v1" || got.Resource.Kind != "ConfigMap" || got.Resource.Name != "extra-images" {
		t.Fatalf("unexpected resource: %#v", got.Resource)
	}
	if got.Key != "sidecar" {
		t.Fatalf("unexpected configured key: %q", got.Key)
	}
	if got.Image != ".data.sidecar" {
		t.Fatalf("unexpected image selector: %q", got.Image)
	}
	if cfg.ImageKeyMappings["ghcr.io/example/api"] != "api" {
		t.Fatalf("unexpected image key mapping: %#v", cfg.ImageKeyMappings)
	}

	imageValues, ok := cfg.DummyValues["image"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested image dummy values, got %#v", cfg.DummyValues["image"])
	}
	if imageValues["repository"] != "ghcr.io/example/api" {
		t.Fatalf("unexpected dummy repository: %#v", imageValues["repository"])
	}
	if imageValues["tag"] != "latest" {
		t.Fatalf("unexpected dummy tag: %#v", imageValues["tag"])
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
dummyValues:
  marker: yml
`)
	yamlContent := []byte(`
dummyValues:
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

	if cfg.DummyValues["marker"] != "yml" {
		t.Fatalf("expected .bomrc.yml to win, got %#v", cfg.DummyValues["marker"])
	}
}
