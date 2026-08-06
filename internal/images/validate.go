package images

import (
	"fmt"
	"slices"
	"strings"

	"github.com/google/go-containerregistry/pkg/crane"
)

type MissingImageError struct {
	References []string
	Causes     []error
}

func (e *MissingImageError) Error() string {
	parts := make([]string, 0, len(e.References))
	for idx, ref := range e.References {
		parts = append(parts, fmt.Sprintf("%s (%v)", ref, e.Causes[idx]))
	}

	return fmt.Sprintf("%d image reference(s) could not be resolved: %s", len(e.References), strings.Join(parts, ", "))
}

func ValidateReferencesExist(refs []ImageRef) error {
	return ValidateReferencesExistWithLookup(refs, func(ref string) error {
		_, err := crane.Get(ref)
		return err
	})
}

func ValidateReferencesExistWithLookup(refs []ImageRef, lookup func(string) error) error {
	unique := make(map[string]struct{}, len(refs))
	references := make([]string, 0, len(refs))
	for _, ref := range refs {
		if _, exists := unique[ref.Reference]; exists {
			continue
		}

		unique[ref.Reference] = struct{}{}
		references = append(references, ref.Reference)
	}

	slices.Sort(references)

	var missing MissingImageError
	for _, ref := range references {
		if err := lookup(ref); err != nil {
			missing.References = append(missing.References, ref)
			missing.Causes = append(missing.Causes, err)
		}
	}

	if len(missing.References) == 0 {
		return nil
	}

	return &missing
}
