package v2

type BOM struct {
	Version         string                    `json:"version,omitempty"`
	Name            string                    `json:"name,omitempty"`
	HelmCharts      map[string]HelmCharts     `json:"helmCharts,omitempty"`
	ContainerImages map[string]ContainerImage `json:"containerImages,omitempty"`
}

type HelmCharts struct {
	Ref string `json:"ref"`
}

type ContainerImage struct {
	Ref     string   `json:"ref"`
	Digest  string   `json:"digest,omitempty"`
	SBOMs   *SBOMs   `json:"sboms,omitempty"`
	Sources []string `json:"sources,omitempty"`
}

type SBOMs struct {
	CycloneDX *SBOM `json:"cyclonedx,omitempty"`
	SPDXJSON  *SBOM `json:"spdxJson,omitempty"`
}

type SBOM struct {
	Path   string `json:"path"`
	Cosign bool   `json:"cosign"`
}
