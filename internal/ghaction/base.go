package ghaction

import (
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/codesphere-cloud/bom/internal/bomlint"
)

func (r baseRunner) discoverChartDirs() ([]string, error) {
	targetSet := map[string]struct{}{}
	if err := r.deps.WalkDir(r.repoRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		kind := "file"
		if entry.IsDir() {
			kind = "dir"
		}
		r.logger.Debugf("discoverChartDirs visited %s: %s", kind, toRelativeSlash(r.repoRoot, path))

		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".github":
				r.logger.Debugf("discoverChartDirs skipping directory: %s", toRelativeSlash(r.repoRoot, path))
				return filepath.SkipDir
			}
			return nil
		}

		if entry.Name() != "Chart.yaml" {
			return nil
		}

		r.logger.Debugf("discoverChartDirs found chart manifest: %s", toRelativeSlash(r.repoRoot, path))
		targetSet[toRelativeSlash(r.repoRoot, filepath.Dir(path))] = struct{}{}
		return nil
	}); err != nil {
		return nil, err
	}

	for target := range targetSet {
		if isSubchart(target, targetSet) {
			delete(targetSet, target)
		}
	}

	return sortedKeys(targetSet), nil
}

func isSubchart(target string, chartDirs map[string]struct{}) bool {
	const chartsDir = "/charts/"
	for searchFrom := 0; searchFrom < len(target); {
		relativeIndex := strings.Index(target[searchFrom:], chartsDir)
		if relativeIndex < 0 {
			return false
		}

		index := searchFrom + relativeIndex
		if _, ok := chartDirs[target[:index]]; ok {
			return true
		}
		searchFrom = index + len(chartsDir)
	}
	return false
}

func (r baseRunner) discoverBOMPaths(defaultOnly bool) ([]string, error) {
	targetSet := map[string]struct{}{}
	if err := r.deps.WalkDir(r.repoRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".github":
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() == bomlint.FileName {
			return nil
		}
		if defaultOnly && entry.Name() != "bom.json" {
			return nil
		}

		if !isSupportedBOMFile(path) {
			return nil
		}

		targetSet[toRelativeSlash(r.repoRoot, path)] = struct{}{}
		return nil
	}); err != nil {
		return nil, err
	}

	return sortedKeys(targetSet), nil
}

func (r baseRunner) finish(mode string, result Result) error {
	outputErr := writeOutputs(mode, result, r.deps.WriteOutput)

	var stdoutErr error
	if mode == "check" {
		_, stdoutErr = io.WriteString(r.stdout, renderCheckOutputSummary(result))
	}

	githubSummaryErr := r.deps.WriteSummary(renderGitHubSummary(mode, result))
	r.logger.Infof("completed %s for %d target(s)", mode, len(result.ProcessedPaths))
	return errors.Join(outputErr, stdoutErr, githubSummaryErr)
}
