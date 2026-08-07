package sbom

import intcsbomv2 "github.com/codesphere-cloud/helm-bom/internal/csbom/v2"

const csbomV2Version = "2"

func csbomV2Payload(document Document) intcsbomv2.BOM {
	images := make(map[string]intcsbomv2.ContainerImage, len(document.Components))
	for _, component := range document.Components {
		images[component.Repository] = intcsbomv2.ContainerImage{
			Ref:     component.Reference,
			Sources: component.Evidence,
		}
	}

	return intcsbomv2.BOM{
		Version:         csbomV2Version,
		Name:            componentName(document.Metadata.Source),
		ContainerImages: images,
	}
}
