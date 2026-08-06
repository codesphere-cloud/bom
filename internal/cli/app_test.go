package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codesphere-cloud/helm-bom/internal/images"
	dockerconfig "github.com/docker/cli/cli/config"
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

func TestRunRegistryLoginStoresCredentials(t *testing.T) {
	dockerConfigDir := t.TempDir()

	var stdout bytes.Buffer
	err := runRegistryLogin(strings.NewReader(""), &stdout, registryLoginConfig{
		server:   "ghcr.io",
		username: "alice",
		password: "secret",
	}, dockerConfigDir)
	if err != nil {
		t.Fatalf("runRegistryLogin returned error: %v", err)
	}

	configPath := filepath.Join(dockerConfigDir, "config.json")
	if _, err := os.ReadFile(configPath); err != nil {
		t.Fatalf("read docker config: %v", err)
	}

	cf, err := dockerconfig.Load(dockerConfigDir)
	if err != nil {
		t.Fatalf("load docker config: %v", err)
	}

	auth, err := cf.GetAuthConfig("ghcr.io")
	if err != nil {
		t.Fatalf("get auth config: %v", err)
	}

	if auth.Username != "alice" || auth.Password != "secret" {
		t.Fatalf("unexpected auth config: %#v", auth)
	}

	if !strings.Contains(stdout.String(), "logged in to ghcr.io") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestRunRegistryLoginReadsPasswordFromStdin(t *testing.T) {
	dockerConfigDir := t.TempDir()

	err := runRegistryLogin(strings.NewReader("hunter2\n"), &bytes.Buffer{}, registryLoginConfig{
		server:        "docker.io",
		username:      "bob",
		passwordStdin: true,
	}, dockerConfigDir)
	if err != nil {
		t.Fatalf("runRegistryLogin returned error: %v", err)
	}

	cf, err := dockerconfig.Load(dockerConfigDir)
	if err != nil {
		t.Fatalf("load docker config: %v", err)
	}

	auth, err := cf.GetAuthConfig("docker.io")
	if err != nil {
		t.Fatalf("get auth config: %v", err)
	}

	if auth.Username != "bob" || auth.Password != "hunter2" {
		t.Fatalf("unexpected auth config: %#v", auth)
	}
}
