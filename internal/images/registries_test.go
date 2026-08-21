package images

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ValidateAllowedRegistries", func() {
	It("checks every reference and reports every disallowed registry", func() {
		refs := []ImageRef{
			{Reference: "ghcr.io/example/api:1.2.3"},
			{Reference: "busybox:1.36.1"},
			{Reference: "quay.io/example/chart:2.0.0"},
		}

		err := ValidateAllowedRegistries(refs, []string{"ghcr.io"})
		var disallowed *DisallowedRegistriesError
		Expect(errors.As(err, &disallowed)).To(BeTrue())
		Expect(disallowed.References).To(HaveLen(2))

		Expect(err.Error()).To(ContainSubstring("docker.io"))
		Expect(err.Error()).To(ContainSubstring("quay.io"))
	})

	It("accepts hosts with ports", func() {
		refs := []ImageRef{
			{Reference: "registry.example.com:5000/team/api:1.0.0"},
			{Reference: "busybox:1.36.1"},
		}

		err := ValidateAllowedRegistries(refs, []string{"registry.example.com:5000", "docker.io"})
		Expect(err).NotTo(HaveOccurred())
	})

	It("accepts repository path prefixes", func() {
		refs := []ImageRef{
			{Reference: "ghcr.io/example/api:1.0.0"},
			{Reference: "ghcr.io/example/platform/worker:1.0.0"},
			{Reference: "registry.example.com:5000/team/api:1.0.0"},
		}

		err := ValidateAllowedRegistries(refs, []string{"ghcr.io/example", "registry.example.com:5000/team"})
		Expect(err).NotTo(HaveOccurred())
	})

	It("matches path prefixes on whole segments only", func() {
		refs := []ImageRef{
			{Reference: "ghcr.io/example/api:1.0.0"},
			{Reference: "ghcr.io/example-other/api:1.0.0"},
			{Reference: "ghcr.io/other/api:1.0.0"},
		}

		err := ValidateAllowedRegistries(refs, []string{"ghcr.io/example"})
		var disallowed *DisallowedRegistriesError
		Expect(errors.As(err, &disallowed)).To(BeTrue())
		Expect(disallowed.References).To(HaveLen(2))
		Expect(err.Error()).To(ContainSubstring("ghcr.io/example-other/api:1.0.0"))
		Expect(err.Error()).To(ContainSubstring("ghcr.io/other/api:1.0.0"))
	})

	DescribeTable("rejects invalid path prefixes",
		func(value string) {
			err := ValidateAllowedRegistries(nil, []string{value})
			Expect(err).To(HaveOccurred())
		},
		Entry("a scheme prefix", "https://ghcr.io/example"),
		Entry("a tag suffix", "ghcr.io/example:latest"),
		Entry("a trailing slash", "ghcr.io/example/"),
	)
})
