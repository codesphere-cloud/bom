package sbom

import (
	"bytes"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"
)

type containerImagesPayload struct {
	Components map[string]struct {
		ContainerImages map[string]string `json:"containerImages"`
	} `json:"components"`
}

type v2Payload struct {
	Version         string `json:"version"`
	Name            string `json:"name"`
	ContainerImages map[string]struct {
		Ref    string `json:"ref"`
		Digest string `json:"digest"`
		SBOMs  *struct {
			CycloneDX *struct {
				Path   string `json:"path"`
				Cosign bool   `json:"cosign"`
			} `json:"cyclonedx"`
			SPDXJSON *struct {
				Path   string `json:"path"`
				Cosign bool   `json:"cosign"`
			} `json:"spdxJson"`
		} `json:"sboms"`
		Sources []string `json:"sources"`
	} `json:"containerImages"`
}

var _ = Describe("NewFormatter", func() {
	DescribeTable("resolving formatter implementations",
		func(format string, expected any) {
			formatter, err := NewFormatter(format)
			Expect(err).NotTo(HaveOccurred())
			Expect(formatter).To(BeAssignableToTypeOf(expected))
		},
		Entry("supports csbom YAML", "csbom", CSBOMYAMLFormatter{}),
		Entry("supports csbom-v2 YAML", "csbom-v2", CSBOMV2YAMLFormatter{}),
	)
})

var _ = Describe("Formatters", func() {
	var document Document

	BeforeEach(func() {
		document = Document{
			Metadata: Metadata{
				Source: SourceMetadata{
					ChartName: "e2e-chart",
				},
			},
			Components: []Component{
				{
					Repository: "ghcr.io/example/api",
					Reference:  "ghcr.io/example/api:1.2.3",
					Digest:     "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
					SBOMs: SBOMs{
						CycloneDX: SBOM{Path: "sboms/api.cdx.json", Cosign: true},
						SPDXJSON:  SBOM{Path: "sboms/api.spdx.json", Cosign: true},
					},
					Evidence: []string{"Deployment/api spec.containers[0]"},
				},
				{
					Repository: "busybox",
					Reference:  "busybox:1.36.1",
				},
			},
		}
	})

	Context("CSBOMJSONFormatter", func() {
		It("uses the internal csbom shape", func() {
			var buffer bytes.Buffer
			formatter := CSBOMJSONFormatter{}
			Expect(formatter.Format(&buffer, document)).To(Succeed())

			var payload containerImagesPayload
			Expect(json.Unmarshal(buffer.Bytes(), &payload)).To(Succeed())

			component, ok := payload.Components["e2e-chart"]
			Expect(ok).To(BeTrue())
			Expect(component.ContainerImages["ghcr.io/example/api"]).To(Equal("ghcr.io/example/api:1.2.3"))
			Expect(component.ContainerImages["busybox"]).To(Equal("busybox:1.36.1"))
		})
	})

	Context("CSBOMYAMLFormatter", func() {
		It("uses the internal csbom shape", func() {
			var buffer bytes.Buffer
			formatter := CSBOMYAMLFormatter{}
			Expect(formatter.Format(&buffer, document)).To(Succeed())

			var payload containerImagesPayload
			Expect(yaml.Unmarshal(buffer.Bytes(), &payload)).To(Succeed())

			component, ok := payload.Components["e2e-chart"]
			Expect(ok).To(BeTrue())
			Expect(component.ContainerImages["ghcr.io/example/api"]).To(Equal("ghcr.io/example/api:1.2.3"))
			Expect(component.ContainerImages["busybox"]).To(Equal("busybox:1.36.1"))
		})
	})

	Context("CSBOMV2JSONFormatter", func() {
		It("uses the internal csbom v2 shape", func() {
			var buffer bytes.Buffer
			formatter := CSBOMV2JSONFormatter{}
			Expect(formatter.Format(&buffer, document)).To(Succeed())

			var payload v2Payload
			Expect(json.Unmarshal(buffer.Bytes(), &payload)).To(Succeed())

			Expect(payload.Version).To(Equal(csbomV2Version))
			Expect(payload.Name).To(Equal("e2e-chart"))
			Expect(payload.ContainerImages["ghcr.io/example/api"].Ref).To(Equal("ghcr.io/example/api:1.2.3"))
			Expect(payload.ContainerImages["ghcr.io/example/api"].Digest).To(Equal("sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"))
			Expect(payload.ContainerImages["ghcr.io/example/api"].SBOMs.CycloneDX.Path).To(Equal("sboms/api.cdx.json"))
			Expect(payload.ContainerImages["ghcr.io/example/api"].SBOMs.CycloneDX.Cosign).To(BeTrue())
			Expect(payload.ContainerImages["ghcr.io/example/api"].SBOMs.SPDXJSON.Path).To(Equal("sboms/api.spdx.json"))
			Expect(payload.ContainerImages["ghcr.io/example/api"].SBOMs.SPDXJSON.Cosign).To(BeTrue())
			Expect(payload.ContainerImages["ghcr.io/example/api"].Sources).To(Equal([]string{"Deployment/api spec.containers[0]"}))
			Expect(payload.ContainerImages["busybox"].Ref).To(Equal("busybox:1.36.1"))
			Expect(payload.ContainerImages["busybox"].Digest).To(BeEmpty())
			Expect(payload.ContainerImages["busybox"].SBOMs).To(BeNil())
		})
	})

	Context("CSBOMV2YAMLFormatter", func() {
		It("uses the internal csbom v2 shape", func() {
			var buffer bytes.Buffer
			formatter := CSBOMV2YAMLFormatter{}
			Expect(formatter.Format(&buffer, document)).To(Succeed())

			var payload v2Payload
			Expect(yaml.Unmarshal(buffer.Bytes(), &payload)).To(Succeed())

			Expect(payload.Version).To(Equal(csbomV2Version))
			Expect(payload.Name).To(Equal("e2e-chart"))
			Expect(payload.ContainerImages["ghcr.io/example/api"].Ref).To(Equal("ghcr.io/example/api:1.2.3"))
			Expect(payload.ContainerImages["ghcr.io/example/api"].Digest).To(Equal("sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"))
			Expect(payload.ContainerImages["ghcr.io/example/api"].SBOMs.CycloneDX.Path).To(Equal("sboms/api.cdx.json"))
			Expect(payload.ContainerImages["ghcr.io/example/api"].SBOMs.CycloneDX.Cosign).To(BeTrue())
			Expect(payload.ContainerImages["ghcr.io/example/api"].SBOMs.SPDXJSON.Path).To(Equal("sboms/api.spdx.json"))
			Expect(payload.ContainerImages["ghcr.io/example/api"].SBOMs.SPDXJSON.Cosign).To(BeTrue())
			Expect(payload.ContainerImages["ghcr.io/example/api"].Sources).To(Equal([]string{"Deployment/api spec.containers[0]"}))
			Expect(payload.ContainerImages["busybox"].Ref).To(Equal("busybox:1.36.1"))
			Expect(payload.ContainerImages["busybox"].Digest).To(BeEmpty())
			Expect(payload.ContainerImages["busybox"].SBOMs).To(BeNil())
		})
	})
})
