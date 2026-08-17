package bomlint

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

const FileName = ".bomlint.yml"

type Config struct {
	ExcludePaths      []string `json:"excludePaths,omitempty"`
	AllowedRegistries []string `json:"allowedRegistries,omitempty"`
}

// Excludes reports whether target is excluded by one of the configured paths.
// Relative exclusion paths are resolved from the directory containing the
// configuration file.
func (config Config) Excludes(configRoot string, target string) bool {
	if !filepath.IsAbs(target) {
		absoluteTarget, err := filepath.Abs(target)
		if err != nil {
			return false
		}
		target = absoluteTarget
	}
	relativeTarget, err := filepath.Rel(configRoot, target)
	if err != nil {
		return false
	}
	return MatchesExcludedPath(filepath.ToSlash(relativeTarget), config.ExcludePaths)
}

// MatchesExcludedPath reports whether target matches an exact path, directory
// prefix, or glob in selectors. Paths use slash separators so configuration is
// portable between operating systems.
func MatchesExcludedPath(target string, selectors []string) bool {
	target = canonicalPath(target)
	for _, selector := range selectors {
		selector = canonicalPath(selector)
		if selector == "" {
			continue
		}

		if strings.ContainsAny(selector, "*?[") {
			matched, err := path.Match(selector, target)
			if err == nil && matched {
				return true
			}
			continue
		}

		if target == selector || strings.HasPrefix(target, selector+"/") {
			return true
		}
	}
	return false
}

func canonicalPath(value string) string {
	value = strings.TrimSpace(filepath.ToSlash(value))
	value = strings.TrimPrefix(value, "./")
	value = strings.TrimSuffix(value, "/")
	return value
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
