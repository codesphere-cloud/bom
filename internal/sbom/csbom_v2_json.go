package sbom

import (
	"encoding/json"
	"io"
)

type CSBOMV2JSONFormatter struct{}

func (CSBOMV2JSONFormatter) Format(w io.Writer, document Document) error {
	document = withDefaults(document)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	return encoder.Encode(csbomV2Payload(document))
}
