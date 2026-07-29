package sbom

import (
	"encoding/json"
	"io"

	intcsbom "github.com/codesphere-cloud/helm-bom/internal/csbom"
)

type CSBOMJSONFormatter struct{}

func (CSBOMJSONFormatter) Format(w io.Writer, document Document) error {
	document = withDefaults(document)

	payload := intcsbom.Config{
		Components: map[string]intcsbom.ComponentConfig{
			componentName(document.Metadata.Source): {
				ContainerImages: containerImages(document.Components),
			},
		},
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	return encoder.Encode(payload)
}

func componentName(source SourceMetadata) string {
	if source.ChartName != "" {
		return source.ChartName
	}
	if source.ReleaseName != "" {
		return source.ReleaseName
	}
	return "chart"
}

func containerImages(components []Component) map[string]string {
	if len(components) == 0 {
		return nil
	}

	images := make(map[string]string, len(components))
	for _, component := range components {
		images[component.Repository] = component.Reference
	}

	return images
}
