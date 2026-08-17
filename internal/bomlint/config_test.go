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

func TestFindLoadsNearestConfig(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, FileName)
	if err := os.WriteFile(configPath, []byte(`
excludePaths:
  - charts/legacy
allowedRegistries:
  - ghcr.io
  - registry.example.com:5000
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	bomPath := filepath.Join(root, "charts", "api", "bom.yaml")
	if err := os.MkdirAll(filepath.Dir(bomPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(bomPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write BOM: %v", err)
	}

	config, directory, found, err := Find(bomPath)
	if err != nil {
		t.Fatalf("Find returned error: %v", err)
	}
	if !found || directory != root {
		t.Fatalf("unexpected config location: found=%t directory=%q", found, directory)
	}
	if !slices.Equal(config.ExcludePaths, []string{"charts/legacy"}) {
		t.Fatalf("unexpected excludes: %#v", config.ExcludePaths)
	}
	if !slices.Equal(config.AllowedRegistries, []string{"ghcr.io", "registry.example.com:5000"}) {
		t.Fatalf("unexpected registries: %#v", config.AllowedRegistries)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, FileName), []byte("allowedRegistry: ghcr.io\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, _, err := Load(directory); err == nil {
		t.Fatal("expected unknown config field to fail")
	}
}

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
