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

var _ = Describe("expectNoImages", func() {
	const withImage = `
version: "2"
name: chart
containerImages:
  api:
    ref: ghcr.io/example/api:1.2.3
`
	const chartsOnly = `
version: "2"
name: chart
helmCharts:
  dependency:
    ref: oci://ghcr.io/example/charts/dependency:2.0.0
`

	run := func(bom string, bomrc string) (error, bool) {
		directory := GinkgoT().TempDir()
		path := filepath.Join(directory, "bom.json")
		Expect(os.WriteFile(path, []byte(bom), 0o600)).To(Succeed())
		if bomrc != "" {
			Expect(os.WriteFile(filepath.Join(directory, ".bomrc.yml"), []byte(bomrc), 0o600)).To(Succeed())
		}

		validatorCalled := false
		err := RunWithValidator(logging.NewWriterLogger(io.Discard, false), Config{BOMPath: path}, func(_ []images.ImageRef) error {
			validatorCalled = true
			return nil
		})
		return err, validatorCalled
	}

	It("fails without calling the registry when a BOM has no images and does not declare it", func() {
		err, validatorCalled := run(chartsOnly, "")
		Expect(err).To(MatchError(ContainSubstring("no container images found")))
		Expect(err).To(MatchError(ContainSubstring("same directory as the BOM")))
		Expect(validatorCalled).To(BeFalse())
	})

	It("accepts a BOM without images when .bomrc.yml next to it declares expectNoImages", func() {
		err, _ := run(chartsOnly, "expectNoImages: true\n")
		Expect(err).NotTo(HaveOccurred())
	})

	It("reads the .bomrc.yaml file name next to the BOM", func() {
		directory := GinkgoT().TempDir()
		path := filepath.Join(directory, "bom.json")
		Expect(os.WriteFile(path, []byte(chartsOnly), 0o600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(directory, ".bomrc.yaml"), []byte("expectNoImages: true\n"), 0o600)).To(Succeed())

		Expect(RunWithValidator(logging.NewWriterLogger(io.Discard, false), Config{BOMPath: path}, func(_ []images.ImageRef) error { return nil })).To(Succeed())
	})

	It("accepts a BOM with images when none are declared", func() {
		err, validatorCalled := run(withImage, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(validatorCalled).To(BeTrue())
	})

	It("fails when a BOM with images sits next to a .bomrc.yml declaring expectNoImages", func() {
		err, _ := run(withImage, "expectNoImages: true\n")
		Expect(err).To(MatchError(ContainSubstring("expectNoImages is set")))
	})

	It("fails when the .bomrc.yml next to the BOM is malformed", func() {
		err, _ := run(withImage, "expectNoImages: [\n")
		Expect(err).To(MatchError(ContainSubstring("parse")))
	})

	It("does not require the declaration for BOMs excluded by .bomlint.yml", func() {
		directory := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(directory, ".bomlint.yml"), []byte("excludePaths:\n  - bom.json\n"), 0o600)).To(Succeed())
		path := filepath.Join(directory, "bom.json")
		Expect(os.WriteFile(path, []byte(chartsOnly), 0o600)).To(Succeed())

		Expect(RunWithValidator(logging.NewWriterLogger(io.Discard, false), Config{BOMPath: path}, func(_ []images.ImageRef) error { return nil })).To(Succeed())
	})
})
