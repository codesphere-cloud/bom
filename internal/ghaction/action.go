package ghaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/codesphere-cloud/helm-bom/internal/cli"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/merkletrie"
)

type Config struct {
	Mode                         string
	Paths                        string
	ChangedOnly                  bool
	Debug                        bool
	Format                       string
	Namespace                    string
	ReleaseName                  string
	ValidateConfiguredImageExist bool
	RegistryServer               string
	RegistryUsername             string
	RegistryPassword             string
	FailOnNoMatches              bool
}

type Dependencies struct {
	Getwd        func() (string, error)
	ReadFile     func(string) ([]byte, error)
	Stat         func(string) (fs.FileInfo, error)
	WalkDir      func(string, fs.WalkDirFunc) error
	RunCLI       func([]string, io.Writer, io.Writer) error
	RunGitDiff   func(context.Context, string, string, string) ([]string, error)
	LookupEnv    func(string) (string, bool)
	WriteOutput  func(string, string) error
	WriteSummary func(string) error
}

func defaultDependencies() Dependencies {
	return Dependencies{
		Getwd:    os.Getwd,
		ReadFile: os.ReadFile,
		Stat:     os.Stat,
		WalkDir:  filepath.WalkDir,
		RunCLI:   cli.Run,
		RunGitDiff: func(ctx context.Context, repoRoot string, baseSHA string, headSHA string) ([]string, error) {
			return changedPathsBetweenCommits(ctx, repoRoot, baseSHA, headSHA)
		},
		LookupEnv: os.LookupEnv,
		WriteOutput: func(name string, value string) error {
			return appendKeyValueOutput("GITHUB_OUTPUT", name, value)
		},
		WriteSummary: func(content string) error {
			path, ok := os.LookupEnv("GITHUB_STEP_SUMMARY")
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

			_, err = file.WriteString(content)
			return err
		},
	}
}

func changedPathsBetweenCommits(ctx context.Context, repoRoot string, baseSHA string, headSHA string) ([]string, error) {
	repo, err := git.PlainOpen(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("open git repository: %w", err)
	}

	baseCommit, err := repo.CommitObject(plumbing.NewHash(baseSHA))
	if err != nil {
		return nil, fmt.Errorf("load base commit %s: %w", baseSHA, err)
	}

	headCommit, err := repo.CommitObject(plumbing.NewHash(headSHA))
	if err != nil {
		return nil, fmt.Errorf("load head commit %s: %w", headSHA, err)
	}

	baseTree, err := baseCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("load base tree %s: %w", baseSHA, err)
	}

	headTree, err := headCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("load head tree %s: %w", headSHA, err)
	}

	changes, err := object.DiffTreeContext(ctx, baseTree, headTree)
	if err != nil {
		return nil, fmt.Errorf("diff commits %s..%s: %w", baseSHA, headSHA, err)
	}

	changedSet := map[string]struct{}{}
	for _, change := range changes {
		action, err := change.Action()
		if err != nil {
			return nil, fmt.Errorf("resolve change action for %s..%s: %w", baseSHA, headSHA, err)
		}
		if action == merkletrie.Delete {
			continue
		}

		path := ""
		if change.To.Name != "" {
			path = change.To.Name
		} else {
			path = change.From.Name
		}
		if path == "" {
			continue
		}

		changedSet[toSlash(path)] = struct{}{}
	}

	return sortedKeys(changedSet), nil
}

type Result struct {
	ChangedPaths         []string
	MatchedPaths         []string
	ProcessedPaths       []string
	ChangedOutputPaths   []string
	ChangedTargetPaths   []string
	AnyProcessedChanged  bool
}

func Run(ctx context.Context, cfg Config, stdout io.Writer, stderr io.Writer) error {
	return RunWithDependencies(ctx, cfg, stdout, stderr, defaultDependencies())
}

func RunWithDependencies(ctx context.Context, cfg Config, stdout io.Writer, stderr io.Writer, deps Dependencies) error {
	repoRoot, err := resolveRepoRoot(deps.Getwd, deps.LookupEnv)
	if err != nil {
		return err
	}

	logf(stderr, "starting helm-bom action in %q mode", cfg.Mode)
	logf(stderr, "repository root: %s", repoRoot)
	logf(stderr, "config: changed-only=%t format=%q namespace=%q release-name=%q fail-on-no-matches=%t", cfg.ChangedOnly, cfg.Format, cfg.Namespace, cfg.ReleaseName, cfg.FailOnNoMatches)
	logf(stderr, "raw paths input: %q", cfg.Paths)
	configuredPaths := parseList(cfg.Paths)
	logList(stderr, "parsed paths input", configuredPaths)

	if err := maybeLoginRegistry(cfg, stdout, stderr, deps.RunCLI); err != nil {
		return err
	}

	changedPaths := []string(nil)
	if cfg.ChangedOnly {
		logf(stderr, "resolving changed paths from GitHub event")
		eventName, eventPath, baseSHA, headSHA, rangeErr := resolveGitRange(cfg, deps.ReadFile, deps.LookupEnv)
		if rangeErr != nil {
			return rangeErr
		}
		logf(stderr, "resolved git range for event %q from payload %q: %s..%s", eventName, eventPath, baseSHA, headSHA)

		changedPaths, err = deps.RunGitDiff(ctx, repoRoot, baseSHA, headSHA)
		if err != nil {
			return err
		}
		logf(stderr, "found %d changed path(s)", len(changedPaths))
		logList(stderr, "changed paths", changedPaths)
	}

	targets, err := resolveTargets(cfg, repoRoot, configuredPaths, changedPaths, deps.Stat, deps.WalkDir, stderr)
	if err != nil {
		return err
	}
	switch cfg.Mode {
	case "generate":
		logf(stderr, "found %d chart(s) to process", len(targets))
		logList(stderr, "matched chart targets", targets)
	case "check":
		logf(stderr, "found %d BOM file(s) to process", len(targets))
		logList(stderr, "matched BOM targets", targets)
	default:
		logf(stderr, "found %d target path(s) for mode %q", len(targets), cfg.Mode)
		logList(stderr, "matched targets", targets)
	}

	result := Result{
		ChangedPaths: changedPaths,
		MatchedPaths: targets,
	}

	if len(targets) == 0 {
		if cfg.FailOnNoMatches {
			return errors.New("no matching paths to process")
		}
		if err := writeOutputs(result, deps.WriteOutput); err != nil {
			return err
		}
		return deps.WriteSummary(renderSummary(cfg.Mode, result))
	}

	switch cfg.Mode {
	case "generate":
		result.ProcessedPaths, result.ChangedOutputPaths, result.ChangedTargetPaths, err = runGenerateTargets(repoRoot, targets, cfg, stdout, stderr, deps.RunCLI)
	case "check":
		result.ProcessedPaths, err = runCheckTargets(repoRoot, targets, cfg, stdout, stderr, deps.RunCLI)
	default:
		return fmt.Errorf("unsupported mode %q", cfg.Mode)
	}
	if err != nil {
		return err
	}
	result.AnyProcessedChanged = len(result.ChangedOutputPaths) > 0

	if err := writeOutputs(result, deps.WriteOutput); err != nil {
		return err
	}

	logf(stderr, "completed mode %q for %d target(s)", cfg.Mode, len(result.ProcessedPaths))
	return deps.WriteSummary(renderSummary(cfg.Mode, result))
}

func resolveRepoRoot(getwd func() (string, error), lookupEnv func(string) (string, bool)) (string, error) {
	if workspace, ok := lookupEnv("GITHUB_WORKSPACE"); ok && strings.TrimSpace(workspace) != "" {
		return workspace, nil
	}
	return getwd()
}

func maybeLoginRegistry(cfg Config, stdout io.Writer, stderr io.Writer, runCLI func([]string, io.Writer, io.Writer) error) error {
	if strings.TrimSpace(cfg.RegistryServer) == "" {
		return nil
	}
	logf(stderr, "logging in to registry %s", cfg.RegistryServer)

	args := []string{
		"registry",
		"login",
		cfg.RegistryServer,
		"--username",
		cfg.RegistryUsername,
		"--password",
		cfg.RegistryPassword,
	}
	if cfg.Debug {
		args = append(args, "--debug")
	}

	return runCLI(args, stdout, stderr)
}

func resolveGitRange(cfg Config, readFile func(string) ([]byte, error), lookupEnv func(string) (string, bool)) (string, string, string, string, error) {
	eventName, _ := lookupEnv("GITHUB_EVENT_NAME")
	eventPath, _ := lookupEnv("GITHUB_EVENT_PATH")
	if strings.TrimSpace(eventPath) == "" {
		return "", "", "", "", errors.New("changed-only requires GITHUB_EVENT_PATH")
	}

	content, err := readFile(eventPath)
	if err != nil {
		return "", "", "", "", fmt.Errorf("read GitHub event payload: %w", err)
	}

	var payload githubEvent
	if err := json.Unmarshal(content, &payload); err != nil {
		return "", "", "", "", fmt.Errorf("decode GitHub event payload: %w", err)
	}

	switch eventName {
	case "pull_request", "pull_request_target":
		if payload.PullRequest.Base.SHA == "" || payload.PullRequest.Head.SHA == "" {
			return "", "", "", "", errors.New("pull_request event payload is missing base/head SHAs")
		}
		return eventName, eventPath, payload.PullRequest.Base.SHA, payload.PullRequest.Head.SHA, nil
	case "push":
		if payload.Before == "" || payload.After == "" {
			return "", "", "", "", errors.New("push event payload is missing before/after SHAs")
		}
		return eventName, eventPath, payload.Before, payload.After, nil
	default:
		return "", "", "", "", fmt.Errorf("changed-only is not supported for GitHub event %q", eventName)
	}
}

type githubEvent struct {
	Before      string `json:"before"`
	After       string `json:"after"`
	PullRequest struct {
		Base struct {
			SHA string `json:"sha"`
		} `json:"base"`
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
	} `json:"pull_request"`
}

func resolveTargets(cfg Config, repoRoot string, patterns []string, changedPaths []string, stat func(string) (fs.FileInfo, error), walkDir func(string, fs.WalkDirFunc) error, stderr io.Writer) ([]string, error) {
	switch cfg.Mode {
	case "generate":
		targets, err := resolveGenerateTargets(repoRoot, patterns, stat, walkDir)
		if err != nil {
			return nil, err
		}
		logList(stderr, "generate targets before changed-path filtering", targets)
		filtered := filterGenerateTargetsByChangedPaths(targets, changedPaths)
		if len(changedPaths) > 0 && len(filtered) == 0 && len(targets) > 0 {
			logf(stderr, "changed-path filtering removed all generate targets")
		}
		return filtered, nil
	case "check":
		targets, err := resolveCheckTargets(repoRoot, patterns, stat, walkDir)
		if err != nil {
			return nil, err
		}
		logList(stderr, "check targets before changed-path filtering", targets)
		filtered := filterCheckTargetsByChangedPaths(targets, changedPaths)
		if len(changedPaths) > 0 && len(filtered) == 0 && len(targets) > 0 {
			logf(stderr, "changed-path filtering removed all check targets")
		}
		return filtered, nil
	default:
		return nil, fmt.Errorf("unsupported mode %q", cfg.Mode)
	}
}

func resolveGenerateTargets(repoRoot string, patterns []string, stat func(string) (fs.FileInfo, error), walkDir func(string, fs.WalkDirFunc) error) ([]string, error) {
	if len(patterns) == 0 {
		// No explicit paths means scan the repository for charts.
		return discoverChartDirs(repoRoot, walkDir)
	}

	targetSet := map[string]struct{}{}
	for _, pattern := range patterns {
		matches, err := expandPattern(repoRoot, pattern)
		if err != nil {
			return nil, err
		}

		for _, match := range matches {
			info, err := stat(match)
			if err != nil {
				return nil, err
			}

			target, err := normalizeGenerateTarget(repoRoot, match, info)
			if err != nil {
				return nil, err
			}

			targetSet[target] = struct{}{}
		}
	}

	return sortedKeys(targetSet), nil
}

func resolveCheckTargets(repoRoot string, patterns []string, stat func(string) (fs.FileInfo, error), walkDir func(string, fs.WalkDirFunc) error) ([]string, error) {
	if len(patterns) == 0 {
		return discoverBOMPaths(repoRoot, walkDir)
	}

	targetSet := map[string]struct{}{}
	for _, pattern := range patterns {
		matches, err := expandPattern(repoRoot, pattern)
		if err != nil {
			return nil, err
		}

		for _, match := range matches {
			info, err := stat(match)
			if err != nil {
				return nil, err
			}

			if info.IsDir() {
				if err := walkDir(match, func(path string, entry fs.DirEntry, walkErr error) error {
					if walkErr != nil {
						return walkErr
					}
					if entry.IsDir() {
						return nil
					}
					if !isSupportedBOMFile(path) {
						return nil
					}
					targetSet[toRelativeSlash(repoRoot, path)] = struct{}{}
					return nil
				}); err != nil {
					return nil, err
				}
				continue
			}

			if !isSupportedBOMFile(match) {
				return nil, fmt.Errorf("check target %q is not a supported BOM file", toRelativeSlash(repoRoot, match))
			}

			targetSet[toRelativeSlash(repoRoot, match)] = struct{}{}
		}
	}

	return sortedKeys(targetSet), nil
}

func discoverBOMPaths(repoRoot string, walkDir func(string, fs.WalkDirFunc) error) ([]string, error) {
	targetSet := map[string]struct{}{}
	if err := walkDir(repoRoot, func(path string, entry fs.DirEntry, walkErr error) error {
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

		if !isSupportedBOMFile(path) {
			return nil
		}

		targetSet[toRelativeSlash(repoRoot, path)] = struct{}{}
		return nil
	}); err != nil {
		return nil, err
	}

	return sortedKeys(targetSet), nil
}

func discoverChartDirs(repoRoot string, walkDir func(string, fs.WalkDirFunc) error) ([]string, error) {
	targetSet := map[string]struct{}{}
	if err := walkDir(repoRoot, func(path string, entry fs.DirEntry, walkErr error) error {
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

		if entry.Name() != "Chart.yaml" {
			return nil
		}

		targetSet[toRelativeSlash(repoRoot, filepath.Dir(path))] = struct{}{}
		return nil
	}); err != nil {
		return nil, err
	}

	return sortedKeys(targetSet), nil
}

func normalizeGenerateTarget(repoRoot string, path string, info fs.FileInfo) (string, error) {
	if info.IsDir() {
		return toRelativeSlash(repoRoot, path), nil
	}
	if filepath.Base(path) == "Chart.yaml" {
		return toRelativeSlash(repoRoot, filepath.Dir(path)), nil
	}

	return "", fmt.Errorf("generate target %q must be a chart directory or Chart.yaml", toRelativeSlash(repoRoot, path))
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

func runGenerateTargets(repoRoot string, targets []string, cfg Config, stdout io.Writer, stderr io.Writer, runCLI func([]string, io.Writer, io.Writer) error) ([]string, []string, []string, error) {
	processed := make([]string, 0, len(targets))
	changedOutputs := make([]string, 0, len(targets))
	changedTargets := make([]string, 0, len(targets))
	for _, target := range targets {
		logf(stderr, "generating BOM for chart %s", target)
		relativeOutputPath := buildGenerateOutputPath(target, cfg.Format)
		outputPath := relativeOutputPath
		if !filepath.IsAbs(outputPath) {
			outputPath = filepath.Join(repoRoot, filepath.FromSlash(relativeOutputPath))
		}

		before, existedBefore, err := readFileIfExists(outputPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read existing output for %s: %w", target, err)
		}

		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return nil, nil, nil, fmt.Errorf("create output directory for %s: %w", target, err)
		}

		args := []string{"generate", filepath.Join(repoRoot, filepath.FromSlash(target)), "--format", cfg.Format, "--output", outputPath}
		if cfg.ReleaseName != "" {
			args = append(args, "--release-name", cfg.ReleaseName)
		}
		if cfg.Namespace != "" {
			args = append(args, "--namespace", cfg.Namespace)
		}
		if cfg.ValidateConfiguredImageExist {
			args = append(args, "--validate-configured-image-exists")
		}
		if cfg.Debug {
			args = append(args, "--debug")
		}

		if err := runCLI(args, stdout, stderr); err != nil {
			return nil, nil, nil, fmt.Errorf("generate chart %s: %w", target, err)
		}

		after, _, err := readFileIfExists(outputPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read generated output for %s: %w", target, err)
		}

		if !existedBefore || !slices.Equal(before, after) {
			logf(stderr, "generated output changed for chart %s -> %s", target, toSlash(relativeOutputPath))
			changedOutputs = append(changedOutputs, toSlash(relativeOutputPath))
			changedTargets = append(changedTargets, target)
		} else {
			logf(stderr, "generated output unchanged for chart %s -> %s", target, toSlash(relativeOutputPath))
		}

		processed = append(processed, toSlash(relativeOutputPath))
	}

	return processed, changedOutputs, changedTargets, nil
}

func runCheckTargets(repoRoot string, targets []string, cfg Config, stdout io.Writer, stderr io.Writer, runCLI func([]string, io.Writer, io.Writer) error) ([]string, error) {
	processed := make([]string, 0, len(targets))
	for _, target := range targets {
		logf(stderr, "checking BOM %s", target)
		args := []string{"check", filepath.Join(repoRoot, filepath.FromSlash(target))}
		if cfg.Debug {
			args = append(args, "--debug")
		}
		if err := runCLI(args, stdout, stderr); err != nil {
			return nil, fmt.Errorf("check BOM %s: %w", target, err)
		}
		processed = append(processed, target)
	}

	return processed, nil
}

func buildGenerateOutputPath(target string, format string) string {
	ext := ".json"
	if format == "csbom" || format == "csbom-yaml" {
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

func logf(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintf(w, "helm-bom-action: "+format+"\n", args...)
}

func logList(w io.Writer, label string, values []string) {
	if w == nil {
		return
	}
	if len(values) == 0 {
		logf(w, "%s: <none>", label)
		return
	}

	const maxItems = 20
	display := values
	if len(display) > maxItems {
		display = display[:maxItems]
	}
	logf(w, "%s (%d): %s", label, len(values), strings.Join(display, ", "))
	if len(values) > maxItems {
		logf(w, "%s: ... %d more", label, len(values)-maxItems)
	}
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
