package imagesbom

import (
	"fmt"
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
	It("names both SBOM formats with the resolved image digest", func() {
		chartPath := GinkgoT().TempDir()
		refs := []images.ImageRef{
			{Reference: "ghcr.io/example/api:1.0.0"},
			{Reference: "ghcr.io/example/worker:2.0.0"},
		}
		digests := map[string]string{
			refs[0].Reference: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			refs[1].Reference: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		}

		var invocations []commandInvocation
		run := func(_ io.Writer, name string, args ...string) error {
			invocations = append(invocations, commandInvocation{name: name, args: append([]string(nil), args...)})
			Expect(name).To(Equal("trivy"))
			Expect(args).To(HaveLen(6))
			return os.WriteFile(args[4], []byte("{}\n"), 0o600)
		}
		resolveDigest := func(reference string) (string, error) {
			return digests[reference], nil
		}

		results, err := generate(newTestLogger(), refs, chartPath, false, run, resolveDigest)
		Expect(err).NotTo(HaveOccurred())
		Expect(invocations).To(HaveLen(4))
		for idx, ref := range refs {
			digest := digests[ref.Reference]
			cycloneDXPath := filepath.Join(chartPath, outputDirectory, fileName(digest, "cdx.json"))
			spdxJSONPath := filepath.Join(chartPath, outputDirectory, fileName(digest, "spdx.json"))
			Expect(results[ref.Reference]).To(Equal(Result{
				Digest:    digest,
				CycloneDX: SBOM{Path: filepath.ToSlash(filepath.Join(outputDirectory, fileName(digest, "cdx.json")))},
				SPDXJSON:  SBOM{Path: filepath.ToSlash(filepath.Join(outputDirectory, fileName(digest, "spdx.json")))},
			}))
			Expect(cycloneDXPath).To(BeAnExistingFile())
			Expect(spdxJSONPath).To(BeAnExistingFile())

			Expect(invocations[idx*2 : idx*2+2]).To(Equal([]commandInvocation{
				{name: "trivy", args: []string{"image", "--format", "cyclonedx", "--output", cycloneDXPath, ref.Reference}},
				{name: "trivy", args: []string{"image", "--format", "spdx-json", "--output", spdxJSONPath, ref.Reference}},
			}))
		}
	})

	It("attests both digest-named SBOM formats with keyless Cosign signing", func() {
		chartPath := GinkgoT().TempDir()
		ref := images.ImageRef{Reference: "ghcr.io/example/api:1.0.0"}
		digest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		digestRef := "ghcr.io/example/api@" + digest
		cycloneDXPath := filepath.Join(chartPath, outputDirectory, fileName(digest, "cdx.json"))
		spdxJSONPath := filepath.Join(chartPath, outputDirectory, fileName(digest, "spdx.json"))

		var invocations []commandInvocation
		run := func(_ io.Writer, name string, args ...string) error {
			invocations = append(invocations, commandInvocation{name: name, args: append([]string(nil), args...)})
			if name == "trivy" {
				return os.WriteFile(args[4], []byte("{}\n"), 0o600)
			}
			return nil
		}
		resolveDigest := func(string) (string, error) { return digest, nil }

		results, err := generate(newTestLogger(), []images.ImageRef{ref}, chartPath, true, run, resolveDigest)
		Expect(err).NotTo(HaveOccurred())
		Expect(invocations).To(HaveLen(4))
		Expect(invocations).To(Equal([]commandInvocation{
			{
				name: "trivy",
				args: []string{"image", "--format", "cyclonedx", "--output", cycloneDXPath, ref.Reference},
			},
			{
				name: "cosign",
				args: []string{"attest", "--yes", "--type", "cyclonedx", "--predicate", cycloneDXPath, digestRef},
			},
			{
				name: "trivy",
				args: []string{"image", "--format", "spdx-json", "--output", spdxJSONPath, ref.Reference},
			},
			{
				name: "cosign",
				args: []string{"attest", "--yes", "--type", "spdxjson", "--predicate", spdxJSONPath, digestRef},
			},
		}))
		Expect(results[ref.Reference]).To(Equal(Result{
			Digest: digest,
			CycloneDX: SBOM{
				Path:   filepath.ToSlash(filepath.Join(outputDirectory, fileName(digest, "cdx.json"))),
				Cosign: true,
			},
			SPDXJSON: SBOM{
				Path:   filepath.ToSlash(filepath.Join(outputDirectory, fileName(digest, "spdx.json"))),
				Cosign: true,
			},
		}))
	})

	It("uses the resolved digest when the input is already digest-qualified", func() {
		oldDigest := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		newDigest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

		result, err := imageDigestReference("ghcr.io/example/api@"+oldDigest, newDigest)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal("ghcr.io/example/api@" + newDigest))
	})

	It("stops before generating SBOMs when resolving the digest fails", func() {
		ref := images.ImageRef{Reference: "ghcr.io/example/missing:1.0.0"}
		resolveErr := fmt.Errorf("manifest unknown")
		run := func(io.Writer, string, ...string) error {
			Fail("Trivy must not run when the digest cannot be resolved")
			return nil
		}
		resolveDigest := func(string) (string, error) { return "", resolveErr }

		_, err := generate(newTestLogger(), []images.ImageRef{ref}, GinkgoT().TempDir(), false, run, resolveDigest)
		Expect(err).To(MatchError(ContainSubstring("resolve image digest for " + ref.Reference)))
		Expect(err).To(MatchError(ContainSubstring(resolveErr.Error())))
	})
})

func newTestLogger() testLogger {
	return testLogger{}
}

type testLogger struct{}

func (testLogger) Infof(string, ...any)  {}
func (testLogger) Debugf(string, ...any) {}
func (testLogger) Writer() io.Writer     { return io.Discard }
