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
	case "csbom", "csbom-json":
		return CSBOMJSONFormatter{}, nil
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
