package bomrc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"
)

const fileName = ".bomrc.yml"

type Config struct {
	AdditionalImages []AdditionalImage `json:"additionalImages"`
}

type AdditionalImage struct {
	Resource ResourceRef `json:"resource"`
	Image    string      `json:"image"`
}

type ResourceRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
}

func Load(chartPath string) (Config, error) {
	path := filepath.Join(chartPath, fileName)
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}

	return cfg, nil
}
