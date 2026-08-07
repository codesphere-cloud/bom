package ghaction

import (
	"errors"
	"fmt"
	"os"
	"path"
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
		fmt.Sprintf("### bom action `%s`", mode),
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

func canonicalPathSelector(pattern string) string {
	pattern = strings.TrimSpace(toSlash(pattern))
	pattern = strings.TrimSuffix(pattern, "/")
	pattern = strings.TrimSuffix(pattern, "/Chart.yaml")
	return pattern
}

func matchesPathPrefix(target string, selector string) bool {
	selector = canonicalPathSelector(selector)
	if selector == "" {
		return false
	}
	return target == selector || strings.HasPrefix(target, selector+"/")
}

func matchesGlob(pattern string, values ...string) bool {
	glob := toSlash(pattern)
	for _, value := range values {
		matched, err := path.Match(glob, toSlash(value))
		if err == nil && matched {
			return true
		}
	}
	return false
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
