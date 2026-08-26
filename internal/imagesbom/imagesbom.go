package imagesbom

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/codesphere-cloud/bom/internal/images"
	"github.com/codesphere-cloud/bom/internal/logging"
)

const outputDirectory = "sboms"

type commandRunner func(io.Writer, string, ...string) error

func Generate(logger logging.Logger, refs []images.ImageRef, chartPath string, attest bool) error {
	return generate(logger, refs, chartPath, attest, runExternalCommand)
}

func generate(logger logging.Logger, refs []images.ImageRef, chartPath string, attest bool, run commandRunner) error {
	if len(refs) == 0 {
		return nil
	}

	outputDir := filepath.Join(chartPath, outputDirectory)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create image SBOM directory: %w", err)
	}

	for _, ref := range refs {
		outputPath := filepath.Join(outputDir, fileName(ref.Reference))
		logger.Infof("generating CycloneDX SBOM for image %s -> %s", ref.Reference, outputPath)
		if err := run(logger.Writer(), "trivy", "image", "--format", "cyclonedx", "--output", outputPath, ref.Reference); err != nil {
			_ = os.Remove(outputPath)
			return fmt.Errorf("generate image SBOM for %s: %w", ref.Reference, err)
		}

		if !attest {
			continue
		}

		logger.Infof("attesting CycloneDX SBOM for image %s with keyless Cosign", ref.Reference)
		if err := run(logger.Writer(), "cosign", "attest", "--yes", "--type", "cyclonedx", "--predicate", outputPath, ref.Reference); err != nil {
			return fmt.Errorf("attest image SBOM for %s: %w", ref.Reference, err)
		}
	}

	return nil
}

func fileName(reference string) string {
	digest := sha256.Sum256([]byte(reference))
	return fmt.Sprintf("sbom-%x.cdx.json", digest)
}

func runExternalCommand(output io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return nil
}
