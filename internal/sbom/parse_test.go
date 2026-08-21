package sbom

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Parse", func() {
	Context("csbom v1 format", func() {
		It("parses YAML with container images", func() {
			document, err := Parse(strings.NewReader(`
components:
  chart:
    containerImages:
      ghcr.io/example/api: ghcr.io/example/api:1.2.3
      quay.io/example/worker: quay.io/example/worker@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
`), "csbom-yaml")
			Expect(err).NotTo(HaveOccurred())

			got := ImageRefs(document)
			Expect(got).To(HaveLen(2))
			Expect(got[0].Reference).To(Equal("ghcr.io/example/api:1.2.3"))
			Expect(got[1].Reference).To(Equal("quay.io/example/worker@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"))
		})

		It("includes Helm chart OCI refs", func() {
			document, err := Parse(strings.NewReader(`
components:
  chart:
    containerImages:
      ghcr.io/example/api: ghcr.io/example/api:1.2.3
    files:
      dependency:
        ociRef: oci://registry.example.com/charts/dependency:2.0.0
`), "csbom")
			Expect(err).NotTo(HaveOccurred())

			refs := OCIRefs(document)
			Expect(refs).To(HaveLen(2))
			Expect(document.Components[1].Type).To(Equal(ComponentTypeHelmChart))
			Expect(document.Components[1].Reference).To(Equal("registry.example.com/charts/dependency:2.0.0"))
		})

		It("skips Helm chart refs without an OCI scheme", func() {
			document, err := Parse(strings.NewReader(`
components:
  chart:
    containerImages:
      ghcr.io/example/api: ghcr.io/example/api:1.2.3
    files:
      dependency:
        ociRef: registry.example.com/charts/dependency:2.0.0
`), "csbom")
			Expect(err).NotTo(HaveOccurred())
			Expect(document.Components).To(HaveLen(1))
			Expect(document.Components[0].Type).To(Equal(ComponentTypeOCIImage))
		})
	})

	Context("csbom v2 format", func() {
		It("parses YAML with container images and evidence", func() {
			document, err := Parse(strings.NewReader(`
version: "2"
name: chart
containerImages:
  ghcr.io/example/api:
    ref: ghcr.io/example/api:1.2.3
    sources:
      - Deployment/api spec.containers[0]
  quay.io/example/worker:
    ref: quay.io/example/worker@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
`), "csbom-v2-yaml")
			Expect(err).NotTo(HaveOccurred())

			got := ImageRefs(document)
			Expect(got).To(HaveLen(2))
			Expect(got[0].Reference).To(Equal("ghcr.io/example/api:1.2.3"))
			Expect(got[1].Reference).To(Equal("quay.io/example/worker@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"))

			Expect(document.Components[0].Evidence).To(Equal([]string{"Deployment/api spec.containers[0]"}))
		})

		It("parses JSON with container images and evidence", func() {
			document, err := Parse(strings.NewReader(`{
  "version": "2",
  "name": "chart",
  "containerImages": {
    "ghcr.io/example/api": {
      "ref": "ghcr.io/example/api:1.2.3",
      "sources": ["Deployment/api spec.containers[0]"]
    },
    "quay.io/example/worker": {
      "ref": "quay.io/example/worker@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
    }
  }
}`), "csbom-v2-json")
			Expect(err).NotTo(HaveOccurred())

			got := ImageRefs(document)
			Expect(got).To(HaveLen(2))
			Expect(got[0].Reference).To(Equal("ghcr.io/example/api:1.2.3"))
			Expect(got[1].Reference).To(Equal("quay.io/example/worker@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"))

			Expect(document.Components[0].Evidence).To(Equal([]string{"Deployment/api spec.containers[0]"}))
		})

		It("parses without any OCI references", func() {
			document, err := Parse(strings.NewReader(`
version: "2"
name: empty-chart
`), "csbom-v2")
			Expect(err).NotTo(HaveOccurred())

			Expect(document.Components).To(BeEmpty())
			Expect(OCIRefs(document)).To(BeEmpty())
		})

		It("does not treat arbitrary YAML as an empty csbom v2 document", func() {
			_, err := Parse(strings.NewReader(`
replicaCount: 2
image:
  repository: ghcr.io/example/api
`), "csbom-v2")
			Expect(err).To(HaveOccurred())
		})

		It("includes Helm chart OCI refs", func() {
			document, err := Parse(strings.NewReader(`
version: "2"
name: chart
helmCharts:
  dependency:
    ref: oci://registry.example.com/charts/dependency:2.0.0
containerImages:
  ghcr.io/example/api:
    ref: ghcr.io/example/api:1.2.3
`), "csbom-v2")
			Expect(err).NotTo(HaveOccurred())

			refs := OCIRefs(document)
			Expect(refs).To(HaveLen(2))
			Expect(ImageRefs(document)).To(HaveLen(1))
			Expect(document.Components[1].Reference).To(Equal("registry.example.com/charts/dependency:2.0.0"))
		})

		It("skips Helm chart refs without an OCI scheme", func() {
			document, err := Parse(strings.NewReader(`
version: "2"
name: chart
helmCharts:
  dependency:
    ref: registry.example.com/charts/dependency:2.0.0
containerImages:
  ghcr.io/example/api:
    ref: ghcr.io/example/api:1.2.3
`), "csbom-v2")
			Expect(err).NotTo(HaveOccurred())
			Expect(document.Components).To(HaveLen(1))
			Expect(document.Components[0].Type).To(Equal(ComponentTypeOCIImage))
		})
	})

	Context("format selection", func() {
		It("uses only the configured format", func() {
			_, err := Parse(strings.NewReader(`
components:
  chart:
    containerImages:
      ghcr.io/example/api: ghcr.io/example/api:1.2.3
`), "csbom-v2")
			Expect(err).To(HaveOccurred())
		})

		It("rejects an unsupported format", func() {
			_, err := Parse(strings.NewReader("{}"), "auto")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`unsupported bom format "auto"`))
		})
	})

	Context("SPDX format", func() {
		It("parses SPDX JSON", func() {
			document, err := Parse(strings.NewReader(`{
  "spdxVersion": "SPDX-2.3",
  "dataLicense": "CC0-1.0",
  "SPDXID": "SPDXRef-DOCUMENT",
  "name": "chart",
  "documentNamespace": "https://example.com/spdx/test",
  "creationInfo": {
    "created": "2026-08-06T00:00:00Z",
    "creators": ["Tool: bom-dev"]
  },
  "packages": [
    {
      "name": "ghcr.io/example/api",
      "SPDXID": "SPDXRef-Package-001",
      "versionInfo": "1.2.3",
      "downloadLocation": "NOASSERTION",
      "filesAnalyzed": false,
      "summary": "ghcr.io/example/api:1.2.3"
    },
    {
      "name": "quay.io/example/worker",
      "SPDXID": "SPDXRef-Package-002",
      "versionInfo": "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
      "downloadLocation": "NOASSERTION",
      "filesAnalyzed": false,
      "summary": "quay.io/example/worker@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
    }
  ]
}`), "spdx-json")
			Expect(err).NotTo(HaveOccurred())

			got := ImageRefs(document)
			Expect(got).To(HaveLen(2))
			Expect(got[0].Reference).To(Equal("ghcr.io/example/api:1.2.3"))
			Expect(got[1].Reference).To(Equal("quay.io/example/worker@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"))
		})
	})
})
