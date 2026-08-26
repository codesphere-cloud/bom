package sbom

import (
	"time"

	"github.com/codesphere-cloud/bom/internal/images"
)

type Document struct {
	Metadata   Metadata    `json:"metadata"`
	Components []Component `json:"components"`
}

const (
	ComponentTypeOCIImage  = "oci-image"
	ComponentTypeHelmChart = "helm-chart"
)

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
	SBOMs      SBOMs    `json:"sboms,omitempty"`
	Evidence   []string `json:"evidence,omitempty"`
}

type SBOMs struct {
	CycloneDX SBOM `json:"cyclonedx,omitempty"`
	SPDXJSON  SBOM `json:"spdxJson,omitempty"`
}

type SBOM struct {
	Path   string `json:"path"`
	Cosign bool   `json:"cosign"`
}

func ComponentsFromImages(refs []images.ImageRef) []Component {
	components := make([]Component, 0, len(refs))
	for _, ref := range refs {
		components = append(components, Component{
			Type:       ComponentTypeOCIImage,
			Repository: ref.Repository,
			Reference:  ref.Reference,
			Tag:        ref.Tag,
			Digest:     ref.Digest,
			Evidence:   ref.Sources,
		})
	}

	return components
}
