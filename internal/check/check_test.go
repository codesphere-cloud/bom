package check

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codesphere-cloud/bom/internal/images"
	"github.com/codesphere-cloud/bom/internal/logging"
)

func TestRunValidatesBOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bom.yaml")
	if err := os.WriteFile(path, []byte(`
components:
  chart:
    containerImages:
      ghcr.io/example/api: ghcr.io/example/api:1.2.3
      busybox: busybox:1.36.1
`), 0o600); err != nil {
		t.Fatalf("write bom: %v", err)
	}

	calls := make([]string, 0, 2)
	if err := RunWithValidator(logging.NewWriterLogger(io.Discard, false), Config{BOMPath: path, BOMFormat: "csbom"}, func(refs []images.ImageRef) error {
		for _, ref := range refs {
			calls = append(calls, ref.Reference)
		}
		return nil
	}); err != nil {
		t.Fatalf("RunWithValidator returned error: %v", err)
	}

	if got, want := strings.Join(calls, ","), "busybox:1.36.1,ghcr.io/example/api:1.2.3"; got != want {
		t.Fatalf("unexpected validated refs:\nwant: %s\ngot:  %s", want, got)
	}
}

func TestRunSkipsBOMExcludedByBomlintConfig(t *testing.T) {
	tests := []struct {
		name     string
		selector string
		bomPath  string
	}{
		{name: "exact path", selector: "boms/legacy/bom.yaml", bomPath: "boms/legacy/bom.yaml"},
		{name: "directory prefix", selector: "boms/legacy", bomPath: "boms/legacy/bom.yaml"},
		{name: "glob", selector: "boms/*/bom.yaml", bomPath: "boms/legacy/bom.yaml"},
		{name: "relative prefix", selector: "./boms/legacy/", bomPath: "boms/legacy/bom.yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.WriteFile(filepath.Join(directory, ".bomlint.yml"), []byte("excludePaths:\n  - "+tt.selector+"\n"), 0o600); err != nil {
				t.Fatalf("write lint config: %v", err)
			}

			path := filepath.Join(directory, filepath.FromSlash(tt.bomPath))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("mkdir BOM directory: %v", err)
			}
			if err := os.WriteFile(path, []byte("not a valid BOM\n"), 0o600); err != nil {
				t.Fatalf("write BOM: %v", err)
			}

			validatorCalled := false
			err := RunWithValidator(logging.NewWriterLogger(io.Discard, false), Config{BOMPath: path}, func(_ []images.ImageRef) error {
				validatorCalled = true
				return nil
			})
			if err != nil {
				t.Fatalf("excluded BOM returned error: %v", err)
			}
			if validatorCalled {
				t.Fatal("validator was called for excluded BOM")
			}
		})
	}
}

func TestRunEnforcesAllowedRegistriesForImagesAndCharts(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, ".bomlint.yml"), []byte(`
allowedRegistries:
  - ghcr.io
`), 0o600); err != nil {
		t.Fatalf("write lint config: %v", err)
	}

	path := filepath.Join(directory, "bom.yaml")
	if err := os.WriteFile(path, []byte(`
version: "2"
name: chart
helmCharts:
  dependency:
    ref: oci://registry.example.com/charts/dependency:2.0.0
containerImages:
  api:
    ref: ghcr.io/example/api:1.2.3
  worker:
    ref: quay.io/example/worker:3.0.0
`), 0o600); err != nil {
		t.Fatalf("write BOM: %v", err)
	}

	validatorCalled := false
	err := RunWithValidator(logging.NewWriterLogger(io.Discard, false), Config{BOMPath: path}, func(_ []images.ImageRef) error {
		validatorCalled = true
		return nil
	})
	if err == nil {
		t.Fatal("expected disallowed registries to fail")
	}
	if validatorCalled {
		t.Fatal("registry existence validator should not run for a disallowed reference")
	}
	if !strings.Contains(err.Error(), "quay.io/example/worker:3.0.0") ||
		!strings.Contains(err.Error(), "registry.example.com/charts/dependency:2.0.0") {
		t.Fatalf("error does not include every disallowed reference: %v", err)
	}
}

func TestRunValidatesImageAndHelmChartExistence(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, ".bomlint.yml"), []byte(`
allowedRegistries:
  - ghcr.io
`), 0o600); err != nil {
		t.Fatalf("write lint config: %v", err)
	}

	path := filepath.Join(directory, "bom.yaml")
	if err := os.WriteFile(path, []byte(`
version: "2"
name: chart
helmCharts:
  dependency:
    ref: oci://ghcr.io/example/charts/dependency:2.0.0
containerImages:
  api:
    ref: ghcr.io/example/api:1.2.3
`), 0o600); err != nil {
		t.Fatalf("write BOM: %v", err)
	}

	var got []string
	err := RunWithValidator(logging.NewWriterLogger(io.Discard, false), Config{BOMPath: path}, func(refs []images.ImageRef) error {
		for _, ref := range refs {
			got = append(got, ref.Reference)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunWithValidator returned error: %v", err)
	}
	if strings.Join(got, ",") != "ghcr.io/example/api:1.2.3,ghcr.io/example/charts/dependency:2.0.0" {
		t.Fatalf("unexpected validated refs: %#v", got)
	}
}
