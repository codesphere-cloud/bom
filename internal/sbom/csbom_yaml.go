package sbom

import (
	"io"

	"sigs.k8s.io/yaml"
)

type CSBOMYAMLFormatter struct{}

func (CSBOMYAMLFormatter) Format(w io.Writer, document Document) error {
	document = withDefaults(document)

	content, err := yaml.Marshal(csbomPayload(document))
	if err != nil {
		return err
	}

	_, err = w.Write(content)
	return err
}
