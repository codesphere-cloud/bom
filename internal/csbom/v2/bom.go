package v2

type BOM struct {
	Version         string                    `json:"version,omitempty"`
	Name            string                    `json:"name,omitempty"`
	HelmCharts      map[string]ContainerImage `json:"helmCharts,omitempty"`
	ContainerImages map[string]ContainerImage `json:"containerImages,omitempty"`
}

type HelmCharts struct {
	Ref string `json:"ref"`
}

type ContainerImage struct {
	Ref     string   `json:"ref"`
	Sources []string `json:"sources,omitempty"`
}
