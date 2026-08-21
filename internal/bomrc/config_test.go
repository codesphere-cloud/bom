package bomrc

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBomrc(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "BOM RC config suite")
}

var _ = Describe("Load", func() {
	It("returns an empty config when no config file exists", func() {
		cfg, err := Load(GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.AdditionalImages).To(BeEmpty())
	})

	It("parses additional images from the config file", func() {
		chartPath := GinkgoT().TempDir()
		content := []byte(`
additionalImages:
  - key: external
    image: ghcr.io/example/external:2.0.0
  - resource:
      apiVersion: v1
      kind: ConfigMap
      name: extra-images
    key: sidecar
    image: .data.sidecar
imageKeyMappings:
  ghcr.io/example/api: api
bomGenerationValues:
  image:
    repository: ghcr.io/example/api
    tag: latest
`)
		Expect(os.WriteFile(filepath.Join(chartPath, fileNames[0]), content, 0o644)).To(Succeed())

		cfg, err := Load(chartPath)
		Expect(err).NotTo(HaveOccurred())

		Expect(cfg.AdditionalImages).To(HaveLen(2))

		direct := cfg.AdditionalImages[0]
		Expect(direct.Key).To(Equal("external"))
		Expect(direct.Image.Literal).To(Equal("ghcr.io/example/external:2.0.0"))
		Expect(direct.Resource).To(Equal(ResourceRef{}))

		got := cfg.AdditionalImages[1]
		Expect(got.Resource.APIVersion).To(Equal("v1"))
		Expect(got.Resource.Kind).To(Equal("ConfigMap"))
		Expect(got.Resource.Name).To(Equal("extra-images"))
		Expect(got.Key).To(Equal("sidecar"))
		Expect(got.Image.Literal).To(Equal(".data.sidecar"))
		Expect(cfg.ImageKeyMappings).To(HaveKeyWithValue("ghcr.io/example/api", "api"))

		imageValues, ok := cfg.BOMGenerationValues["image"].(map[string]any)
		Expect(ok).To(BeTrue(), "expected nested image generation values, got %#v", cfg.BOMGenerationValues["image"])
		Expect(imageValues["repository"]).To(Equal("ghcr.io/example/api"))
		Expect(imageValues["tag"]).To(Equal("latest"))
	})

	It("parses a structured additional image", func() {
		chartPath := GinkgoT().TempDir()
		content := []byte(`
additionalImages:
  - key: external
    image:
      repository: ghcr.io/example/external
      tag: "2.0.0"
      digest: sha256:1234567890123456789012345678901234567890123456789012345678901234
`)
		Expect(os.WriteFile(filepath.Join(chartPath, fileNames[0]), content, 0o644)).To(Succeed())

		cfg, err := Load(chartPath)
		Expect(err).NotTo(HaveOccurred())

		Expect(cfg.AdditionalImages).To(HaveLen(1))

		got := cfg.AdditionalImages[0].Image
		Expect(got.Repository).To(Equal("ghcr.io/example/external"))
		Expect(got.Tag).To(Equal("2.0.0"))
		Expect(got.Digest).To(Equal("sha256:1234567890123456789012345678901234567890123456789012345678901234"))

		ref, ok := got.Ref()
		Expect(ok).To(BeTrue(), "expected Ref() to succeed")
		Expect(ref).To(Equal("ghcr.io/example/external:2.0.0@sha256:1234567890123456789012345678901234567890123456789012345678901234"))
	})

	It("supports the YAML config file name", func() {
		chartPath := GinkgoT().TempDir()
		content := []byte(`
additionalImages:
  - resource:
      apiVersion: v1
      kind: ConfigMap
      name: extra-images
    image: .data.sidecar
`)
		Expect(os.WriteFile(filepath.Join(chartPath, fileNames[1]), content, 0o644)).To(Succeed())

		cfg, err := Load(chartPath)
		Expect(err).NotTo(HaveOccurred())

		Expect(cfg.AdditionalImages).To(HaveLen(1))
	})

	It("prefers .bomrc.yml when both files exist", func() {
		chartPath := GinkgoT().TempDir()
		ymlContent := []byte(`
bomGenerationValues:
  marker: yml
`)
		yamlContent := []byte(`
bomGenerationValues:
  marker: yaml
`)
		Expect(os.WriteFile(filepath.Join(chartPath, fileNames[0]), ymlContent, 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(chartPath, fileNames[1]), yamlContent, 0o644)).To(Succeed())

		cfg, err := Load(chartPath)
		Expect(err).NotTo(HaveOccurred())

		Expect(cfg.BOMGenerationValues["marker"]).To(Equal("yml"), "expected .bomrc.yml to win, got %#v", cfg.BOMGenerationValues["marker"])
	})
})

var _ = Describe("ImageValue", func() {
	It("rejects a value missing both tag and digest", func() {
		var v ImageValue
		err := v.UnmarshalJSON([]byte(`{"repository":"ghcr.io/example/external"}`))
		Expect(err).To(HaveOccurred())
	})

	It("rejects a value missing repository", func() {
		var v ImageValue
		err := v.UnmarshalJSON([]byte(`{"tag":"2.0.0"}`))
		Expect(err).To(HaveOccurred())
	})
})
