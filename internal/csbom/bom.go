package csbom

// Config represents the Bill of Materials configuration.
type Config struct {
	Components map[string]ComponentConfig `json:"components"`
}

// ComponentConfig represents a component in the BOM.
type ComponentConfig struct {
	ContainerImages map[string]string  `json:"containerImages,omitempty"`
	Files           map[string]FileRef `json:"files,omitempty"`
}

// FileRef represents a file reference in the BOM.
type FileRef struct {
	SrcPath    string   `json:"srcPath,omitempty"`
	SrcUrl     string   `json:"srcUrl,omitempty"`
	Executable bool     `json:"executable,omitempty"`
	Glob       *GlobRef `json:"glob,omitempty"`
	// OciRef is an OCI image reference for a Helm chart, e.g. ghcr.io/org/charts/my-chart:1.0.0
	OciRef string `json:"ociRef,omitempty"`
}

// GlobRef represents a glob-based file reference.
type GlobRef struct {
	Cwd     string   `json:"cwd"`
	Include string   `json:"include"`
	Exclude []string `json:"exclude,omitempty"`
}
