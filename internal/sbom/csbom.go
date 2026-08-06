package sbom

import intcsbom "github.com/codesphere-cloud/helm-bom/internal/csbom"

func csbomPayload(document Document) intcsbom.Config {
	return intcsbom.Config{
		Components: map[string]intcsbom.ComponentConfig{
			componentName(document.Metadata.Source): {
				ContainerImages: containerImages(document.Components),
			},
		},
	}
}
