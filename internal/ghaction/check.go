package ghaction

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/helm-bom/internal/logging"
)

func (r checkRunner) run() error {
	r.logStartup()

	if err := r.loginRegistry(); err != nil {
		return err
	}

	changedPaths, err := r.resolveChangedPaths(r.cfg.ChangedOnly)
	if err != nil {
		return err
	}

	targets, err := r.resolveTargets()
	if err != nil {
		return err
	}
	logging.LogList(r.logger, "check targets before changed-path filtering", targets)

	filteredTargets := filterCheckTargetsByChangedPaths(targets, changedPaths)
	if len(changedPaths) > 0 && len(filteredTargets) == 0 && len(targets) > 0 {
		r.logger.Infof("changed-path filtering removed all check targets")
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
		return r.finish("check", result)
	}

	result.ProcessedPaths, err = r.runTargets(filteredTargets)
	if err != nil {
		return err
	}
	return r.finish("check", result)
}

func (r checkRunner) logStartup() {
	r.logger.Infof("starting check")
	r.logger.Infof("repository root source: %s", r.repoRootSource)
	r.logger.Infof("repository root: %s", r.repoRoot)
	r.logger.Infof("config: changed-only=%t fail-on-no-matches=%t", r.cfg.ChangedOnly, r.cfg.FailOnNoMatches)
	r.logger.Infof("raw include-paths input: %q", r.cfg.IncludePaths)
	logging.LogList(r.logger, "parsed include-paths input", r.configuredPaths)
	r.logger.Infof("raw exclude-paths input: %q", r.cfg.ExcludePaths)
	logging.LogList(r.logger, "parsed exclude-paths input", r.excludedPaths)
}

func (r checkRunner) loginRegistry() error {
	if strings.TrimSpace(r.cfg.RegistryServer) == "" {
		return nil
	}
	r.logger.Infof("logging in to registry %s", r.cfg.RegistryServer)

	args := []string{
		"registry",
		"login",
		r.cfg.RegistryServer,
		"--username",
		r.cfg.RegistryUsername,
		"--password",
		r.cfg.RegistryPassword,
	}
	if r.cfg.Debug {
		args = append(args, "--debug")
	}

	return r.deps.RunCLI(args, r.stdout, r.logger.Writer())
}

func (r checkRunner) resolveTargets() ([]string, error) {
	if len(r.configuredPaths) == 0 {
		return r.discoverBOMPaths()
	}

	targetSet := map[string]struct{}{}
	for _, pattern := range r.configuredPaths {
		matches, err := expandPattern(r.repoRoot, pattern)
		if err != nil {
			return nil, err
		}

		for _, match := range matches {
			info, err := r.deps.Stat(match)
			if err != nil {
				return nil, err
			}

			if info.IsDir() {
				if err := r.deps.WalkDir(match, func(path string, entry fs.DirEntry, walkErr error) error {
					if walkErr != nil {
						return walkErr
					}
					if entry.IsDir() || !isSupportedBOMFile(path) {
						return nil
					}
					targetSet[toRelativeSlash(r.repoRoot, path)] = struct{}{}
					return nil
				}); err != nil {
					return nil, err
				}
				continue
			}

			if !isSupportedBOMFile(match) {
				return nil, fmt.Errorf("check target %q is not a supported BOM file", toRelativeSlash(r.repoRoot, match))
			}
			targetSet[toRelativeSlash(r.repoRoot, match)] = struct{}{}
		}
	}

	targets := sortedKeys(targetSet)
	filteredTargets, err := r.excludeTargets(targets)
	if err != nil {
		return nil, err
	}
	return filteredTargets, nil
}

func (r checkRunner) runTargets(targets []string) ([]string, error) {
	processed := make([]string, 0, len(targets))
	for _, target := range targets {
		r.logger.Infof("checking BOM %s", target)
		args := []string{"check", filepath.Join(r.repoRoot, filepath.FromSlash(target))}
		if r.cfg.Debug {
			args = append(args, "--debug")
		}
		if err := r.deps.RunCLI(args, r.stdout, r.logger.Writer()); err != nil {
			return nil, fmt.Errorf("check BOM %s: %w", target, err)
		}
		processed = append(processed, target)
	}
	return processed, nil
}

func (r checkRunner) logMatchedTargets(targets []string) {
	r.logger.Infof("found %d BOM file(s) to process", len(targets))
	logging.LogList(r.logger, "matched BOM targets", targets)
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

	excludedSet := map[string]struct{}{}
	for _, pattern := range r.excludedPaths {
		matches, err := expandPattern(r.repoRoot, pattern)
		if err != nil {
			return nil, err
		}

		for _, match := range matches {
			info, err := r.deps.Stat(match)
			if err != nil {
				return nil, err
			}

			if info.IsDir() {
				if err := r.deps.WalkDir(match, func(path string, entry fs.DirEntry, walkErr error) error {
					if walkErr != nil {
						return walkErr
					}
					if entry.IsDir() || !isSupportedBOMFile(path) {
						return nil
					}
					excludedSet[toRelativeSlash(r.repoRoot, path)] = struct{}{}
					return nil
				}); err != nil {
					return nil, err
				}
				continue
			}

			if isSupportedBOMFile(match) {
				excludedSet[toRelativeSlash(r.repoRoot, match)] = struct{}{}
			}
		}
	}

	filtered := make([]string, 0, len(targets))
	for _, target := range targets {
		if _, ok := excludedSet[target]; ok {
			continue
		}
		filtered = append(filtered, target)
	}
	return filtered, nil
}
