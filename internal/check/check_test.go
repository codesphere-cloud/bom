package check

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codesphere-cloud/bom/internal/images"
	"github.com/codesphere-cloud/bom/internal/logging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCheck(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "BOM check suite")
}

var _ = Describe("RunWithValidator", func() {
	It("validates the BOM's container images", func() {
		path := filepath.Join(GinkgoT().TempDir(), "bom.yaml")
		Expect(os.WriteFile(path, []byte(`
components:
  chart:
    containerImages:
      ghcr.io/example/api: ghcr.io/example/api:1.2.3
      busybox: busybox:1.36.1
`), 0o600)).To(Succeed())

		calls := make([]string, 0, 2)
		err := RunWithValidator(logging.NewWriterLogger(io.Discard, false), Config{BOMPath: path, BOMFormat: "csbom"}, func(refs []images.ImageRef) error {
			for _, ref := range refs {
				calls = append(calls, ref.Reference)
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Join(calls, ",")).To(Equal("busybox:1.36.1,ghcr.io/example/api:1.2.3"))
	})

	It("enforces allowed registries for images and charts", func() {
		directory := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(directory, ".bomlint.yml"), []byte(`
allowedRegistries:
  - ghcr.io
`), 0o600)).To(Succeed())

		path := filepath.Join(directory, "bom.yaml")
		Expect(os.WriteFile(path, []byte(`
version: "2"
name: chart
helmCharts:
  dependency:
    ref: oci://registry.example.com/charts/dependency:2.0.0
containerImages:
  api:
    ref: ghcr.io/example/api:1.2.3
  worker:
    ref: quay.io/example/worker:3.0.0
`), 0o600)).To(Succeed())

		validatorCalled := false
		err := RunWithValidator(logging.NewWriterLogger(io.Discard, false), Config{BOMPath: path}, func(_ []images.ImageRef) error {
			validatorCalled = true
			return nil
		})
		Expect(err).To(HaveOccurred())
		Expect(validatorCalled).To(BeFalse())
		Expect(err.Error()).To(ContainSubstring("quay.io/example/worker:3.0.0"))
		Expect(err.Error()).To(ContainSubstring("registry.example.com/charts/dependency:2.0.0"))
	})

	It("validates image and helm chart existence", func() {
		directory := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(directory, ".bomlint.yml"), []byte(`
allowedRegistries:
  - ghcr.io
`), 0o600)).To(Succeed())

		path := filepath.Join(directory, "bom.yaml")
		Expect(os.WriteFile(path, []byte(`
version: "2"
name: chart
helmCharts:
  dependency:
    ref: oci://ghcr.io/example/charts/dependency:2.0.0
containerImages:
  api:
    ref: ghcr.io/example/api:1.2.3
`), 0o600)).To(Succeed())

		var got []string
		err := RunWithValidator(logging.NewWriterLogger(io.Discard, false), Config{BOMPath: path}, func(refs []images.ImageRef) error {
			for _, ref := range refs {
				got = append(got, ref.Reference)
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Join(got, ",")).To(Equal("ghcr.io/example/api:1.2.3,ghcr.io/example/charts/dependency:2.0.0"))
	})
})

var _ = Describe("BOM exclusions", func() {
	DescribeTable("skipping BOMs excluded by .bomlint.yml",
		func(selector string) {
			directory := GinkgoT().TempDir()
			Expect(os.WriteFile(
				filepath.Join(directory, ".bomlint.yml"),
				[]byte("excludePaths:\n  - "+selector+"\n"),
				0o600,
			)).To(Succeed())

			bomPath := filepath.Join(directory, "boms", "legacy", "bom.yaml")
			Expect(os.MkdirAll(filepath.Dir(bomPath), 0o755)).To(Succeed())
			Expect(os.WriteFile(bomPath, []byte("not a valid BOM\n"), 0o600)).To(Succeed())

			validatorCalled := false
			err := RunWithValidator(logging.NewWriterLogger(io.Discard, false), Config{BOMPath: bomPath}, func(_ []images.ImageRef) error {
				validatorCalled = true
				return nil
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(validatorCalled).To(BeFalse())
		},
		Entry("by exact path", "boms/legacy/bom.yaml"),
		Entry("by directory prefix", "boms/legacy"),
		Entry("by glob", "boms/*/bom.yaml"),
		Entry("by normalized relative prefix", "./boms/legacy/"),
	)
})
