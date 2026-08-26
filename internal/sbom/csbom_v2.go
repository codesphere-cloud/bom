package sbom

import intcsbomv2 "github.com/codesphere-cloud/bom/internal/csbom/v2"

const csbomV2Version = "2"

func csbomV2Payload(document Document) intcsbomv2.BOM {
	images := make(map[string]intcsbomv2.ContainerImage, len(document.Components))
	for _, component := range document.Components {
		image := intcsbomv2.ContainerImage{
			Ref:     component.Reference,
			Digest:  component.Digest,
			Sources: component.Evidence,
		}
		if component.SBOMs.CycloneDX.Path != "" || component.SBOMs.SPDXJSON.Path != "" {
			image.SBOMs = &intcsbomv2.SBOMs{}
			if component.SBOMs.CycloneDX.Path != "" {
				image.SBOMs.CycloneDX = &intcsbomv2.SBOM{
					Path:   component.SBOMs.CycloneDX.Path,
					Cosign: component.SBOMs.CycloneDX.Cosign,
				}
			}
			if component.SBOMs.SPDXJSON.Path != "" {
				image.SBOMs.SPDXJSON = &intcsbomv2.SBOM{
					Path:   component.SBOMs.SPDXJSON.Path,
					Cosign: component.SBOMs.SPDXJSON.Cosign,
				}
			}
		}
		images[component.Repository] = image
	}

	return intcsbomv2.BOM{
		Version:         csbomV2Version,
		Name:            componentName(document.Metadata.Source),
		ContainerImages: images,
	}
}
