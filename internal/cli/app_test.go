package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codesphere-cloud/helm-bom/internal/images"
	"github.com/codesphere-cloud/helm-bom/internal/logging"
	dockerconfig "github.com/docker/cli/cli/config"
	"sigs.k8s.io/yaml"
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
	if err := runCheckWithValidator(&stdout, logging.NewWriterLogger(io.Discard, false), checkConfig{bomPath: path}, func(refs []images.ImageRef) error {
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
	t.Setenv("DOCKER_CONFIG", dockerConfigDir)

	var stdout bytes.Buffer
	err := runRegistryLogin(strings.NewReader(""), &stdout, logging.NewWriterLogger(io.Discard, false), registryLoginConfig{
		server:   "ghcr.io",
		username: "alice",
		password: "secret",
	})
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
}

func TestRunRegistryLoginReadsPasswordFromStdin(t *testing.T) {
	dockerConfigDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dockerConfigDir)

	err := runRegistryLogin(strings.NewReader("hunter2\n"), &bytes.Buffer{}, logging.NewWriterLogger(io.Discard, false), registryLoginConfig{
		server:        "docker.io",
		username:      "bob",
		passwordStdin: true,
	})
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

func TestPrependBOMGenerationValuesFile(t *testing.T) {
	valuesFiles := []string{"values.yaml"}
	cleanup, err := prependBOMGenerationValuesFile(&valuesFiles, map[string]any{
		"image": map[string]any{
			"repository": "ghcr.io/example/api",
			"tag":        "latest",
		},
	})
	if err != nil {
		t.Fatalf("prependBOMGenerationValuesFile returned error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("expected cleanup function")
	}

	if len(valuesFiles) != 2 {
		t.Fatalf("expected 2 values files, got %d", len(valuesFiles))
	}
	if got, want := valuesFiles[1], "values.yaml"; got != want {
		t.Fatalf("unexpected values file order:\nwant second: %s\ngot:  %s", want, got)
	}

	content, err := os.ReadFile(valuesFiles[0])
	if err != nil {
		t.Fatalf("read bom generation values file: %v", err)
	}

	var payload map[string]any
	if err := yaml.Unmarshal(content, &payload); err != nil {
		t.Fatalf("unmarshal bom generation values file: %v", err)
	}

	imageValues, ok := payload["image"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested image payload, got %#v", payload["image"])
	}
	if imageValues["repository"] != "ghcr.io/example/api" {
		t.Fatalf("unexpected repository: %#v", imageValues["repository"])
	}
	if imageValues["tag"] != "latest" {
		t.Fatalf("unexpected tag: %#v", imageValues["tag"])
	}

	path := valuesFiles[0]
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected bom generation values file to be removed, stat err=%v", err)
	}
}
