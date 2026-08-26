package imagesbom

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/codesphere-cloud/bom/internal/images"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestImageSBOM(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Image SBOM suite")
}

type commandInvocation struct {
	name string
	args []string
}

var _ = Describe("Generate", func() {
	It("generates and retains one CycloneDX SBOM per image without attesting", func() {
		chartPath := GinkgoT().TempDir()
		refs := []images.ImageRef{
			{Reference: "ghcr.io/example/api:1.0.0"},
			{Reference: "ghcr.io/example/worker@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		}

		var invocations []commandInvocation
		run := func(_ io.Writer, name string, args ...string) error {
			invocations = append(invocations, commandInvocation{name: name, args: append([]string(nil), args...)})
			Expect(name).To(Equal("trivy"))
			Expect(args).To(HaveLen(6))
			return os.WriteFile(args[4], []byte("{}\n"), 0o600)
		}

		Expect(generate(newTestLogger(), refs, chartPath, false, run)).To(Succeed())
		Expect(invocations).To(HaveLen(2))
		for idx, ref := range refs {
			outputPath := filepath.Join(chartPath, outputDirectory, fileName(ref.Reference))
			Expect(invocations[idx]).To(Equal(commandInvocation{
				name: "trivy",
				args: []string{"image", "--format", "cyclonedx", "--output", outputPath, ref.Reference},
			}))
			Expect(outputPath).To(BeAnExistingFile())
		}
	})

	It("attests each generated SBOM with keyless Cosign signing", func() {
		chartPath := GinkgoT().TempDir()
		ref := images.ImageRef{Reference: "ghcr.io/example/api:1.0.0"}
		outputPath := filepath.Join(chartPath, outputDirectory, fileName(ref.Reference))

		var invocations []commandInvocation
		run := func(_ io.Writer, name string, args ...string) error {
			invocations = append(invocations, commandInvocation{name: name, args: append([]string(nil), args...)})
			if name == "trivy" {
				return os.WriteFile(args[4], []byte("{}\n"), 0o600)
			}
			return nil
		}

		Expect(generate(newTestLogger(), []images.ImageRef{ref}, chartPath, true, run)).To(Succeed())
		Expect(invocations).To(Equal([]commandInvocation{
			{
				name: "trivy",
				args: []string{"image", "--format", "cyclonedx", "--output", outputPath, ref.Reference},
			},
			{
				name: "cosign",
				args: []string{"attest", "--yes", "--type", "cyclonedx", "--predicate", outputPath, ref.Reference},
			},
		}))
	})
})

func newTestLogger() testLogger {
	return testLogger{}
}

type testLogger struct{}

func (testLogger) Infof(string, ...any)  {}
func (testLogger) Debugf(string, ...any) {}
func (testLogger) Writer() io.Writer     { return io.Discard }
