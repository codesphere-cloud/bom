package ghaction

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/codesphere-cloud/bom/internal/images"
	"sigs.k8s.io/yaml"
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

func writeOutputs(mode string, result Result, writeOutput func(string, string) error) error {
	if mode == "check" {
		return writeCheckOutputs(result, writeOutput)
	}

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

func writeCheckOutputs(result Result, writeOutput func(string, string) error) error {
	if err := writeOutput("changed-boms", strings.Join(result.ChangedPaths, "\n")); err != nil {
		return err
	}
	if err := writeOutput("matched-boms", strings.Join(result.MatchedPaths, "\n")); err != nil {
		return err
	}
	if err := writeOutput("processed-boms", strings.Join(result.ProcessedPaths, "\n")); err != nil {
		return err
	}
	failedBOMs := make([]string, 0, len(result.Failures))
	for _, failure := range result.Failures {
		failedBOMs = append(failedBOMs, failure.Path)
	}
	return writeOutput("failed-boms", strings.Join(failedBOMs, "\n"))
}

func renderGitHubSummary(mode string, result Result) string {
	if mode == "check" {
		return renderGitHubCheckSummary(result)
	}

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

func renderGitHubCheckSummary(result Result) string {
	metrics := collectCheckMetrics(result.Failures)
	lines := []string{
		"### bom action `check`",
		"",
		fmt.Sprintf("- Changed BOMs: %d", len(result.ChangedPaths)),
		fmt.Sprintf("- Matched BOMs: %d", len(result.MatchedPaths)),
		fmt.Sprintf("- Processed BOMs: %d", len(result.ProcessedPaths)),
		fmt.Sprintf("- Failed BOMs: %d", len(result.Failures)),
		fmt.Sprintf("- Images in wrong registry: %d", metrics.WrongRegistryImages),
		fmt.Sprintf("- Images not found: %d", metrics.MissingImages),
	}
	if len(result.ProcessedPaths) > 0 {
		lines = append(lines, "", "#### Check results", "", renderMarkdownCheckResultsTable(result))
	}
	return strings.Join(lines, "\n") + "\n"
}

type yamlCheckSummary struct {
	ChangedBOMs         int               `json:"changedBoms"`
	MatchedBOMs         int               `json:"matchedBoms"`
	ProcessedBOMs       int               `json:"processedBoms"`
	FailedBOMs          int               `json:"failedBoms"`
	WrongRegistryImages int               `json:"wrongRegistryImages"`
	MissingImages       int               `json:"missingImages"`
	Results             []yamlCheckResult `json:"results,omitempty"`
}

type yamlCheckResult struct {
	BOM    string `json:"bom"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func renderCheckSummaryYAML(result Result) string {
	metrics := collectCheckMetrics(result.Failures)
	failuresByPath := make(map[string]error, len(result.Failures))
	for _, failure := range result.Failures {
		failuresByPath[failure.Path] = failure.Err
	}

	summary := yamlCheckSummary{
		ChangedBOMs:         len(result.ChangedPaths),
		MatchedBOMs:         len(result.MatchedPaths),
		ProcessedBOMs:       len(result.ProcessedPaths),
		FailedBOMs:          len(result.Failures),
		WrongRegistryImages: metrics.WrongRegistryImages,
		MissingImages:       metrics.MissingImages,
		Results:             make([]yamlCheckResult, 0, len(result.ProcessedPaths)),
	}
	for _, path := range result.ProcessedPaths {
		checkResult := yamlCheckResult{BOM: path, Status: "passed"}
		if failure, failed := failuresByPath[path]; failed {
			checkResult.Status = "failed"
			checkResult.Error = failure.Error()
		}
		summary.Results = append(summary.Results, checkResult)
	}

	content, err := yaml.Marshal(summary)
	if err != nil {
		return fmt.Sprintf("error: failed to render YAML summary: %v\n", err)
	}
	return string(content)
}

func renderCheckOutputSummary(result Result) string {
	if result.SummaryFormat == "yaml" {
		return renderCheckSummaryYAML(result)
	}
	return renderCheckSummaryTable(result)
}

func renderCheckSummaryTable(result Result) string {
	metrics := collectCheckMetrics(result.Failures)
	failuresByPath := make(map[string]error, len(result.Failures))
	for _, failure := range result.Failures {
		failuresByPath[failure.Path] = failure.Err
	}

	var builder strings.Builder
	table := tabwriter.NewWriter(&builder, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(table, "METRIC\tVALUE")
	_, _ = fmt.Fprintf(table, "Changed BOMs\t%d\n", len(result.ChangedPaths))
	_, _ = fmt.Fprintf(table, "Matched BOMs\t%d\n", len(result.MatchedPaths))
	_, _ = fmt.Fprintf(table, "Processed BOMs\t%d\n", len(result.ProcessedPaths))
	_, _ = fmt.Fprintf(table, "Failed BOMs\t%d\n", len(result.Failures))
	_, _ = fmt.Fprintf(table, "Images in wrong registry\t%d\n", metrics.WrongRegistryImages)
	_, _ = fmt.Fprintf(table, "Images not found\t%d\n", metrics.MissingImages)
	_, _ = fmt.Fprintln(table)
	_, _ = fmt.Fprintln(table, "BOM\tSTATUS\tERROR")
	for _, path := range result.ProcessedPaths {
		status := "passed"
		message := ""
		if failure, failed := failuresByPath[path]; failed {
			status = "failed"
			message = failure.Error()
		}
		_, _ = fmt.Fprintf(table, "%s\t%s\t%s\n", textTableCell(path), status, textTableCell(message))
	}
	_ = table.Flush()
	return builder.String()
}

type checkMetrics struct {
	WrongRegistryImages int
	MissingImages       int
}

func collectCheckMetrics(failures []CheckFailure) checkMetrics {
	var metrics checkMetrics
	for _, failure := range failures {
		var disallowed *images.DisallowedRegistriesError
		if errors.As(failure.Err, &disallowed) {
			metrics.WrongRegistryImages += len(disallowed.References)
		}

		var missing *images.MissingImageError
		if errors.As(failure.Err, &missing) {
			metrics.MissingImages += len(missing.References)
		}
	}
	return metrics
}

func renderMarkdownCheckResultsTable(result Result) string {
	failuresByPath := make(map[string]error, len(result.Failures))
	for _, failure := range result.Failures {
		failuresByPath[failure.Path] = failure.Err
	}

	var builder strings.Builder
	table := tabwriter.NewWriter(&builder, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(table, "| BOM\t| Status\t| Error |")
	_, _ = fmt.Fprintln(table, "| ---\t| ---\t| --- |")
	for _, path := range result.ProcessedPaths {
		status := "passed"
		message := ""
		if failure, failed := failuresByPath[path]; failed {
			status = "failed"
			message = failure.Error()
		}
		_, _ = fmt.Fprintf(
			table,
			"| %s\t| %s\t| %s |\n",
			markdownTableCell(path),
			status,
			markdownTableCell(message),
		)
	}
	_ = table.Flush()
	return strings.TrimSuffix(builder.String(), "\n")
}

func textTableCell(value string) string {
	value = strings.ReplaceAll(value, "\t", " ")
	value = strings.ReplaceAll(value, "\r\n", "; ")
	value = strings.ReplaceAll(value, "\n", "; ")
	return strings.ReplaceAll(value, "\r", "; ")
}

func markdownTableCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\t", " ")
	value = strings.ReplaceAll(value, "\r\n", "<br>")
	value = strings.ReplaceAll(value, "\n", "<br>")
	return strings.ReplaceAll(value, "\r", "<br>")
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
