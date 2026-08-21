package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type spdxDocument struct {
	Name     string        `json:"name"`
	Packages []spdxPackage `json:"packages"`
}

type spdxPackage struct {
	Name           string            `json:"name"`
	VersionInfo    string            `json:"versionInfo"`
	ExternalRefs   []spdxExternalRef `json:"externalRefs"`
	PrimaryPurpose string            `json:"primaryPackagePurpose"`
}

type spdxExternalRef struct {
	Category string `json:"referenceCategory"`
	Type     string `json:"referenceType"`
	Locator  string `json:"referenceLocator"`
}

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "BOM e2e suite")
}

var _ = Describe("bom", func() {
	It("extracts images from chart", func() {
		root := repoRoot()
		binary := buildBinary(root)
		chartPath := filepath.Join(root, "testdata", "charts", "e2e")
		outputPath := filepath.Join(GinkgoT().TempDir(), "sbom.json")

		cmd := exec.Command(binary, chartPath, "--output", outputPath)
		cmd.Env = os.Environ()
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "run bom: %s", output)

		content, err := os.ReadFile(outputPath)
		Expect(err).NotTo(HaveOccurred(), "read output file")

		var doc spdxDocument
		Expect(json.Unmarshal(content, &doc)).To(Succeed(), "decode SPDX output")

		Expect(doc.Name).To(Equal("e2e-chart"))

		got := make([]string, 0, len(doc.Packages))
		for _, pkg := range doc.Packages {
			got = append(got, pkg.Name+"@"+pkg.VersionInfo)
			Expect(pkg.PrimaryPurpose).To(Equal("CONTAINER"), "expected package %s to have CONTAINER purpose, got %q", pkg.Name, pkg.PrimaryPurpose)
		}
		slices.Sort(got)

		want := []string{
			"ghcr.io/example/external@2.0.0",
			"registry.k8s.io/coredns/coredns@v1.11.3",
			"registry.k8s.io/e2e-test-images/agnhost@2.53",
			"registry.k8s.io/etcd@3.5.15-0",
			"registry.k8s.io/pause@3.10",
		}
		Expect(got).To(Equal(want))

		assertPURL(doc.Packages, "ghcr.io/example/external", "pkg:oci/ghcr.io/example/external@2.0.0")
		assertPURL(doc.Packages, "registry.k8s.io/coredns/coredns", "pkg:oci/registry.k8s.io/coredns/coredns@v1.11.3")
		assertPURL(doc.Packages, "registry.k8s.io/e2e-test-images/agnhost", "pkg:oci/registry.k8s.io/e2e-test-images/agnhost@2.53")
		assertPURL(doc.Packages, "registry.k8s.io/etcd", "pkg:oci/registry.k8s.io/etcd@3.5.15-0")
		assertPURL(doc.Packages, "registry.k8s.io/pause", "pkg:oci/registry.k8s.io/pause@3.10")
	})
})

func buildBinary(repoRoot string) string {
	binaryName := "bom"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}

	binaryPath := filepath.Join(GinkgoT().TempDir(), binaryName)
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/bom")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "build bom binary: %s", output)

	return binaryPath
}

func repoRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	Expect(ok).To(BeTrue(), "resolve caller path")

	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func assertPURL(packages []spdxPackage, name string, want string) {
	for _, pkg := range packages {
		if pkg.Name != name {
			continue
		}

		found := false
		for _, ref := range pkg.ExternalRefs {
			if ref.Category == "PACKAGE-MANAGER" && ref.Type == "purl" && ref.Locator == want {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(), fmt.Sprintf("package %s missing purl %q", name, want))
		return
	}

	Fail(fmt.Sprintf("package %s not found in SPDX output", name))
}
