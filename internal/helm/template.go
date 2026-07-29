package helm

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type TemplateRequest struct {
	ChartPath   string
	ReleaseName string
	Namespace   string
	ValuesFiles []string
	SetValues   []string
	SetStrings  []string
	ExtraArgs   []string
}

type Renderer struct{}

type chartMetadata struct {
	Name string `yaml:"name"`
}

func (Renderer) Template(req TemplateRequest) ([]byte, error) {
	if err := EnsureInstalled(); err != nil {
		return nil, err
	}

	args := []string{"template", req.ReleaseName, req.ChartPath, "--namespace", req.Namespace}

	for _, valuesFile := range req.ValuesFiles {
		args = append(args, "--values", valuesFile)
	}

	for _, setValue := range req.SetValues {
		args = append(args, "--set", setValue)
	}

	for _, setString := range req.SetStrings {
		args = append(args, "--set-string", setString)
	}

	args = append(args, req.ExtraArgs...)

	cmd := exec.Command("helm", args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("helm template failed: %w: %s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

func EnsureInstalled() error {
	if _, err := exec.LookPath("helm"); err != nil {
		return fmt.Errorf("required tool not found: helm: %w", err)
	}

	return nil
}

func ChartName(chartPath string) (string, error) {
	metadataPath := filepath.Join(chartPath, "Chart.yaml")
	content, err := os.ReadFile(metadataPath)
	if err != nil {
		return "", fmt.Errorf("read chart metadata: %w", err)
	}

	var metadata chartMetadata
	if err := yaml.Unmarshal(content, &metadata); err != nil {
		return "", fmt.Errorf("parse chart metadata: %w", err)
	}

	name := strings.TrimSpace(metadata.Name)
	if name == "" {
		return "", fmt.Errorf("chart metadata %s does not declare a name", metadataPath)
	}

	return name, nil
}
