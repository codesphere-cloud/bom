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

type actionRunner struct {
	ctx             context.Context
	cfg             Config
	deps            Dependencies
	stdout          io.Writer
	stderr          io.Writer
	repoRoot        string
	configuredPaths []string
}

func Run(ctx context.Context, cfg Config, stdout io.Writer, stderr io.Writer) error {
	return RunWithDependencies(ctx, cfg, stdout, stderr, defaultDependencies())
}

func RunWithDependencies(ctx context.Context, cfg Config, stdout io.Writer, stderr io.Writer, deps Dependencies) error {
	repoRoot, err := resolveRepoRoot(deps.Getwd, deps.LookupEnv)
	if err != nil {
		return err
	}

	runner := actionRunner{
		ctx:             ctx,
		cfg:             cfg,
		deps:            deps,
		stdout:          stdout,
		stderr:          stderr,
		repoRoot:        repoRoot,
		configuredPaths: parseList(cfg.Paths),
	}
	return runner.run()
}

func resolveRepoRoot(getwd func() (string, error), lookupEnv func(string) (string, bool)) (string, error) {
	if workspace, ok := lookupEnv("GITHUB_WORKSPACE"); ok && strings.TrimSpace(workspace) != "" {
		return workspace, nil
	}
	return getwd()
}

func (r actionRunner) run() error {
	logf(r.stderr, "starting helm-bom action in %q mode", r.cfg.Mode)
	logf(r.stderr, "repository root: %s", r.repoRoot)
	logf(r.stderr, "config: changed-only=%t format=%q namespace=%q release-name=%q fail-on-no-matches=%t", r.cfg.ChangedOnly, r.cfg.Format, r.cfg.Namespace, r.cfg.ReleaseName, r.cfg.FailOnNoMatches)
	logf(r.stderr, "raw paths input: %q", r.cfg.Paths)
	logList(r.stderr, "parsed paths input", r.configuredPaths)

	if err := r.maybeLoginRegistry(); err != nil {
		return err
	}

	changedPaths, err := r.resolveChangedPaths()
	if err != nil {
		return err
	}

	targets, err := r.resolveTargets(changedPaths)
	if err != nil {
		return err
	}
	r.logMatchedTargets(targets)

	result := Result{
		ChangedPaths: changedPaths,
		MatchedPaths: targets,
	}

	if len(targets) == 0 {
		if r.cfg.FailOnNoMatches {
			return errors.New("no matching paths to process")
		}
		return r.finish(result)
	}

	if err := r.runTargets(&result, targets); err != nil {
		return err
	}

	return r.finish(result)
}

func (r actionRunner) maybeLoginRegistry() error {
	if strings.TrimSpace(r.cfg.RegistryServer) == "" {
		return nil
	}
	logf(r.stderr, "logging in to registry %s", r.cfg.RegistryServer)

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

	return r.deps.RunCLI(args, r.stdout, r.stderr)
}

func (r actionRunner) resolveChangedPaths() ([]string, error) {
	if !r.cfg.ChangedOnly {
		return nil, nil
	}

	logf(r.stderr, "resolving changed paths from GitHub event")
	eventName, eventPath, baseSHA, headSHA, err := r.resolveGitRange()
	if err != nil {
		return nil, err
	}
	logf(r.stderr, "resolved git range for event %q from payload %q: %s..%s", eventName, eventPath, baseSHA, headSHA)

	changedPaths, err := r.deps.RunGitDiff(r.ctx, r.repoRoot, baseSHA, headSHA)
	if err != nil {
		return nil, err
	}
	logf(r.stderr, "found %d changed path(s)", len(changedPaths))
	logList(r.stderr, "changed paths", changedPaths)
	return changedPaths, nil
}

func (r actionRunner) resolveGitRange() (string, string, string, string, error) {
	eventName, _ := r.deps.LookupEnv("GITHUB_EVENT_NAME")
	eventPath, _ := r.deps.LookupEnv("GITHUB_EVENT_PATH")
	if strings.TrimSpace(eventPath) == "" {
		return "", "", "", "", errors.New("changed-only requires GITHUB_EVENT_PATH")
	}

	content, err := r.deps.ReadFile(eventPath)
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

func (r actionRunner) resolveTargets(changedPaths []string) ([]string, error) {
	switch r.cfg.Mode {
	case "generate":
		targets, err := r.resolveGenerateTargets()
		if err != nil {
			return nil, err
		}
		logList(r.stderr, "generate targets before changed-path filtering", targets)
		filtered := filterGenerateTargetsByChangedPaths(targets, changedPaths)
		if len(changedPaths) > 0 && len(filtered) == 0 && len(targets) > 0 {
			logf(r.stderr, "changed-path filtering removed all generate targets")
		}
		return filtered, nil
	case "check":
		targets, err := r.resolveCheckTargets()
		if err != nil {
			return nil, err
		}
		logList(r.stderr, "check targets before changed-path filtering", targets)
		filtered := filterCheckTargetsByChangedPaths(targets, changedPaths)
		if len(changedPaths) > 0 && len(filtered) == 0 && len(targets) > 0 {
			logf(r.stderr, "changed-path filtering removed all check targets")
		}
		return filtered, nil
	default:
		return nil, fmt.Errorf("unsupported mode %q", r.cfg.Mode)
	}
}

func (r actionRunner) resolveGenerateTargets() ([]string, error) {
	if len(r.configuredPaths) == 0 {
		// No explicit paths means scan the repository for charts.
		return r.discoverChartDirs()
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

			target, err := normalizeGenerateTarget(r.repoRoot, match, info)
			if err != nil {
				return nil, err
			}

			targetSet[target] = struct{}{}
		}
	}

	return sortedKeys(targetSet), nil
}

func (r actionRunner) resolveCheckTargets() ([]string, error) {
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
					if entry.IsDir() {
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
				continue
			}

			if !isSupportedBOMFile(match) {
				return nil, fmt.Errorf("check target %q is not a supported BOM file", toRelativeSlash(r.repoRoot, match))
			}

			targetSet[toRelativeSlash(r.repoRoot, match)] = struct{}{}
		}
	}

	return sortedKeys(targetSet), nil
}

func (r actionRunner) discoverBOMPaths() ([]string, error) {
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

func (r actionRunner) discoverChartDirs() ([]string, error) {
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

		if entry.Name() != "Chart.yaml" {
			return nil
		}

		targetSet[toRelativeSlash(r.repoRoot, filepath.Dir(path))] = struct{}{}
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

func (r actionRunner) runGenerateTargets(targets []string) ([]string, []string, []string, error) {
	processed := make([]string, 0, len(targets))
	changedOutputs := make([]string, 0, len(targets))
	changedTargets := make([]string, 0, len(targets))
	for _, target := range targets {
		logf(r.stderr, "generating BOM for chart %s", target)
		relativeOutputPath := buildGenerateOutputPath(target, r.cfg.Format)
		outputPath := relativeOutputPath
		if !filepath.IsAbs(outputPath) {
			outputPath = filepath.Join(r.repoRoot, filepath.FromSlash(relativeOutputPath))
		}

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

		if err := r.deps.RunCLI(args, r.stdout, r.stderr); err != nil {
			return nil, nil, nil, fmt.Errorf("generate chart %s: %w", target, err)
		}

		after, _, err := readFileIfExists(outputPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read generated output for %s: %w", target, err)
		}

		if !existedBefore || !slices.Equal(before, after) {
			logf(r.stderr, "generated output changed for chart %s -> %s", target, toSlash(relativeOutputPath))
			changedOutputs = append(changedOutputs, toSlash(relativeOutputPath))
			changedTargets = append(changedTargets, target)
		} else {
			logf(r.stderr, "generated output unchanged for chart %s -> %s", target, toSlash(relativeOutputPath))
		}

		processed = append(processed, toSlash(relativeOutputPath))
	}

	return processed, changedOutputs, changedTargets, nil
}

func (r actionRunner) runCheckTargets(targets []string) ([]string, error) {
	processed := make([]string, 0, len(targets))
	for _, target := range targets {
		logf(r.stderr, "checking BOM %s", target)
		args := []string{"check", filepath.Join(r.repoRoot, filepath.FromSlash(target))}
		if r.cfg.Debug {
			args = append(args, "--debug")
		}
		if err := r.deps.RunCLI(args, r.stdout, r.stderr); err != nil {
			return nil, fmt.Errorf("check BOM %s: %w", target, err)
		}
		processed = append(processed, target)
	}

	return processed, nil
}

func (r actionRunner) logMatchedTargets(targets []string) {
	switch r.cfg.Mode {
	case "generate":
		logf(r.stderr, "found %d chart(s) to process", len(targets))
		logList(r.stderr, "matched chart targets", targets)
	case "check":
		logf(r.stderr, "found %d BOM file(s) to process", len(targets))
		logList(r.stderr, "matched BOM targets", targets)
	default:
		logf(r.stderr, "found %d target path(s) for mode %q", len(targets), r.cfg.Mode)
		logList(r.stderr, "matched targets", targets)
	}
}

func (r actionRunner) runTargets(result *Result, targets []string) error {
	var err error
	switch r.cfg.Mode {
	case "generate":
		result.ProcessedPaths, result.ChangedOutputPaths, result.ChangedTargetPaths, err = r.runGenerateTargets(targets)
	case "check":
		result.ProcessedPaths, err = r.runCheckTargets(targets)
	default:
		return fmt.Errorf("unsupported mode %q", r.cfg.Mode)
	}
	if err != nil {
		return err
	}
	result.AnyProcessedChanged = len(result.ChangedOutputPaths) > 0
	return nil
}

func (r actionRunner) finish(result Result) error {
	if err := writeOutputs(result, r.deps.WriteOutput); err != nil {
		return err
	}
	logf(r.stderr, "completed mode %q for %d target(s)", r.cfg.Mode, len(result.ProcessedPaths))
	return r.deps.WriteSummary(renderSummary(r.cfg.Mode, result))
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
