package bomlint

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"
)

const FileName = ".bomlint.yml"

type Config struct {
	ExcludePaths      []string `json:"excludePaths,omitempty"`
	AllowedRegistries []string `json:"allowedRegistries,omitempty"`
}

func Load(directory string) (Config, bool, error) {
	path := filepath.Join(directory, FileName)
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, false, nil
	}
	if err != nil {
		return Config{}, false, fmt.Errorf("read %s: %w", path, err)
	}

	var config Config
	if err := yaml.UnmarshalStrict(content, &config); err != nil {
		return Config{}, false, fmt.Errorf("parse %s: %w", path, err)
	}

	return config, true, nil
}

func Find(path string) (Config, string, bool, error) {
	directory := path
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		directory = filepath.Dir(path)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, "", false, fmt.Errorf("stat %s: %w", path, err)
	}

	directory, err = filepath.Abs(directory)
	if err != nil {
		return Config{}, "", false, fmt.Errorf("resolve config search path: %w", err)
	}

	for {
		config, found, loadErr := Load(directory)
		if loadErr != nil {
			return Config{}, "", false, loadErr
		}
		if found {
			return config, directory, true, nil
		}

		parent := filepath.Dir(directory)
		if parent == directory {
			return Config{}, "", false, nil
		}
		directory = parent
	}
}
