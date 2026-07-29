package sbom

import (
	"time"

	"github.com/codesphere-cloud/helm-bom/internal/images"
)

type Document struct {
	Metadata   Metadata    `json:"metadata"`
	Components []Component `json:"components"`
}

type Metadata struct {
	GeneratedAt time.Time      `json:"generatedAt"`
	Tool        ToolMetadata   `json:"tool"`
	Source      SourceMetadata `json:"source"`
}

type ToolMetadata struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type SourceMetadata struct {
	Chart       string   `json:"chart"`
	ChartName   string   `json:"chartName"`
	ReleaseName string   `json:"releaseName"`
	Namespace   string   `json:"namespace"`
	ValuesFiles []string `json:"valuesFiles,omitempty"`
	SetValues   []string `json:"setValues,omitempty"`
	SetStrings  []string `json:"setStrings,omitempty"`
	HelmArgs    []string `json:"helmArgs,omitempty"`
}

type Component struct {
	Type       string   `json:"type"`
	Repository string   `json:"repository"`
	Reference  string   `json:"reference"`
	Tag        string   `json:"tag,omitempty"`
	Digest     string   `json:"digest,omitempty"`
	Evidence   []string `json:"evidence,omitempty"`
}

func ComponentsFromImages(refs []images.ImageRef) []Component {
	components := make([]Component, 0, len(refs))
	for _, ref := range refs {
		components = append(components, Component{
			Type:       "oci-image",
			Repository: ref.Repository,
			Reference:  ref.Reference,
			Tag:        ref.Tag,
			Digest:     ref.Digest,
			Evidence:   ref.Sources,
		})
	}

	return components
}
