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

func TestMatchesExcludedPath(t *testing.T) {
	selectors := []string{"boms/legacy", "boms/*/generated.yaml", "./boms/archive/"}
	tests := []struct {
		path string
		want bool
	}{
		{path: "boms/legacy/bom.yaml", want: true},
		{path: "boms/api/generated.yaml", want: true},
		{path: "boms/archive/bom.json", want: true},
		{path: "boms/legacy-v2/bom.yaml", want: false},
		{path: "boms/api/bom.yaml", want: false},
	}

	for _, tt := range tests {
		if got := MatchesExcludedPath(tt.path, selectors); got != tt.want {
			t.Errorf("MatchesExcludedPath(%q) = %t, want %t", tt.path, got, tt.want)
		}
	}
}

func TestConfigExcludesRelativeTarget(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	config := Config{ExcludePaths: []string{"fixtures/legacy"}}
	if !config.Excludes(workingDirectory, "fixtures/legacy/bom.yaml") {
		t.Fatal("expected relative target to be excluded")
	}
}
