package sbom

import (
	"fmt"
	"io"
	"time"
)

type Formatter interface {
	Format(w io.Writer, document Document) error
}

func NewFormatter(name string) (Formatter, error) {
	switch name {
	case "spdx", "spdx-json":
		return SPDXJSONFormatter{}, nil
	case "csbom", "csbom-yaml":
		return CSBOMYAMLFormatter{}, nil
	case "csbom-json":
		return CSBOMJSONFormatter{}, nil
	case "csbom-v2", "csbom-v2-yaml":
		return CSBOMV2YAMLFormatter{}, nil
	case "csbom-v2-json":
		return CSBOMV2JSONFormatter{}, nil
	default:
		return nil, fmt.Errorf("unsupported format %q", name)
	}
}

func withDefaults(document Document) Document {
	if document.Metadata.GeneratedAt.IsZero() {
		document.Metadata.GeneratedAt = time.Now().UTC()
	}

	return document
}
