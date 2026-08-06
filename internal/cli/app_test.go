package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codesphere-cloud/helm-bom/internal/images"
)

func TestRunCheckValidatesBOM(t *testing.T) {
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
	var stdout bytes.Buffer
	if err := runCheckWithValidator(&stdout, checkConfig{bomPath: path}, func(refs []images.ImageRef) error {
		for _, ref := range refs {
			calls = append(calls, ref.Reference)
		}
		return nil
	}); err != nil {
		t.Fatalf("runCheckWithValidator returned error: %v", err)
	}

	if got, want := strings.Join(calls, ","), "busybox:1.36.1,ghcr.io/example/api:1.2.3"; got != want {
		t.Fatalf("unexpected validated refs:\nwant: %s\ngot:  %s", want, got)
	}

	if !strings.Contains(stdout.String(), "validated 2 image reference(s)") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}
