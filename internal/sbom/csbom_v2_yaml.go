package sbom

import (
	"io"

	"sigs.k8s.io/yaml"
)

type CSBOMV2YAMLFormatter struct{}

func (CSBOMV2YAMLFormatter) Format(w io.Writer, document Document) error {
	document = withDefaults(document)

	content, err := yaml.Marshal(csbomV2Payload(document))
	if err != nil {
		return err
	}

	_, err = w.Write(content)
	return err
}
