package generate

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/codesphere-cloud/bom/internal/images"
	"github.com/codesphere-cloud/bom/internal/imagesbom"
	"github.com/codesphere-cloud/bom/internal/logging"
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

var _ = Describe("Run image expectations", func() {
	const configMapTemplate = `apiVersion: v1
kind: ConfigMap
metadata:
  name: settings
data:
  key: value
`
	const deploymentTemplate = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  selector:
    matchLabels:
      app: api
  template:
    metadata:
      labels:
        app: api
    spec:
      containers:
        - name: api
          image: ghcr.io/example/api:1.2.3
`

	newChart := func(template string, bomrc string) string {
		chartPath := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(chartPath, "Chart.yaml"), []byte("apiVersion: v2\nname: demo\nversion: 0.1.0\n"), 0o600)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(chartPath, "templates"), 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(chartPath, "templates", "resource.yaml"), []byte(template), 0o600)).To(Succeed())
		if bomrc != "" {
			Expect(os.WriteFile(filepath.Join(chartPath, ".bomrc.yml"), []byte(bomrc), 0o600)).To(Succeed())
		}
		return chartPath
	}

	run := func(chartPath string) error {
		return Run(io.Discard, logging.NewWriterLogger(io.Discard, false), Config{
			ChartPath: chartPath,
			Format:    "csbom-v2-json",
			Namespace: "default",
		})
	}

	It("fails when a chart yields no images and does not declare it", func() {
		err := run(newChart(configMapTemplate, ""))
		Expect(err).To(MatchError(ContainSubstring("no container images found")))
		Expect(err).To(MatchError(ContainSubstring("bomGenerationValues or additionalImages")))
	})

	It("counts images that only come from additionalImages", func() {
		bomrc := "additionalImages:\n  - key: support-tool\n    image: ghcr.io/example/support-tool:2.1.0\n"
		Expect(run(newChart(configMapTemplate, bomrc))).To(Succeed())
	})

	It("accepts a chart without images that declares expectNoImages", func() {
		Expect(run(newChart(configMapTemplate, "expectNoImages: true\n"))).To(Succeed())
	})

	It("accepts a chart with images that does not declare expectNoImages", func() {
		Expect(run(newChart(deploymentTemplate, ""))).To(Succeed())
	})

	It("fails when a chart with images declares expectNoImages", func() {
		err := run(newChart(deploymentTemplate, "expectNoImages: true\n"))
		Expect(err).To(MatchError(ContainSubstring("expectNoImages is set")))
	})
})
