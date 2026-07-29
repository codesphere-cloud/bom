package helm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChartName(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte("name: example-chart\nversion: 0.1.0\n"), 0o644); err != nil {
		t.Fatalf("write Chart.yaml: %v", err)
	}

	name, err := ChartName(dir)
	if err != nil {
		t.Fatalf("ChartName returned error: %v", err)
	}

	if name != "example-chart" {
		t.Fatalf("expected chart name example-chart, got %s", name)
	}
}

func TestEnsureInstalled(t *testing.T) {
	if err := EnsureInstalled(); err != nil {
		t.Fatalf("expected helm to be installed in test environment: %v", err)
	}
}
