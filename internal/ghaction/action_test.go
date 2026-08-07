package ghaction

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/codesphere-cloud/helm-bom/internal/logging"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type testLogger struct {
	builder strings.Builder
	debug   bool
}

func newTestLogger(debug bool) *testLogger {
	return &testLogger{debug: debug}
}

func (l *testLogger) Infof(format string, args ...any) {
	_, _ = fmt.Fprintf(&l.builder, format+"\n", args...)
}

func (l *testLogger) Debugf(format string, args ...any) {
	if !l.debug {
		return
	}
	l.Infof(format, args...)
}

func (l *testLogger) Writer() io.Writer {
	return io.Discard
}

func (l *testLogger) String() string {
	return l.builder.String()
}

func TestResolveGitRangeForPullRequest(t *testing.T) {
	eventPath := filepath.Join(t.TempDir(), "event.json")
	content := `{"pull_request":{"base":{"sha":"base123"},"head":{"sha":"head456"}}}`
	if err := os.WriteFile(eventPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write event payload: %v", err)
	}

	runner := baseRunner{
		deps: Dependencies{
			ReadFile: os.ReadFile,
			LookupEnv: func(key string) (string, bool) {
				switch key {
				case "GITHUB_EVENT_NAME":
					return "pull_request", true
				case "GITHUB_EVENT_PATH":
					return eventPath, true
				default:
					return "", false
				}
			},
		},
	}

	eventName, gotEventPath, base, head, err := runner.resolveGitRange()
	if err != nil {
		t.Fatalf("resolveGitRange returned error: %v", err)
	}

	if eventName != "pull_request" || gotEventPath != eventPath {
		t.Fatalf("unexpected event metadata: %q %q", eventName, gotEventPath)
	}

	if base != "base123" || head != "head456" {
		t.Fatalf("unexpected range: %s..%s", base, head)
	}
}

func TestResolveRepoRootPrefersGitHubWorkspace(t *testing.T) {
	repoRoot, source, err := resolveRepoRoot(func() (string, error) {
		return "/tmp/cwd", nil
	}, func(key string) (string, bool) {
		if key == "GITHUB_WORKSPACE" {
			return "/github/workspace", true
		}
		return "", false
	})
	if err != nil {
		t.Fatalf("resolveRepoRoot returned error: %v", err)
	}
	if repoRoot != "/github/workspace" {
		t.Fatalf("unexpected repo root: %q", repoRoot)
	}
	if source != "GITHUB_WORKSPACE" {
		t.Fatalf("unexpected repo root source: %q", source)
	}
}

func TestResolveRepoRootFallsBackToGetwd(t *testing.T) {
	repoRoot, source, err := resolveRepoRoot(func() (string, error) {
		return "/tmp/cwd", nil
	}, func(key string) (string, bool) {
		return "", false
	})
	if err != nil {
		t.Fatalf("resolveRepoRoot returned error: %v", err)
	}
	if repoRoot != "/tmp/cwd" {
		t.Fatalf("unexpected repo root: %q", repoRoot)
	}
	if source != "cwd" {
		t.Fatalf("unexpected repo root source: %q", source)
	}
}

func TestChangedPathsBetweenCommits(t *testing.T) {
	repoRoot := t.TempDir()

	repo, err := git.PlainInit(repoRoot, false)
	if err != nil {
		t.Fatalf("PlainInit returned error: %v", err)
	}

	writeFile := func(path string, content string) {
		t.Helper()

		absolutePath := filepath.Join(repoRoot, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
			t.Fatalf("MkdirAll returned error: %v", err)
		}
		if err := os.WriteFile(absolutePath, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile returned error: %v", err)
		}
	}

	commitAll := func(message string, paths ...string) string {
		t.Helper()

		worktree, err := repo.Worktree()
		if err != nil {
			t.Fatalf("Worktree returned error: %v", err)
		}
		for _, path := range paths {
			if _, err := worktree.Add(path); err != nil {
				t.Fatalf("Add(%s) returned error: %v", path, err)
			}
		}

		hash, err := worktree.Commit(message, &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Test",
				Email: "test@example.com",
				When:  time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC),
			},
		})
		if err != nil {
			t.Fatalf("Commit returned error: %v", err)
		}

		return hash.String()
	}

	writeFile("charts/api/Chart.yaml", "name: api\n")
	writeFile("README.md", "before\n")
	baseSHA := commitAll("base", "charts/api/Chart.yaml", "README.md")

	writeFile("charts/api/values.yaml", "replicas: 2\n")
	writeFile("README.md", "after\n")
	headSHA := commitAll("head", "charts/api/values.yaml", "README.md")

	got, err := changedPathsBetweenCommits(context.Background(), repoRoot, baseSHA, headSHA)
	if err != nil {
		t.Fatalf("changedPathsBetweenCommits returned error: %v", err)
	}

	want := []string{"README.md", "charts/api/values.yaml"}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected changed paths:\nwant: %v\ngot:  %v", want, got)
	}
}

func TestGenerateResolveTargetsDiscoversCharts(t *testing.T) {
	repoRoot := t.TempDir()
	for _, path := range []string{
		filepath.Join(repoRoot, "charts", "api", "Chart.yaml"),
		filepath.Join(repoRoot, "charts", "worker", "Chart.yaml"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir chart dir: %v", err)
		}
		if err := os.WriteFile(path, []byte("name: test\n"), 0o600); err != nil {
			t.Fatalf("write chart file: %v", err)
		}
	}

	runner := generateRunner{
		baseRunner: baseRunner{
			repoRoot: repoRoot,
			deps: Dependencies{
				Stat:    os.Stat,
				WalkDir: filepath.WalkDir,
			},
			logger: newTestLogger(false),
		},
	}

	targets, err := runner.resolveTargets()
	if err != nil {
		t.Fatalf("resolveTargets returned error: %v", err)
	}

	want := []string{"charts/api", "charts/worker"}
	if !slices.Equal(targets, want) {
		t.Fatalf("unexpected targets:\nwant: %v\ngot:  %v", want, targets)
	}
}

func TestGenerateResolveTargetsSupportsGlobs(t *testing.T) {
	repoRoot := t.TempDir()
	for _, dir := range []string{
		filepath.Join(repoRoot, "charts", "api"),
		filepath.Join(repoRoot, "charts", "worker"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir chart: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte("name: test\n"), 0o600); err != nil {
			t.Fatalf("write chart file: %v", err)
		}
	}

	runner := generateRunner{
		baseRunner: baseRunner{
			repoRoot:        repoRoot,
			configuredPaths: []string{"charts/*"},
			deps: Dependencies{
				Stat:    os.Stat,
				WalkDir: filepath.WalkDir,
			},
			logger: newTestLogger(false),
		},
	}

	targets, err := runner.resolveTargets()
	if err != nil {
		t.Fatalf("resolveTargets returned error: %v", err)
	}

	want := []string{"charts/api", "charts/worker"}
	if !slices.Equal(targets, want) {
		t.Fatalf("unexpected targets:\nwant: %v\ngot:  %v", want, targets)
	}
}

func TestGenerateResolveTargetsExcludesPaths(t *testing.T) {
	repoRoot := t.TempDir()
	for _, path := range []string{
		filepath.Join(repoRoot, "charts", "api", "Chart.yaml"),
		filepath.Join(repoRoot, "charts", "worker", "Chart.yaml"),
		filepath.Join(repoRoot, "charts", "skip", "Chart.yaml"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir chart dir: %v", err)
		}
		if err := os.WriteFile(path, []byte("name: test\n"), 0o600); err != nil {
			t.Fatalf("write chart file: %v", err)
		}
	}

	runner := generateRunner{
		baseRunner: baseRunner{
			repoRoot:        repoRoot,
			configuredPaths: []string{"charts/*"},
			deps: Dependencies{
				Stat:    os.Stat,
				WalkDir: filepath.WalkDir,
			},
			logger: newTestLogger(false),
		},
		excludedPaths: []string{"charts/worker", "charts/skip/Chart.yaml"},
	}

	targets, err := runner.resolveTargets()
	if err != nil {
		t.Fatalf("resolveTargets returned error: %v", err)
	}

	want := []string{"charts/api"}
	if !slices.Equal(targets, want) {
		t.Fatalf("unexpected excluded generate targets:\nwant: %v\ngot:  %v", want, targets)
	}
}

func TestFilterGenerateTargetsByChangedPaths(t *testing.T) {
	targets := []string{"charts/api", "charts/worker"}
	changed := []string{"charts/api/values.yaml", "README.md"}

	got := filterGenerateTargetsByChangedPaths(targets, changed)
	want := []string{"charts/api"}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected filtered targets:\nwant: %v\ngot:  %v", want, got)
	}
}

func TestCheckResolveTargetsExpandsDirectories(t *testing.T) {
	repoRoot := t.TempDir()
	bomDir := filepath.Join(repoRoot, "boms")
	if err := os.MkdirAll(bomDir, 0o755); err != nil {
		t.Fatalf("mkdir bom dir: %v", err)
	}

	for _, path := range []string{
		filepath.Join(bomDir, "app.json"),
		filepath.Join(bomDir, "worker.yaml"),
	} {
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatalf("write bom file: %v", err)
		}
	}

	runner := checkRunner{
		baseRunner: baseRunner{
			repoRoot:        repoRoot,
			configuredPaths: []string{"boms"},
			deps: Dependencies{
				Stat:    os.Stat,
				WalkDir: filepath.WalkDir,
			},
			logger: newTestLogger(false),
		},
	}

	targets, err := runner.resolveTargets()
	if err != nil {
		t.Fatalf("resolveTargets returned error: %v", err)
	}

	want := []string{"boms/app.json", "boms/worker.yaml"}
	if !slices.Equal(targets, want) {
		t.Fatalf("unexpected check targets:\nwant: %v\ngot:  %v", want, targets)
	}
}

func TestCheckResolveTargetsExcludesPaths(t *testing.T) {
	repoRoot := t.TempDir()
	bomDir := filepath.Join(repoRoot, "boms")
	if err := os.MkdirAll(bomDir, 0o755); err != nil {
		t.Fatalf("mkdir bom dir: %v", err)
	}

	for _, path := range []string{
		filepath.Join(bomDir, "app.json"),
		filepath.Join(bomDir, "worker.yaml"),
		filepath.Join(bomDir, "skip", "nested.yaml"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir bom parent: %v", err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatalf("write bom file: %v", err)
		}
	}

	runner := checkRunner{
		baseRunner: baseRunner{
			repoRoot:        repoRoot,
			configuredPaths: []string{"boms"},
			deps: Dependencies{
				Stat:    os.Stat,
				WalkDir: filepath.WalkDir,
			},
			logger: newTestLogger(false),
		},
		excludedPaths: []string{"boms/worker.yaml", "boms/skip"},
	}

	targets, err := runner.resolveTargets()
	if err != nil {
		t.Fatalf("resolveTargets returned error: %v", err)
	}

	want := []string{"boms/app.json"}
	if !slices.Equal(targets, want) {
		t.Fatalf("unexpected excluded check targets:\nwant: %v\ngot:  %v", want, targets)
	}
}

func TestCheckResolveTargetsDiscoversRepoRootWhenEmpty(t *testing.T) {
	repoRoot := t.TempDir()
	for _, path := range []string{
		filepath.Join(repoRoot, "charts", "api", "bom.json"),
		filepath.Join(repoRoot, "charts", "worker", "bom.yaml"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir bom dir: %v", err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatalf("write bom file: %v", err)
		}
	}

	runner := checkRunner{
		baseRunner: baseRunner{
			repoRoot: repoRoot,
			deps: Dependencies{
				Stat:    os.Stat,
				WalkDir: filepath.WalkDir,
			},
			logger: newTestLogger(false),
		},
	}

	targets, err := runner.resolveTargets()
	if err != nil {
		t.Fatalf("resolveTargets returned error: %v", err)
	}

	want := []string{"charts/api/bom.json", "charts/worker/bom.yaml"}
	if !slices.Equal(targets, want) {
		t.Fatalf("unexpected discovered check targets:\nwant: %v\ngot:  %v", want, targets)
	}
}

func TestBuildGenerateOutputPath(t *testing.T) {
	got := buildGenerateOutputPath("charts/api", "csbom-yaml")
	want := filepath.Join("charts", "api", "bom.yaml")
	if got != want {
		t.Fatalf("unexpected output path:\nwant: %s\ngot:  %s", want, got)
	}
}

func TestBuildGenerateOutputPathCSBOMV2YAML(t *testing.T) {
	got := buildGenerateOutputPath("charts/api", "csbom-v2-yaml")
	want := filepath.Join("charts", "api", "bom.yaml")
	if got != want {
		t.Fatalf("unexpected output path:\nwant: %s\ngot:  %s", want, got)
	}
}

func TestRunGenerateTargetsReportsChangedOutputs(t *testing.T) {
	repoRoot := t.TempDir()
	chartDir := filepath.Join(repoRoot, "charts", "api")
	if err := os.MkdirAll(chartDir, 0o755); err != nil {
		t.Fatalf("mkdir chart dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte("name: api\n"), 0o600); err != nil {
		t.Fatalf("write chart file: %v", err)
	}
	outputPath := filepath.Join(chartDir, "bom.yaml")
	if err := os.WriteFile(outputPath, []byte("before\n"), 0o600); err != nil {
		t.Fatalf("write existing output: %v", err)
	}

	runner := generateRunner{
		baseRunner: baseRunner{
			repoRoot: repoRoot,
			stdout:   io.Discard,
			logger:   newTestLogger(false),
			deps: Dependencies{
				RunCLI: func(args []string, stdout io.Writer, stderr io.Writer) error {
					if len(args) == 0 {
						return fmt.Errorf("missing args")
					}
					return os.WriteFile(outputPath, []byte("after\n"), 0o600)
				},
			},
		},
		cfg: GenerateConfig{Format: "csbom-yaml", Namespace: "default"},
	}

	processed, changedOutputs, changedTargets, err := runner.runTargets([]string{"charts/api"})
	if err != nil {
		t.Fatalf("runTargets returned error: %v", err)
	}

	if !slices.Equal(processed, []string{"charts/api/bom.yaml"}) {
		t.Fatalf("unexpected processed paths: %v", processed)
	}
	if !slices.Equal(changedOutputs, []string{"charts/api/bom.yaml"}) {
		t.Fatalf("unexpected changed outputs: %v", changedOutputs)
	}
	if !slices.Equal(changedTargets, []string{"charts/api"}) {
		t.Fatalf("unexpected changed targets: %v", changedTargets)
	}
}

func TestRunGenerateTargetsReportsUnchangedOutputs(t *testing.T) {
	repoRoot := t.TempDir()
	chartDir := filepath.Join(repoRoot, "charts", "api")
	if err := os.MkdirAll(chartDir, 0o755); err != nil {
		t.Fatalf("mkdir chart dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte("name: api\n"), 0o600); err != nil {
		t.Fatalf("write chart file: %v", err)
	}
	outputPath := filepath.Join(chartDir, "bom.yaml")
	if err := os.WriteFile(outputPath, []byte("stable\n"), 0o600); err != nil {
		t.Fatalf("write existing output: %v", err)
	}

	runner := generateRunner{
		baseRunner: baseRunner{
			repoRoot: repoRoot,
			stdout:   io.Discard,
			logger:   newTestLogger(false),
			deps: Dependencies{
				RunCLI: func(args []string, stdout io.Writer, stderr io.Writer) error {
					return os.WriteFile(outputPath, []byte("stable\n"), 0o600)
				},
			},
		},
		cfg: GenerateConfig{Format: "csbom-yaml", Namespace: "default"},
	}

	processed, changedOutputs, changedTargets, err := runner.runTargets([]string{"charts/api"})
	if err != nil {
		t.Fatalf("runTargets returned error: %v", err)
	}

	if !slices.Equal(processed, []string{"charts/api/bom.yaml"}) {
		t.Fatalf("unexpected processed paths: %v", processed)
	}
	if len(changedOutputs) != 0 {
		t.Fatalf("expected no changed outputs, got %v", changedOutputs)
	}
	if len(changedTargets) != 0 {
		t.Fatalf("expected no changed targets, got %v", changedTargets)
	}
}

func TestRenderSummary(t *testing.T) {
	summary := renderSummary("generate", Result{
		ChangedPaths:        []string{"charts/api/values.yaml"},
		MatchedPaths:        []string{"charts/api"},
		ProcessedPaths:      []string{"charts/api/bom.json"},
		ChangedOutputPaths:  []string{"charts/api/bom.json"},
		AnyProcessedChanged: true,
	})

	for _, fragment := range []string{
		"helm-bom action `generate`",
		"Changed paths: 1",
		"Matched paths: 1",
		"Processed paths: 1",
		"Changed generated outputs: 1",
		"Any processed output changed: true",
	} {
		if !strings.Contains(summary, fragment) {
			t.Fatalf("summary missing %q:\n%s", fragment, summary)
		}
	}
}

func TestLogList(t *testing.T) {
	logger := newTestLogger(false)
	logging.LogList(logger, "changed paths", []string{"charts/api/Chart.yaml", "charts/api/values.yaml"})

	got := logger.String()
	for _, fragment := range []string{
		"changed paths (2):",
		"charts/api/Chart.yaml",
		"charts/api/values.yaml",
	} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("log output missing %q:\n%s", fragment, got)
		}
	}
	if strings.Contains(got, "helm-bom-action:") {
		t.Fatalf("log output still includes old prefix:\n%s", got)
	}
}

func TestGenerateRunnerLogsRepoRootSource(t *testing.T) {
	logger := newTestLogger(false)

	runner := generateRunner{
		baseRunner: baseRunner{
			stdout:         io.Discard,
			repoRoot:       "/github/workspace",
			repoRootSource: "GITHUB_WORKSPACE",
			logger:         logger,
			deps: Dependencies{
				WalkDir: func(root string, fn fs.WalkDirFunc) error {
					return nil
				},
				WriteSummary: func(content string) error {
					return nil
				},
				WriteOutput: func(name string, value string) error {
					return nil
				},
			},
		},
		cfg: GenerateConfig{
			BaseConfig: BaseConfig{},
			Format:     "spdx-json",
			Namespace:  "default",
		},
	}

	if err := runner.run(); err != nil {
		t.Fatalf("runner.run returned error: %v", err)
	}
	if !strings.Contains(logger.String(), "repository root source: GITHUB_WORKSPACE") {
		t.Fatalf("expected startup log to include repo root source, got logs=%s", logger.String())
	}
}
