package generate

import (
	"os"
	"testing"

	"github.com/codesphere-cloud/bom/internal/images"
	"github.com/codesphere-cloud/bom/internal/imagesbom"
	"github.com/codesphere-cloud/bom/internal/sbom"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"
)

func TestGenerate(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Generate suite")
}

var _ = Describe("prependBOMGenerationValuesFile", func() {
	It("prepends a values file containing the BOM generation values", func() {
		valuesFiles := []string{"values.yaml"}
		cleanup, err := prependBOMGenerationValuesFile(&valuesFiles, map[string]any{
			"image": map[string]any{
				"repository": "ghcr.io/example/api",
				"tag":        "latest",
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(cleanup).NotTo(BeNil(), "expected cleanup function")

		Expect(valuesFiles).To(HaveLen(2))
		Expect(valuesFiles[1]).To(Equal("values.yaml"), "unexpected values file order")

		content, err := os.ReadFile(valuesFiles[0])
		Expect(err).NotTo(HaveOccurred())

		var payload map[string]any
		Expect(yaml.Unmarshal(content, &payload)).To(Succeed())

		imageValues, ok := payload["image"].(map[string]any)
		Expect(ok).To(BeTrue(), "expected nested image payload, got %#v", payload["image"])
		Expect(imageValues["repository"]).To(Equal("ghcr.io/example/api"))
		Expect(imageValues["tag"]).To(Equal("latest"))

		path := valuesFiles[0]
		cleanup()
		_, err = os.Stat(path)
		Expect(os.IsNotExist(err)).To(BeTrue(), "expected bom generation values file to be removed, stat err=%v", err)
	})
})

var _ = Describe("componentsFromImages", func() {
	It("adds generated image SBOM metadata to matching components", func() {
		digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		ref := images.ImageRef{
			Reference:  "ghcr.io/example/api:1.0.0",
			Repository: "ghcr.io/example/api",
		}
		components := componentsFromImages([]images.ImageRef{ref}, map[string]imagesbom.Result{
			ref.Reference: {
				Digest:    digest,
				CycloneDX: imagesbom.SBOM{Path: "sboms/api.cdx.json", Cosign: true},
				SPDXJSON:  imagesbom.SBOM{Path: "sboms/api.spdx.json", Cosign: true},
			},
		})

		Expect(components).To(HaveLen(1))
		Expect(components[0].Digest).To(Equal(digest))
		Expect(components[0].SBOMs).To(Equal(sbom.SBOMs{
			CycloneDX: sbom.SBOM{Path: "sboms/api.cdx.json", Cosign: true},
			SPDXJSON:  sbom.SBOM{Path: "sboms/api.spdx.json", Cosign: true},
		}))
	})
})
