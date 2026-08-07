package login

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codesphere-cloud/bom/internal/logging"
	dockerconfig "github.com/docker/cli/cli/config"
)

func TestRunStoresCredentials(t *testing.T) {
	dockerConfigDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dockerConfigDir)

	var stdout bytes.Buffer
	err := Run(strings.NewReader(""), &stdout, logging.NewWriterLogger(io.Discard, false), Config{
		Server:   "ghcr.io",
		Username: "alice",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	configPath := filepath.Join(dockerConfigDir, "config.json")
	if _, err := os.ReadFile(configPath); err != nil {
		t.Fatalf("read docker config: %v", err)
	}

	cfg, err := dockerconfig.Load(dockerConfigDir)
	if err != nil {
		t.Fatalf("load docker config: %v", err)
	}
	auth, err := cfg.GetAuthConfig("ghcr.io")
	if err != nil {
		t.Fatalf("get auth config: %v", err)
	}
	if auth.Username != "alice" || auth.Password != "secret" {
		t.Fatalf("unexpected auth config: %#v", auth)
	}
}

func TestRunReadsPasswordFromStdin(t *testing.T) {
	dockerConfigDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dockerConfigDir)

	err := Run(strings.NewReader("hunter2\n"), &bytes.Buffer{}, logging.NewWriterLogger(io.Discard, false), Config{
		Server:        "docker.io",
		Username:      "bob",
		PasswordStdin: true,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	cfg, err := dockerconfig.Load(dockerConfigDir)
	if err != nil {
		t.Fatalf("load docker config: %v", err)
	}
	auth, err := cfg.GetAuthConfig("docker.io")
	if err != nil {
		t.Fatalf("get auth config: %v", err)
	}
	if auth.Username != "bob" || auth.Password != "hunter2" {
		t.Fatalf("unexpected auth config: %#v", auth)
	}
}
