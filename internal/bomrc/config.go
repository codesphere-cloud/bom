package bomrc

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"
)

var fileNames = []string{".bomrc.yml", ".bomrc.yaml"}

type Config struct {
	AdditionalImages    []AdditionalImage `json:"additionalImages"`
	ImageKeyMappings    map[string]string `json:"imageKeyMappings,omitempty"`
	BOMGenerationValues map[string]any    `json:"bomGenerationValues,omitempty"`
	ExpectNoImages      bool              `json:"expectNoImages,omitempty"`
}

// CheckImageCount fails when the number of container images found for a chart
// contradicts ExpectNoImages: none found without the declaration, or some
// found with it. Helm chart references are not images and must not be counted.
// fix tells the caller's user how to resolve the no-images case.
func (c Config) CheckImageCount(count int, fix string) error {
	switch {
	case count == 0 && !c.ExpectNoImages:
		return fmt.Errorf("no container images found; %s", fix)
	case count > 0 && c.ExpectNoImages:
		return fmt.Errorf("expectNoImages is set in .bomrc.yml but %d container image(s) were found; remove expectNoImages", count)
	}
	return nil
}

type AdditionalImage struct {
	Resource ResourceRef `json:"resource,omitempty"`
	Key      string      `json:"key,omitempty"`
	Image    ImageValue  `json:"image"`
}

type ResourceRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
}

// ImageValue holds an additional image's "image" field, which may be given
// either as a plain string (a direct OCI reference, or a yq-style selector
// when Resource is set) or as a struct with a repository and a tag and/or
// digest.
type ImageValue struct {
	Literal    string
	Repository string
	Tag        string
	Digest     string
}

// Ref returns the image reference as a single string, combining Repository,
// Tag, and Digest when they were set from struct form.
func (v ImageValue) Ref() (string, bool) {
	if v.Literal != "" {
		return v.Literal, true
	}
	if v.Repository == "" {
		return "", false
	}

	ref := v.Repository
	if v.Tag != "" {
		ref += ":" + v.Tag
	}
	if v.Digest != "" {
		ref += "@" + v.Digest
	}
	return ref, true
}

func (v *ImageValue) UnmarshalJSON(data []byte) error {
	var literal string
	if err := json.Unmarshal(data, &literal); err == nil {
		*v = ImageValue{Literal: literal}
		return nil
	}

	var obj struct {
		Repository string `json:"repository"`
		Tag        string `json:"tag,omitempty"`
		Digest     string `json:"digest,omitempty"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("image must be a string or an object with repository and tag/digest: %w", err)
	}
	if obj.Repository == "" {
		return fmt.Errorf("image object must set repository")
	}
	if obj.Tag == "" && obj.Digest == "" {
		return fmt.Errorf("image object must set tag and/or digest")
	}

	*v = ImageValue{Repository: obj.Repository, Tag: obj.Tag, Digest: obj.Digest}
	return nil
}

func (v ImageValue) MarshalJSON() ([]byte, error) {
	if v.Repository != "" {
		return json.Marshal(struct {
			Repository string `json:"repository"`
			Tag        string `json:"tag,omitempty"`
			Digest     string `json:"digest,omitempty"`
		}{v.Repository, v.Tag, v.Digest})
	}
	return json.Marshal(v.Literal)
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
