package bomlint

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBomlint(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "BOM lint config suite")
}

var _ = Describe("Find", func() {
	It("loads the nearest config", func() {
		root := GinkgoT().TempDir()
		configPath := filepath.Join(root, FileName)
		Expect(os.WriteFile(configPath, []byte(`
excludePaths:
  - charts/legacy
allowedRegistries:
  - ghcr.io
  - registry.example.com:5000
`), 0o600)).To(Succeed())

		bomPath := filepath.Join(root, "charts", "api", "bom.yaml")
		Expect(os.MkdirAll(filepath.Dir(bomPath), 0o755)).To(Succeed())
		Expect(os.WriteFile(bomPath, []byte("{}\n"), 0o600)).To(Succeed())

		config, directory, found, err := Find(bomPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(directory).To(Equal(root))
		Expect(slices.Equal(config.ExcludePaths, []string{"charts/legacy"})).To(BeTrue())
		Expect(slices.Equal(config.AllowedRegistries, []string{"ghcr.io", "registry.example.com:5000"})).To(BeTrue())
	})
})

var _ = Describe("Load", func() {
	It("rejects unknown fields", func() {
		directory := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(directory, FileName), []byte("allowedRegistry: ghcr.io\n"), 0o600)).To(Succeed())

		_, _, err := Load(directory)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("BOM exclusions", func() {
	selectors := []string{"boms/legacy", "boms/*/generated.yaml", "./boms/archive/"}

	DescribeTable("matching configured paths",
		func(target string, expected bool) {
			Expect(MatchesExcludedPath(target, selectors)).To(Equal(expected))
		},
		Entry("matches a directory prefix", "boms/legacy/bom.yaml", true),
		Entry("matches a glob", "boms/api/generated.yaml", true),
		Entry("normalizes a relative directory prefix", "boms/archive/bom.json", true),
		Entry("does not match a similar prefix", "boms/legacy-v2/bom.yaml", false),
		Entry("does not match an unrelated path", "boms/api/bom.yaml", false),
	)

	It("resolves relative BOM targets", func() {
		workingDirectory, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		config := Config{ExcludePaths: []string{"fixtures/legacy"}}
		Expect(config.Excludes(workingDirectory, "fixtures/legacy/bom.yaml")).To(BeTrue())
	})
})
