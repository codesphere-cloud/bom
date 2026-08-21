package helm

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHelm(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Helm suite")
}

var _ = Describe("ChartName", func() {
	It("returns the chart name from Chart.yaml", func() {
		dir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte("name: example-chart\nversion: 0.1.0\n"), 0o644)).To(Succeed())

		name, err := ChartName(dir)
		Expect(err).NotTo(HaveOccurred())
		Expect(name).To(Equal("example-chart"))
	})
})

var _ = Describe("EnsureInstalled", func() {
	It("succeeds when helm is installed in the test environment", func() {
		Expect(EnsureInstalled()).NotTo(HaveOccurred())
	})
})
