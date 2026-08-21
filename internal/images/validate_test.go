package images

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ValidateReferencesExistWithLookup", func() {
	It("deduplicates and sorts references before looking them up", func() {
		refs := []ImageRef{
			{Reference: "ghcr.io/example/z:1.0.0"},
			{Reference: "ghcr.io/example/a:1.0.0"},
			{Reference: "ghcr.io/example/z:1.0.0"},
		}

		lookups := make([]string, 0, 2)
		err := ValidateReferencesExistWithLookup(refs, func(ref string) error {
			lookups = append(lookups, ref)
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(lookups).To(Equal([]string{"ghcr.io/example/a:1.0.0", "ghcr.io/example/z:1.0.0"}))
	})

	It("returns the missing images", func() {
		refs := []ImageRef{
			{Reference: "ghcr.io/example/a:1.0.0"},
			{Reference: "ghcr.io/example/b:2.0.0"},
		}

		err := ValidateReferencesExistWithLookup(refs, func(ref string) error {
			if ref == "ghcr.io/example/b:2.0.0" {
				return errors.New("manifest unknown")
			}

			return nil
		})
		Expect(err).To(HaveOccurred())

		var missing *MissingImageError
		Expect(errors.As(err, &missing)).To(BeTrue())
		Expect(missing.References[0]).To(Equal("ghcr.io/example/b:2.0.0"))
	})
})
