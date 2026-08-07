package bomlint

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestFindLoadsNearestConfig(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, FileName)
	if err := os.WriteFile(configPath, []byte(`
excludePaths:
  - charts/legacy
allowedRegistries:
  - ghcr.io
  - registry.example.com:5000
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	bomPath := filepath.Join(root, "charts", "api", "bom.yaml")
	if err := os.MkdirAll(filepath.Dir(bomPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(bomPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write BOM: %v", err)
	}

	config, directory, found, err := Find(bomPath)
	if err != nil {
		t.Fatalf("Find returned error: %v", err)
	}
	if !found || directory != root {
		t.Fatalf("unexpected config location: found=%t directory=%q", found, directory)
	}
	if !slices.Equal(config.ExcludePaths, []string{"charts/legacy"}) {
		t.Fatalf("unexpected excludes: %#v", config.ExcludePaths)
	}
	if !slices.Equal(config.AllowedRegistries, []string{"ghcr.io", "registry.example.com:5000"}) {
		t.Fatalf("unexpected registries: %#v", config.AllowedRegistries)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, FileName), []byte("allowedRegistry: ghcr.io\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, _, err := Load(directory); err == nil {
		t.Fatal("expected unknown config field to fail")
	}
}
