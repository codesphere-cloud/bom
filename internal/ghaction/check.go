package ghaction

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/codesphere-cloud/bom/internal/logging"
)

func (r checkRunner) run() error {
	summaryFormat, err := normalizeCheckSummaryFormat(r.cfg.SummaryFormat)
	if err != nil {
		return err
	}
	r.cfg.SummaryFormat = summaryFormat
	r.logStartup()

	changedPaths, err := r.resolveChangedPaths(r.cfg.ChangedOnly)
	if err != nil {
		return err
	}

	targets, err := r.resolveTargets()
	if err != nil {
		return err
	}
	logging.LogListDebug(r.logger, "check targets before changed-path filtering", targets)

	filteredTargets := filterCheckTargetsByChangedPaths(targets, changedPaths)
	if len(changedPaths) > 0 && len(filteredTargets) == 0 && len(targets) > 0 {
		r.logger.Infof("changed-path filtering removed all check targets")
	}
	r.logMatchedTargets(filteredTargets)
	changedBOMs := []string(nil)
	if len(changedPaths) > 0 {
		changedBOMs = filteredTargets
	}

	result := Result{
		ChangedPaths:  changedBOMs,
		MatchedPaths:  filteredTargets,
		SummaryFormat: summaryFormat,
	}

	if len(filteredTargets) == 0 {
		if r.cfg.FailOnNoMatches {
			return errors.New("no matching paths to process")
		}
		return r.finish("check", result)
	}

	result.ProcessedPaths, result.Failures = r.runTargets(filteredTargets)
	finishErr := r.finish("check", result)
	validationErr := newCheckFailuresError(result.Failures)
	if finishErr != nil {
		return fmt.Errorf("write check summary: %w", finishErr)
	}
	if validationErr != nil {
		return validationErr
	}
	return finishErr
}

func (r checkRunner) logStartup() {
	logging.LogTable(r.logger, "starting check",
		logging.TableRow{Label: "repository root source", Value: r.repoRootSource},
		logging.TableRow{Label: "repository root", Value: r.repoRoot},
		logging.TableRow{Label: "changed only", Value: strconv.FormatBool(r.cfg.ChangedOnly)},
		logging.TableRow{Label: "fail on no matches", Value: strconv.FormatBool(r.cfg.FailOnNoMatches)},
		logging.TableRow{Label: "summary format", Value: r.cfg.SummaryFormat},
		logging.TableRow{Label: "include paths (raw)", Value: strconv.Quote(r.cfg.IncludePaths)},
		logging.TableRow{Label: "include paths (parsed)", Value: logging.FormatList(r.configuredPaths)},
		logging.TableRow{Label: "exclude paths (raw)", Value: strconv.Quote(r.cfg.ExcludePaths)},
		logging.TableRow{Label: "exclude paths (parsed)", Value: logging.FormatList(r.excludedPaths)},
	)
}

func normalizeCheckSummaryFormat(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "table":
		return "table", nil
	case "yaml", "yml":
		return "yaml", nil
	default:
		return "", fmt.Errorf("unsupported check summary format %q: expected table or yaml", value)
	}
}

func (r checkRunner) resolveTargets() ([]string, error) {
	defaultOnly := len(r.configuredPaths) == 0
	allTargets, err := r.discoverBOMPaths(defaultOnly)
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

func (r checkRunner) runTargets(targets []string) ([]string, []CheckFailure) {
	processed := make([]string, 0, len(targets))
	failures := make([]CheckFailure, 0)
	for _, target := range targets {
		r.logger.Debugf("checking BOM %s", target)
		processed = append(processed, target)
		args := []string{"check", filepath.Join(r.repoRoot, filepath.FromSlash(target))}
		if r.cfg.Debug {
			args = append(args, "--debug")
		}
		if err := r.deps.RunCLI(args, r.stdout, r.logger.Writer()); err != nil {
			r.logger.Debugf("BOM %s failed validation: %v", target, err)
			failures = append(failures, CheckFailure{Path: target, Err: err})
		}
	}
	return processed, failures
}

type checkFailuresError struct {
	failures []CheckFailure
}

func newCheckFailuresError(failures []CheckFailure) error {
	if len(failures) == 0 {
		return nil
	}
	return &checkFailuresError{failures: failures}
}

func IsCheckFailuresError(err error) bool {
	var checkErr *checkFailuresError
	return errors.As(err, &checkErr)
}

func (e *checkFailuresError) Error() string {
	message := fmt.Sprintf("%d BOM file(s) failed validation:", len(e.failures))
	for _, failure := range e.failures {
		message += fmt.Sprintf("\n- %s: %v", failure.Path, failure.Err)
	}
	return message
}

func (e *checkFailuresError) Unwrap() []error {
	errs := make([]error, 0, len(e.failures))
	for _, failure := range e.failures {
		errs = append(errs, failure.Err)
	}
	return errs
}

func (r checkRunner) logMatchedTargets(targets []string) {
	r.logger.Debugf("found %d BOM file(s) to process", len(targets))
	logging.LogListDebug(r.logger, "matched BOM targets", targets)
}

func (r checkRunner) matchesConfiguredTarget(target string) bool {
	for _, pattern := range r.configuredPaths {
		if hasGlob(pattern) {
			if matchesGlob(pattern, target) {
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

func filterCheckTargetsByChangedPaths(targets []string, changedPaths []string) []string {
	if len(changedPaths) == 0 {
		return targets
	}

	changedSet := make(map[string]struct{}, len(changedPaths))
	for _, changedPath := range changedPaths {
		changedSet[changedPath] = struct{}{}
	}

	filtered := make([]string, 0, len(targets))
	for _, target := range targets {
		if _, ok := changedSet[target]; ok {
			filtered = append(filtered, target)
		}
	}

	return filtered
}

func (r checkRunner) excludeTargets(targets []string) ([]string, error) {
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

func (r checkRunner) matchesExcludedTarget(target string) bool {
	for _, pattern := range r.excludedPaths {
		if hasGlob(pattern) {
			if matchesGlob(pattern, target) {
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
