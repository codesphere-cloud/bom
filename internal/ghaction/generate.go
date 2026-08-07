package ghaction

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/codesphere-cloud/helm-bom/internal/logging"
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
	r.logger.Infof("starting generate")
	r.logger.Infof("repository root source: %s", r.repoRootSource)
	r.logger.Infof("repository root: %s", r.repoRoot)
	r.logger.Infof(
		"config: changed-only=%t format=%q namespace=%q release-name=%q fail-on-no-matches=%t",
		r.cfg.ChangedOnly,
		r.cfg.Format,
		r.cfg.Namespace,
		r.cfg.ReleaseName,
		r.cfg.FailOnNoMatches,
	)
	r.logger.Infof("raw include-paths input: %q", r.cfg.IncludePaths)
	logging.LogList(r.logger, "parsed include-paths input", r.configuredPaths)
	r.logger.Infof("raw exclude-paths input: %q", r.cfg.ExcludePaths)
	logging.LogList(r.logger, "parsed exclude-paths input", r.excludedPaths)
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

		args := []string{"generate", filepath.Join(r.repoRoot, filepath.FromSlash(target)), "--format", r.cfg.Format, "--output", outputPath}
		if r.cfg.ReleaseName != "" {
			args = append(args, "--release-name", r.cfg.ReleaseName)
		}
		if r.cfg.Namespace != "" {
			args = append(args, "--namespace", r.cfg.Namespace)
		}
		if r.cfg.ValidateConfiguredImageExist {
			args = append(args, "--validate-configured-image-exists")
		}
		if r.cfg.Debug {
			args = append(args, "--debug")
		}

		if err := r.deps.RunCLI(args, r.stdout, r.logger.Writer()); err != nil {
			return nil, nil, nil, fmt.Errorf("generate chart %s: %w", target, err)
		}

		after, _, err := readFileIfExists(outputPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read generated output for %s: %w", target, err)
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
