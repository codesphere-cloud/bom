package imagesbom

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/bom/internal/images"
	"github.com/codesphere-cloud/bom/internal/logging"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
)

const outputDirectory = "sboms"

type sbomFormat struct {
	trivy            string
	suffix           string
	cosign           string
	predicateTypeURI string
}

var sbomFormats = []sbomFormat{
	{trivy: "cyclonedx", suffix: "cdx.json", cosign: "cyclonedx", predicateTypeURI: "https://cyclonedx.org/bom"},
	{trivy: "spdx-json", suffix: "spdx.json", cosign: "spdxjson", predicateTypeURI: "https://spdx.dev/Document"},
}

type commandRunner func(io.Writer, string, ...string) error
type digestResolver func(string) (string, error)

type Result struct {
	Digest    string
	CycloneDX SBOM
	SPDXJSON  SBOM
}

type SBOM struct {
	Path   string
	Cosign bool
}

func Generate(logger logging.Logger, refs []images.ImageRef, chartPath string, attest bool, force bool) (map[string]Result, error) {
	return generate(logger, refs, chartPath, attest, force, runExternalCommand, resolveImageDigest)
}

func resolveImageDigest(reference string) (string, error) {
	return crane.Digest(reference)
}

func generate(logger logging.Logger, refs []images.ImageRef, chartPath string, attest bool, force bool, run commandRunner, resolveDigest digestResolver) (map[string]Result, error) {
	results := make(map[string]Result, len(refs))
	if len(refs) == 0 {
		return results, nil
	}

	outputDir := filepath.Join(chartPath, outputDirectory)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create image SBOM directory: %w", err)
	}

	for _, ref := range refs {
		digest := ref.Digest
		if digest == "" {
			var err error
			digest, err = resolveDigest(ref.Reference)
			if err != nil {
				return nil, fmt.Errorf("resolve image digest for %s: %w", ref.Reference, err)
			}
		}

		result, err := generateForImage(logger, ref.Reference, digest, outputDir, attest, force, run)
		if err != nil {
			return nil, err
		}
		results[ref.Reference] = result
	}

	return results, nil
}

func generateForImage(logger logging.Logger, reference string, digest string, outputDir string, attest bool, force bool, run commandRunner) (Result, error) {
	cosignReference := ""
	if attest {
		var err error
		cosignReference, err = imageDigestReference(reference, digest)
		if err != nil {
			return Result{}, fmt.Errorf("build digest reference for %s: %w", reference, err)
		}
	}

	result := Result{Digest: digest}
	for _, format := range sbomFormats {
		name := fileName(digest, format.suffix)
		localPath := filepath.ToSlash(filepath.Join(outputDirectory, name))
		outputPath := filepath.Join(outputDir, name)
		if !force && fileExists(outputPath) {
			logger.Infof("reusing existing %s SBOM for image digest %s -> %s", format.trivy, digest, outputPath)
		} else {
			logger.Infof("generating %s SBOM for image %s -> %s", format.trivy, reference, outputPath)
			if err := run(logger.Writer(), "trivy", "image", "--format", format.trivy, "--output", outputPath, reference); err != nil {
				_ = os.Remove(outputPath)
				return Result{}, fmt.Errorf("generate %s image SBOM for %s: %w", format.trivy, reference, err)
			}
		}

		artifact := SBOM{Path: localPath}
		if attest {
			attestationExists := !force && remoteAttestationExists(run, format.predicateTypeURI, cosignReference)
			if attestationExists {
				logger.Infof("reusing existing %s SBOM attestation for image %s", format.trivy, cosignReference)
			} else {
				logger.Infof("attesting %s SBOM for image %s with keyless Cosign", format.trivy, cosignReference)
				if err := run(logger.Writer(), "cosign", "attest", "--yes", "--type", format.cosign, "--predicate", outputPath, cosignReference); err != nil {
					return Result{}, fmt.Errorf("attest %s image SBOM for %s: %w", format.trivy, reference, err)
				}
			}
			artifact.Cosign = true
		}

		if format.trivy == "cyclonedx" {
			result.CycloneDX = artifact
		} else {
			result.SPDXJSON = artifact
		}
	}

	return result, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func remoteAttestationExists(run commandRunner, predicateType string, reference string) bool {
	return run(io.Discard, "cosign", "download", "attestation", "--predicate-type", predicateType, reference) == nil
}

func imageDigestReference(reference string, digest string) (string, error) {
	parsed, err := name.ParseReference(reference)
	if err != nil {
		return "", err
	}
	return parsed.Context().Digest(digest).Name(), nil
}

func fileName(digest string, suffix string) string {
	return fmt.Sprintf("%s.%s", strings.ReplaceAll(digest, ":", "-"), suffix)
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
