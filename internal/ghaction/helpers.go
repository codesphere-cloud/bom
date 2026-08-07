package ghaction

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func buildGenerateOutputPath(target string, format string) string {
	ext := ".json"
	if format == "csbom" || format == "csbom-yaml" || format == "csbom-v2" || format == "csbom-v2-yaml" {
		ext = ".yaml"
	}

	return filepath.Join(filepath.FromSlash(target), "bom"+ext)
}

func parseList(raw string) []string {
	raw = strings.ReplaceAll(raw, ",", "\n")
	parts := strings.Split(raw, "\n")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		values = append(values, toSlash(part))
	}
	return values
}

func expandPattern(repoRoot string, pattern string) ([]string, error) {
	if pattern == "" {
		return nil, nil
	}

	absPattern := pattern
	if !filepath.IsAbs(absPattern) {
		absPattern = filepath.Join(repoRoot, filepath.FromSlash(pattern))
	}

	if !hasGlob(absPattern) {
		return []string{absPattern}, nil
	}

	matches, err := filepath.Glob(absPattern)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("path pattern %q did not match any files", pattern)
	}

	return matches, nil
}

func hasGlob(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

func isSupportedBOMFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json", ".yaml", ".yml":
		return true
	default:
		return false
	}
}

func writeOutputs(result Result, writeOutput func(string, string) error) error {
	if err := writeOutput("changed-paths", strings.Join(result.ChangedPaths, "\n")); err != nil {
		return err
	}
	if err := writeOutput("matched-paths", strings.Join(result.MatchedPaths, "\n")); err != nil {
		return err
	}
	if err := writeOutput("processed-paths", strings.Join(result.ProcessedPaths, "\n")); err != nil {
		return err
	}
	if err := writeOutput("changed-output-paths", strings.Join(result.ChangedOutputPaths, "\n")); err != nil {
		return err
	}
	if err := writeOutput("changed-target-paths", strings.Join(result.ChangedTargetPaths, "\n")); err != nil {
		return err
	}
	return writeOutput("any-processed-changed", fmt.Sprintf("%t", result.AnyProcessedChanged))
}

func renderSummary(mode string, result Result) string {
	lines := []string{
		fmt.Sprintf("### helm-bom action `%s`", mode),
		"",
		fmt.Sprintf("- Changed paths: %d", len(result.ChangedPaths)),
		fmt.Sprintf("- Matched paths: %d", len(result.MatchedPaths)),
		fmt.Sprintf("- Processed paths: %d", len(result.ProcessedPaths)),
		fmt.Sprintf("- Changed generated outputs: %d", len(result.ChangedOutputPaths)),
		fmt.Sprintf("- Any processed output changed: %t", result.AnyProcessedChanged),
	}
	return strings.Join(lines, "\n") + "\n"
}

func appendKeyValueOutput(envName string, name string, value string) error {
	path, ok := os.LookupEnv(envName)
	if !ok || path == "" {
		return nil
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()

	_, err = fmt.Fprintf(file, "%s<<EOF\n%s\nEOF\n", name, value)
	return err
}

func readFileIfExists(path string) ([]byte, bool, error) {
	content, err := os.ReadFile(path)
	if err == nil {
		return content, true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	return nil, false, err
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func toRelativeSlash(repoRoot string, path string) string {
	relative, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return toSlash(path)
	}
	return toSlash(relative)
}

func toSlash(path string) string {
	return filepath.ToSlash(path)
}
