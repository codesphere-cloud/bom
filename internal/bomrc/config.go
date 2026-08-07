package bomrc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"
)

var fileNames = []string{".bomrc.yml", ".bomrc.yaml"}

type Config struct {
	AdditionalImages []AdditionalImage `json:"additionalImages"`
	ImageKeyMappings map[string]string `json:"imageKeyMappings,omitempty"`
	BOMGenerationValues map[string]any `json:"bomGenerationValues,omitempty"`
}

type AdditionalImage struct {
	Resource ResourceRef `json:"resource"`
	Key      string      `json:"key,omitempty"`
	Image    string      `json:"image"`
}

type ResourceRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
}

func Load(chartPath string) (Config, error) {
	for _, fileName := range fileNames {
		path := filepath.Join(chartPath, fileName)
		content, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return Config{}, fmt.Errorf("read %s: %w", path, err)
		}

		var cfg Config
		if err := yaml.Unmarshal(content, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse %s: %w", path, err)
		}

		return cfg, nil
	}

	return Config{}, nil
}
