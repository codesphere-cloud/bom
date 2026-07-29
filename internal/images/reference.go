package images

import (
	"strings"

	"github.com/distribution/reference"
)

type ImageRef struct {
	Reference  string   `json:"reference"`
	Repository string   `json:"repository"`
	Tag        string   `json:"tag,omitempty"`
	Digest     string   `json:"digest,omitempty"`
	Sources    []string `json:"sources,omitempty"`
}

func ParseImageRef(value string) (ImageRef, bool) {
	ref := strings.TrimSpace(strings.Trim(value, `"'`))
	if ref == "" || strings.ContainsAny(ref, " \t\r\n{}") {
		return ImageRef{}, false
	}

	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return ImageRef{}, false
	}

	image := ImageRef{
		Reference:  reference.FamiliarString(named),
		Repository: reference.FamiliarName(named),
	}

	if tagged, ok := named.(reference.NamedTagged); ok {
		image.Tag = tagged.Tag()
	}

	if digested, ok := named.(reference.Canonical); ok {
		image.Digest = digested.Digest().String()
	}

	return image, true
}
