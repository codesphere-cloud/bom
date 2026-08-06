package sbom

import (
	"encoding/json"
	"io"
)

type CSBOMJSONFormatter struct{}

func (CSBOMJSONFormatter) Format(w io.Writer, document Document) error {
	document = withDefaults(document)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	return encoder.Encode(csbomPayload(document))
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
