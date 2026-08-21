package images

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MapRepositories", func() {
	It("applies exact mappings and leaves unmatched refs and the original slice untouched", func() {
		refs := []ImageRef{
			{
				Reference:  "ghcr.io/example/api:1.2.3",
				Repository: "ghcr.io/example/api",
				Tag:        "1.2.3",
			},
			{
				Reference:  "busybox:1.36",
				Repository: "busybox",
				Tag:        "1.36",
			},
		}

		got := MapRepositories(refs, map[string]string{
			"ghcr.io/example/api": "api",
		})

		Expect(got[0].Repository).To(Equal("api"))
		Expect(got[1].Repository).To(Equal("busybox"))
		Expect(refs[0].Repository).To(Equal("ghcr.io/example/api"))
	})
})
