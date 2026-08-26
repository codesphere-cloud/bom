package ghaction

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	generateworkflow "github.com/codesphere-cloud/bom/internal/generate"
	"github.com/codesphere-cloud/bom/internal/logging"
)

func (r generateRunner) run() error {
	r.logStartup()

	changedPaths, err := r.resolveChangedPaths(r.cfg.ChangedOnly)
	if err != nil {
		return err
	}

	targets, err := r.resolveTargets()
	if err != nil {
		return err
	}
	r.logger.Infof("found %d initial chart(s)", len(targets))
	logging.LogList(r.logger, "generate targets before changed-path filtering", targets)

	filteredTargets := filterGenerateTargetsByChangedPaths(targets, changedPaths)
	if len(changedPaths) > 0 && len(filteredTargets) == 0 && len(targets) > 0 {
		r.logger.Infof("changed-path filtering removed all generate targets")
	}
	r.logMatchedTargets(filteredTargets)

	result := Result{
		ChangedPaths: changedPaths,
		MatchedPaths: filteredTargets,
	}

	if len(filteredTargets) == 0 {
		if r.cfg.FailOnNoMatches {
			return errors.New("no matching paths to process")
		}
		return r.finish("generate", result)
	}

	result.ProcessedPaths, result.ChangedOutputPaths, result.ChangedTargetPaths, err = r.runTargets(filteredTargets)
	if err != nil {
		return err
	}
	result.AnyProcessedChanged = len(result.ChangedOutputPaths) > 0
	return r.finish("generate", result)
}

func (r generateRunner) logStartup() {
	logging.LogTable(r.logger, "starting generate",
		logging.TableRow{Label: "repository root source", Value: r.repoRootSource},
		logging.TableRow{Label: "repository root", Value: r.repoRoot},
		logging.TableRow{Label: "changed only", Value: strconv.FormatBool(r.cfg.ChangedOnly)},
		logging.TableRow{Label: "format", Value: strconv.Quote(r.cfg.Format)},
		logging.TableRow{Label: "namespace", Value: strconv.Quote(r.cfg.Namespace)},
		logging.TableRow{Label: "release name", Value: strconv.Quote(r.cfg.ReleaseName)},
		logging.TableRow{Label: "image SBOMs", Value: strconv.FormatBool(r.cfg.SBOM)},
		logging.TableRow{Label: "keyless Cosign attestation", Value: strconv.FormatBool(r.cfg.Cosign)},
		logging.TableRow{Label: "fail on no matches", Value: strconv.FormatBool(r.cfg.FailOnNoMatches)},
		logging.TableRow{Label: "include paths (raw)", Value: strconv.Quote(r.cfg.IncludePaths)},
		logging.TableRow{Label: "include paths (parsed)", Value: logging.FormatList(r.configuredPaths)},
		logging.TableRow{Label: "exclude paths (raw)", Value: strconv.Quote(r.cfg.ExcludePaths)},
		logging.TableRow{Label: "exclude paths (parsed)", Value: logging.FormatList(r.excludedPaths)},
	)
}

func (r generateRunner) resolveTargets() ([]string, error) {
	allTargets, err := r.discoverChartDirs()
	if err != nil {
		return nil, err
	}

	if len(r.configuredPaths) == 0 {
		return r.excludeTargets(allTargets)
	}

	targets := make([]string, 0, len(allTargets))
	for _, target := range allTargets {
		if r.matchesConfiguredTarget(target) {
			targets = append(targets, target)
		}
	}
	filteredTargets, err := r.excludeTargets(targets)
	if err != nil {
		return nil, err
	}
	return filteredTargets, nil
}

func (r generateRunner) runTargets(targets []string) ([]string, []string, []string, error) {
	processed := make([]string, 0, len(targets))
	changedOutputs := make([]string, 0, len(targets))
	changedTargets := make([]string, 0, len(targets))

	for _, target := range targets {
		r.logger.Infof("generating BOM for chart %s", target)
		relativeOutputPath := buildGenerateOutputPath(target, r.cfg.Format)
		outputPath := filepath.Join(r.repoRoot, filepath.FromSlash(relativeOutputPath))

		before, existedBefore, err := readFileIfExists(outputPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read existing output for %s: %w", target, err)
		}

		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return nil, nil, nil, fmt.Errorf("create output directory for %s: %w", target, err)
		}

		cfg := generateworkflow.Config{
			ChartPath:                    filepath.Join(r.repoRoot, filepath.FromSlash(target)),
			ReleaseName:                  r.cfg.ReleaseName,
			Namespace:                    r.cfg.Namespace,
			Format:                       r.cfg.Format,
			OutputPath:                   outputPath,
			Debug:                        r.cfg.Debug,
			SBOM:                         r.cfg.SBOM,
			Cosign:                       r.cfg.Cosign,
			ValidateConfiguredImageExist: r.cfg.ValidateConfiguredImageExist,
		}
		if err := r.deps.GenerateBOM(r.stdout, r.logger, cfg); err != nil {
			return nil, nil, nil, fmt.Errorf("generate chart %s: %w", target, err)
		}

		after, _, err := readFileIfExists(outputPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read generated output for %s: %w", target, err)
		}
		if _, err := r.stdout.Write(after); err != nil {
			return nil, nil, nil, fmt.Errorf("write generated output for %s to stdout: %w", target, err)
		}

		if !existedBefore || !slices.Equal(before, after) {
			r.logger.Infof("generated output changed for chart %s -> %s", target, toSlash(relativeOutputPath))
			changedOutputs = append(changedOutputs, toSlash(relativeOutputPath))
			changedTargets = append(changedTargets, target)
		} else {
			r.logger.Infof("generated output unchanged for chart %s -> %s", target, toSlash(relativeOutputPath))
		}

		processed = append(processed, toSlash(relativeOutputPath))
	}

	return processed, changedOutputs, changedTargets, nil
}

func (r generateRunner) logMatchedTargets(targets []string) {
	r.logger.Infof("found %d chart(s) to process", len(targets))
	logging.LogList(r.logger, "matched chart targets", targets)
}

func (r generateRunner) matchesConfiguredTarget(target string) bool {
	targetChartFile := target + "/Chart.yaml"
	for _, pattern := range r.configuredPaths {
		if hasGlob(pattern) {
			if matchesGlob(pattern, target, targetChartFile) {
				return true
			}
			continue
		}

		if matchesPathPrefix(target, pattern) {
			return true
		}
	}
	return false
}

func filterGenerateTargetsByChangedPaths(targets []string, changedPaths []string) []string {
	if len(changedPaths) == 0 {
		return targets
	}

	filtered := make([]string, 0, len(targets))
	for _, target := range targets {
		prefix := target + "/"
		for _, changedPath := range changedPaths {
			if changedPath == target || strings.HasPrefix(changedPath, prefix) {
				filtered = append(filtered, target)
				break
			}
		}
	}

	return filtered
}

func (r generateRunner) excludeTargets(targets []string) ([]string, error) {
	if len(r.excludedPaths) == 0 {
		return targets, nil
	}

	filtered := make([]string, 0, len(targets))
	for _, target := range targets {
		if r.matchesExcludedTarget(target) {
			continue
		}
		filtered = append(filtered, target)
	}
	return filtered, nil
}

func (r generateRunner) matchesExcludedTarget(target string) bool {
	targetChartFile := target + "/Chart.yaml"
	for _, pattern := range r.excludedPaths {
		if hasGlob(pattern) {
			if matchesGlob(pattern, target, targetChartFile) {
				return true
			}
			continue
		}

		if matchesPathPrefix(target, pattern) {
			return true
		}
	}
	return false
}
