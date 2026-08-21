package generate

import (
	"os"
	"testing"

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
