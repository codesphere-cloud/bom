package images

import (
	"slices"

	"github.com/codesphere-cloud/bom/internal/bomrc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ExtractConfigured", func() {
	It("extracts configured images referenced from a manifest", func() {
		manifest := []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: extra-images
data:
  primary: ghcr.io/acme/primary:1.2.3
  sidecars:
    - name: metrics
      image: ghcr.io/acme/metrics:4.5.6
    - name: logs
      image: ghcr.io/acme/logs:7.8.9
`)

		refs, err := ExtractConfigured(manifest, []bomrc.AdditionalImage{
			{
				Resource: bomrc.ResourceRef{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "extra-images",
				},
				Image: bomrc.ImageValue{Literal: `.data.primary`},
			},
			{
				Resource: bomrc.ResourceRef{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "extra-images",
				},
				Key:   "metrics",
				Image: bomrc.ImageValue{Literal: `.data.sidecars[] | select(.name == "metrics") | .image`},
			},
		}, ExtractConfiguredOptions{})
		Expect(err).NotTo(HaveOccurred())

		got := []string{refs[0].Reference, refs[1].Reference}
		slices.Sort(got)

		want := []string{
			"ghcr.io/acme/metrics:4.5.6",
			"ghcr.io/acme/primary:1.2.3",
		}
		Expect(slices.Equal(got, want)).To(BeTrue())

		for _, ref := range refs {
			if ref.Reference == "ghcr.io/acme/metrics:4.5.6" {
				Expect(ref.Repository).To(Equal("metrics"))
			}
		}
	})

	It("supports direct image references", func() {
		refs, err := ExtractConfigured(nil, []bomrc.AdditionalImage{
			{
				Image: bomrc.ImageValue{Literal: "ghcr.io/acme/external:1.2.3"},
			},
			{
				Key:   "support-tool",
				Image: bomrc.ImageValue{Literal: "docker.io/acme/support-tool:4.5.6"},
			},
		}, ExtractConfiguredOptions{ValidateExists: true})
		Expect(err).NotTo(HaveOccurred())

		Expect(refs).To(HaveLen(2))
		Expect(refs[0].Reference).To(Equal("ghcr.io/acme/external:1.2.3"))
		Expect(refs[0].Repository).To(Equal("ghcr.io/acme/external"))
		Expect(refs[1].Reference).To(Equal("acme/support-tool:4.5.6"))
		Expect(refs[1].Repository).To(Equal("support-tool"))

		Expect(refs[0].Sources).To(HaveLen(1))
		Expect(refs[0].Sources[0]).To(Equal("configured directly by .bomrc.yaml/.yml: ghcr.io/acme/external:1.2.3"))
	})

	It("supports a structured direct image reference", func() {
		refs, err := ExtractConfigured(nil, []bomrc.AdditionalImage{
			{
				Image: bomrc.ImageValue{
					Repository: "ghcr.io/acme/external",
					Tag:        "1.2.3",
				},
			},
		}, ExtractConfiguredOptions{ValidateExists: true})
		Expect(err).NotTo(HaveOccurred())

		Expect(refs).To(HaveLen(1))
		Expect(refs[0].Reference).To(Equal("ghcr.io/acme/external:1.2.3"))
		Expect(refs[0].Repository).To(Equal("ghcr.io/acme/external"))
	})

	It("supports a structured image with literal fields and a resource selector", func() {
		refs, err := ExtractConfigured([]byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: extra-images
data:
  primary: ghcr.io/acme/primary:1.2.3
`), []bomrc.AdditionalImage{
			{
				Resource: bomrc.ResourceRef{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "extra-images",
				},
				Image: bomrc.ImageValue{
					Repository: "ghcr.io/acme/external",
					Tag:        "1.2.3",
				},
			},
		}, ExtractConfiguredOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(refs).To(HaveLen(1))
		Expect(refs[0].Reference).To(Equal("ghcr.io/acme/external:1.2.3"))
	})

	It("supports a structured image with selector fields", func() {
		manifest := []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: extra-images
data:
  tag: 1.2.3
  digest: sha256:1234567890123456789012345678901234567890123456789012345678901234
`)

		refs, err := ExtractConfigured(manifest, []bomrc.AdditionalImage{
			{
				Resource: bomrc.ResourceRef{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "extra-images",
				},
				Image: bomrc.ImageValue{
					Repository: "ghcr.io/acme/external",
					Tag:        ".data.tag",
				},
			},
			{
				Resource: bomrc.ResourceRef{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "extra-images",
				},
				Key: "by-digest",
				Image: bomrc.ImageValue{
					Repository: "ghcr.io/acme/external",
					Digest:     ".data.digest",
				},
			},
		}, ExtractConfiguredOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(refs).To(HaveLen(2))

		Expect(refs[0].Reference).To(Equal("ghcr.io/acme/external:1.2.3"))
		Expect(refs[1].Reference).To(Equal("ghcr.io/acme/external@sha256:1234567890123456789012345678901234567890123456789012345678901234"))
		Expect(refs[1].Repository).To(Equal("by-digest"))
	})

	It("errors when a structured selector field is missing", func() {
		manifest := []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: extra-images
data: {}
`)

		_, err := ExtractConfigured(manifest, []bomrc.AdditionalImage{
			{
				Resource: bomrc.ResourceRef{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "extra-images",
				},
				Image: bomrc.ImageValue{
					Repository: "ghcr.io/acme/external",
					Tag:        ".data.missing",
				},
			},
		}, ExtractConfiguredOptions{ValidateExists: true})
		Expect(err).To(HaveOccurred())
	})

	It("supports arbitrary yq expressions", func() {
		manifest := []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: extra-images
data:
  registry: ghcr.io
  repository: acme/primary
  version: 1.2.3
  sidecars:
    - name: metrics
      enabled: false
      image: ghcr.io/acme/metrics:4.5.6
    - name: logs
      enabled: true
      image: ghcr.io/acme/logs:7.8.9
`)

		refs, err := ExtractConfigured(manifest, []bomrc.AdditionalImage{
			{
				Resource: bomrc.ResourceRef{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "extra-images",
				},
				Image: bomrc.ImageValue{Literal: `.data.registry + "/" + .data.repository + ":" + .data.version`},
			},
			{
				Resource: bomrc.ResourceRef{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "extra-images",
				},
				Key:   "enabled-sidecar",
				Image: bomrc.ImageValue{Literal: `[.data.sidecars[] | select(.enabled) | .image] | .[0]`},
			},
			{
				Resource: bomrc.ResourceRef{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "extra-images",
				},
				Key: "structured",
				Image: bomrc.ImageValue{
					Repository: `.data.repository | sub("acme", "other")`,
					Tag:        `.data.sidecars[] | select(.name == "logs") | .image | split(":") | .[-1]`,
				},
			},
		}, ExtractConfiguredOptions{ValidateExists: true})
		Expect(err).NotTo(HaveOccurred())
		Expect(refs).To(HaveLen(3))

		Expect(refs[0].Reference).To(Equal("ghcr.io/acme/primary:1.2.3"))
		Expect(refs[1].Reference).To(Equal("ghcr.io/acme/logs:7.8.9"))
		Expect(refs[1].Repository).To(Equal("enabled-sidecar"))
		Expect(refs[2].Reference).To(Equal("other/primary:7.8.9"))
	})

	It("errors when an expression is invalid", func() {
		manifest := []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: extra-images
data:
  primary: ghcr.io/acme/primary:1.2.3
`)

		_, err := ExtractConfigured(manifest, []bomrc.AdditionalImage{
			{
				Resource: bomrc.ResourceRef{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "extra-images",
				},
				Image: bomrc.ImageValue{Literal: `.data | select(`},
			},
		}, ExtractConfiguredOptions{})
		Expect(err).To(HaveOccurred())
	})

	It("errors when an expression returns multiple values", func() {
		manifest := []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: extra-images
data:
  sidecars:
    - image: ghcr.io/acme/metrics:4.5.6
    - image: ghcr.io/acme/logs:7.8.9
`)

		_, err := ExtractConfigured(manifest, []bomrc.AdditionalImage{
			{
				Resource: bomrc.ResourceRef{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "extra-images",
				},
				Image: bomrc.ImageValue{Literal: `.data.sidecars[] | .image`},
			},
		}, ExtractConfiguredOptions{})
		Expect(err).To(HaveOccurred())
	})

	It("errors when an expression returns a non-scalar value", func() {
		manifest := []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: extra-images
data:
  primary: ghcr.io/acme/primary:1.2.3
`)

		_, err := ExtractConfigured(manifest, []bomrc.AdditionalImage{
			{
				Resource: bomrc.ResourceRef{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "extra-images",
				},
				Image: bomrc.ImageValue{Literal: `.data`},
			},
		}, ExtractConfiguredOptions{})
		Expect(err).To(HaveOccurred())
	})

	It("rejects an invalid direct image reference", func() {
		_, err := ExtractConfigured(nil, []bomrc.AdditionalImage{
			{Image: bomrc.ImageValue{Literal: "https://example.com/image"}},
		}, ExtractConfiguredOptions{})
		Expect(err).To(HaveOccurred())
	})

	It("rejects a partial resource selector", func() {
		_, err := ExtractConfigured(nil, []bomrc.AdditionalImage{
			{
				Resource: bomrc.ResourceRef{Kind: "ConfigMap"},
				Image:    bomrc.ImageValue{Literal: "ghcr.io/acme/external:1.2.3"},
			},
		}, ExtractConfiguredOptions{})
		Expect(err).To(HaveOccurred())
	})

	It("errors when a selector matches nothing", func() {
		manifest := []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: extra-images
data:
  sidecars:
    - name: metrics
      image: ghcr.io/acme/metrics:4.5.6
`)

		_, err := ExtractConfigured(manifest, []bomrc.AdditionalImage{
			{
				Resource: bomrc.ResourceRef{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "extra-images",
				},
				Image: bomrc.ImageValue{Literal: `.data.sidecars[] | select(.name == "logs") | .image`},
			},
		}, ExtractConfiguredOptions{ValidateExists: true})
		Expect(err).To(HaveOccurred())
	})
})
