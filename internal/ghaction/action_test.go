package ghaction

import (
	"bytes"
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

	checkworkflow "github.com/codesphere-cloud/bom/internal/check"
	generateworkflow "github.com/codesphere-cloud/bom/internal/generate"
	"github.com/codesphere-cloud/bom/internal/images"
	"github.com/codesphere-cloud/bom/internal/logging"
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

func TestDefaultWriteSummaryDoesNothingOutsideGitHub(t *testing.T) {
	t.Setenv("GITHUB_STEP_SUMMARY", "")

	deps := defaultDependencies()
	if err := deps.WriteSummary("check summary\n"); err != nil {
		t.Fatalf("WriteSummary returned error: %v", err)
	}
}

func TestDefaultWriteSummaryWritesGitHubSummaryFile(t *testing.T) {
	summaryPath := filepath.Join(t.TempDir(), "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)

	deps := defaultDependencies()
	if err := deps.WriteSummary("check summary\n"); err != nil {
		t.Fatalf("WriteSummary returned error: %v", err)
	}
	content, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary file: %v", err)
	}
	if string(content) != "check summary\n" {
		t.Fatalf("unexpected summary file content: %q", content)
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

func TestGenerateResolveTargetsAppliesExcludesWhenIncludePathsEmpty(t *testing.T) {
	repoRoot := t.TempDir()
	for _, path := range []string{
		filepath.Join(repoRoot, "charts", "api", "Chart.yaml"),
		filepath.Join(repoRoot, "charts", "pc-applications", "Chart.yaml"),
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
		excludedPaths: []string{"charts/pc-applications"},
	}

	targets, err := runner.resolveTargets()
	if err != nil {
		t.Fatalf("resolveTargets returned error: %v", err)
	}

	want := []string{"charts/api", "charts/worker"}
	if !slices.Equal(targets, want) {
		t.Fatalf("unexpected targets with excludes and empty includes:\nwant: %v\ngot:  %v", want, targets)
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

func TestGenerateResolveTargetsSupportsPrefixes(t *testing.T) {
	repoRoot := t.TempDir()
	for _, dir := range []string{
		filepath.Join(repoRoot, "charts", "team-a", "api"),
		filepath.Join(repoRoot, "charts", "team-a", "worker"),
		filepath.Join(repoRoot, "charts", "team-b", "web"),
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
			configuredPaths: []string{"charts/team-a"},
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

	want := []string{"charts/team-a/api", "charts/team-a/worker"}
	if !slices.Equal(targets, want) {
		t.Fatalf("unexpected prefix-matched generate targets:\nwant: %v\ngot:  %v", want, targets)
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

func TestGenerateResolveTargetsExcludesPrefixes(t *testing.T) {
	repoRoot := t.TempDir()
	for _, path := range []string{
		filepath.Join(repoRoot, "charts", "team-a", "api", "Chart.yaml"),
		filepath.Join(repoRoot, "charts", "team-a", "worker", "Chart.yaml"),
		filepath.Join(repoRoot, "charts", "team-b", "web", "Chart.yaml"),
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
			configuredPaths: []string{"charts"},
			deps: Dependencies{
				Stat:    os.Stat,
				WalkDir: filepath.WalkDir,
			},
			logger: newTestLogger(false),
		},
		excludedPaths: []string{"charts/team-a"},
	}

	targets, err := runner.resolveTargets()
	if err != nil {
		t.Fatalf("resolveTargets returned error: %v", err)
	}

	want := []string{"charts/team-b/web"}
	if !slices.Equal(targets, want) {
		t.Fatalf("unexpected prefix-excluded generate targets:\nwant: %v\ngot:  %v", want, targets)
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

func TestCheckResolveTargetsSupportsPrefixes(t *testing.T) {
	repoRoot := t.TempDir()
	for _, path := range []string{
		filepath.Join(repoRoot, "boms", "team-a", "app.json"),
		filepath.Join(repoRoot, "boms", "team-a", "worker.yaml"),
		filepath.Join(repoRoot, "boms", "team-b", "web.yaml"),
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
			configuredPaths: []string{"boms/team-a"},
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

	want := []string{"boms/team-a/app.json", "boms/team-a/worker.yaml"}
	if !slices.Equal(targets, want) {
		t.Fatalf("unexpected prefix-matched check targets:\nwant: %v\ngot:  %v", want, targets)
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

func TestCheckResolveTargetsExcludesPrefixes(t *testing.T) {
	repoRoot := t.TempDir()
	for _, path := range []string{
		filepath.Join(repoRoot, "boms", "team-a", "app.json"),
		filepath.Join(repoRoot, "boms", "team-a", "worker.yaml"),
		filepath.Join(repoRoot, "boms", "team-b", "web.yaml"),
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
		excludedPaths: []string{"boms/team-a"},
	}

	targets, err := runner.resolveTargets()
	if err != nil {
		t.Fatalf("resolveTargets returned error: %v", err)
	}

	want := []string{"boms/team-b/web.yaml"}
	if !slices.Equal(targets, want) {
		t.Fatalf("unexpected prefix-excluded check targets:\nwant: %v\ngot:  %v", want, targets)
	}
}

func TestCheckResolveTargetsDiscoversRepoRootWhenEmpty(t *testing.T) {
	repoRoot := t.TempDir()
	for _, path := range []string{
		filepath.Join(repoRoot, "charts", "api", "bom.json"),
		filepath.Join(repoRoot, "charts", "worker", "bom.json"),
		filepath.Join(repoRoot, "charts", "worker", "values.yaml"),
		filepath.Join(repoRoot, "manifests.json"),
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

	want := []string{"charts/api/bom.json", "charts/worker/bom.json"}
	if !slices.Equal(targets, want) {
		t.Fatalf("unexpected discovered check targets:\nwant: %v\ngot:  %v", want, targets)
	}
}

func TestCheckResolveTargetsAppliesExcludesWhenIncludePathsEmpty(t *testing.T) {
	repoRoot := t.TempDir()
	for _, path := range []string{
		filepath.Join(repoRoot, "charts", "api", "bom.json"),
		filepath.Join(repoRoot, "charts", "pc-applications", "bom.json"),
		filepath.Join(repoRoot, "charts", "worker", "bom.json"),
		filepath.Join(repoRoot, "charts", "worker", "values.yaml"),
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
		excludedPaths: []string{"charts/pc-applications"},
	}

	targets, err := runner.resolveTargets()
	if err != nil {
		t.Fatalf("resolveTargets returned error: %v", err)
	}

	want := []string{"charts/api/bom.json", "charts/worker/bom.json"}
	if !slices.Equal(targets, want) {
		t.Fatalf("unexpected check targets with excludes and empty includes:\nwant: %v\ngot:  %v", want, targets)
	}
}

func TestRunCheckMergesBomlintExcludes(t *testing.T) {
	repoRoot := t.TempDir()
	for _, path := range []string{
		filepath.Join(repoRoot, "boms", "api.json"),
		filepath.Join(repoRoot, "boms", "legacy", "worker.yaml"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir BOM directory: %v", err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatalf("write BOM: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".bomlint.yml"), []byte(`
excludePaths:
  - boms/legacy
allowedRegistries:
  - ghcr.io
`), 0o600); err != nil {
		t.Fatalf("write lint config: %v", err)
	}

	var checked []string
	var checkedFormat string
	err := RunCheckWithDependencies(context.Background(), CheckConfig{
		BaseConfig: BaseConfig{IncludePaths: "boms"},
	}, io.Discard, io.Discard, Dependencies{
		Getwd:     func() (string, error) { return repoRoot, nil },
		LookupEnv: func(string) (string, bool) { return "", false },
		Stat:      os.Stat,
		WalkDir:   filepath.WalkDir,
		CheckBOM: func(_ logging.Logger, cfg checkworkflow.Config) error {
			checked = append(checked, filepath.ToSlash(cfg.BOMPath))
			checkedFormat = cfg.BOMFormat
			return nil
		},
		WriteOutput:  func(string, string) error { return nil },
		WriteSummary: func(string) error { return nil },
	})
	if err != nil {
		t.Fatalf("RunCheckWithDependencies returned error: %v", err)
	}
	if len(checked) != 1 || !strings.HasSuffix(checked[0], "/boms/api.json") {
		t.Fatalf("unexpected checked BOMs: %#v", checked)
	}
	if checkedFormat != "csbom-v2" {
		t.Fatalf("expected default BOM format csbom-v2, got %q", checkedFormat)
	}
}

func TestRunCheckCollectsAllBOMFailuresAndWritesSummary(t *testing.T) {
	repoRoot := t.TempDir()
	for _, path := range []string{
		filepath.Join(repoRoot, "boms", "fail-api.json"),
		filepath.Join(repoRoot, "boms", "good.json"),
		filepath.Join(repoRoot, "boms", "fail-worker.yaml"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir BOM directory: %v", err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatalf("write BOM: %v", err)
		}
	}

	var checked []string
	var summary string
	outputs := map[string]string{}
	err := RunCheckWithDependencies(context.Background(), CheckConfig{
		BaseConfig: BaseConfig{IncludePaths: "boms"},
		BOMFormat:  "spdx-json",
	}, io.Discard, io.Discard, Dependencies{
		Getwd:     func() (string, error) { return repoRoot, nil },
		LookupEnv: func(string) (string, bool) { return "", false },
		Stat:      os.Stat,
		WalkDir:   filepath.WalkDir,
		CheckBOM: func(_ logging.Logger, cfg checkworkflow.Config) error {
			if cfg.BOMFormat != "spdx-json" {
				t.Fatalf("expected configured BOM format spdx-json, got %q", cfg.BOMFormat)
			}
			target := filepath.ToSlash(cfg.BOMPath)
			checked = append(checked, target)
			if strings.Contains(target, "fail-") {
				return fmt.Errorf("invalid reference in %s", filepath.Base(target))
			}
			return nil
		},
		WriteOutput: func(name string, value string) error {
			outputs[name] = value
			return nil
		},
		WriteSummary: func(content string) error {
			summary = content
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected aggregated validation error")
	}
	if len(checked) != 3 {
		t.Fatalf("expected every BOM to be checked, got %#v", checked)
	}
	for _, fragment := range []string{"2 BOM file(s) failed validation", "boms/fail-api.json", "boms/fail-worker.yaml"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("aggregated error missing %q:\n%s", fragment, err)
		}
		if !strings.Contains(summary, fragment) && fragment != "2 BOM file(s) failed validation" {
			t.Fatalf("summary missing %q:\n%s", fragment, summary)
		}
	}
	if !strings.Contains(summary, "Failed BOMs: 2") || !strings.Contains(summary, "Check results") {
		t.Fatalf("summary does not describe failures:\n%s", summary)
	}
	for _, fragment := range []string{"| BOM", "| Status", "| Error", "| failed", "| passed"} {
		if !strings.Contains(summary, fragment) {
			t.Fatalf("summary table missing %q:\n%s", fragment, summary)
		}
	}
	wantFailedPaths := "boms/fail-api.json\nboms/fail-worker.yaml"
	if outputs["failed-boms"] != wantFailedPaths {
		t.Fatalf("unexpected failed-boms output:\nwant: %q\ngot:  %q", wantFailedPaths, outputs["failed-boms"])
	}
	wantProcessedBOMs := "boms/fail-api.json\nboms/fail-worker.yaml\nboms/good.json"
	if outputs["matched-boms"] != wantProcessedBOMs || outputs["processed-boms"] != wantProcessedBOMs {
		t.Fatalf("unexpected matched/processed BOM outputs: %#v", outputs)
	}
	if _, exists := outputs["failed-paths"]; exists {
		t.Fatalf("legacy failed-paths output was written: %#v", outputs)
	}
}

func TestIsCheckFailuresError(t *testing.T) {
	validationErr := newCheckFailuresError([]CheckFailure{{
		Path: "boms/failed.json",
		Err:  fmt.Errorf("invalid image"),
	}})
	if !IsCheckFailuresError(validationErr) {
		t.Fatalf("expected validation error to be recognized: %v", validationErr)
	}
	if IsCheckFailuresError(fmt.Errorf("write summary")) {
		t.Fatal("unexpected non-validation error classification")
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

	var stdout bytes.Buffer
	runner := generateRunner{
		baseRunner: baseRunner{
			repoRoot: repoRoot,
			stdout:   &stdout,
			logger:   newTestLogger(false),
			deps: Dependencies{
				GenerateBOM: func(_ io.Writer, _ logging.Logger, cfg generateworkflow.Config) error {
					if cfg.ChartPath == "" {
						return fmt.Errorf("missing chart path")
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
	if stdout.String() != "after\n" {
		t.Fatalf("unexpected generated BOM on stdout: %q", stdout.String())
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

	var stdout bytes.Buffer
	runner := generateRunner{
		baseRunner: baseRunner{
			repoRoot: repoRoot,
			stdout:   &stdout,
			logger:   newTestLogger(false),
			deps: Dependencies{
				GenerateBOM: func(_ io.Writer, _ logging.Logger, _ generateworkflow.Config) error {
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
	if stdout.String() != "stable\n" {
		t.Fatalf("unexpected generated BOM on stdout: %q", stdout.String())
	}
}

func TestRenderGitHubSummary(t *testing.T) {
	summary := renderGitHubSummary("generate", Result{
		ChangedPaths:        []string{"charts/api/values.yaml"},
		MatchedPaths:        []string{"charts/api"},
		ProcessedPaths:      []string{"charts/api/bom.json"},
		ChangedOutputPaths:  []string{"charts/api/bom.json"},
		AnyProcessedChanged: true,
	})

	for _, fragment := range []string{
		"bom action `generate`",
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

func TestRenderCheckSummaryAsYAML(t *testing.T) {
	summary := renderCheckOutputSummary(Result{
		ChangedPaths:   []string{"boms/failed.json"},
		MatchedPaths:   []string{"boms/failed.json", "boms/passed.json"},
		ProcessedPaths: []string{"boms/failed.json", "boms/passed.json"},
		Failures: []CheckFailure{
			{Path: "boms/failed.json", Err: fmt.Errorf("registry denied")},
		},
		SummaryFormat: "yaml",
	})

	for _, fragment := range []string{
		"changedBoms: 1",
		"matchedBoms: 2",
		"processedBoms: 2",
		"failedBoms: 1",
		"bom: boms/failed.json",
		"status: failed",
		"error: registry denied",
		"bom: boms/passed.json",
		"status: passed",
	} {
		if !strings.Contains(summary, fragment) {
			t.Fatalf("YAML summary missing %q:\n%s", fragment, summary)
		}
	}
	if strings.Contains(summary, "| BOM") || strings.Contains(summary, "```yaml") {
		t.Fatalf("YAML summary contains table output:\n%s", summary)
	}
}

func TestRenderCheckSummaryAsTable(t *testing.T) {
	summary := renderCheckOutputSummary(Result{
		MatchedPaths:   []string{"boms/passed.json"},
		ProcessedPaths: []string{"boms/passed.json"},
		SummaryFormat:  "table",
	})

	for _, fragment := range []string{"METRIC", "VALUE", "Processed BOMs", "BOM", "STATUS", "boms/passed.json", "passed"} {
		if !strings.Contains(summary, fragment) {
			t.Fatalf("table summary missing %q:\n%s", fragment, summary)
		}
	}
	if strings.Contains(summary, "| BOM") || strings.Contains(summary, "status:") {
		t.Fatalf("stdout table contains another format:\n%s", summary)
	}
}

func TestCheckSummariesIncludeImageFailureMetrics(t *testing.T) {
	result := Result{Failures: []CheckFailure{
		{
			Path: "boms/wrong-registry.json",
			Err: &images.DisallowedRegistriesError{References: []string{
				"quay.io/example/api:1.0.0",
				"quay.io/example/worker:1.0.0",
			}},
		},
		{
			Path: "boms/missing.json",
			Err: &images.MissingImageError{References: []string{
				"ghcr.io/example/a:1.0.0",
				"ghcr.io/example/b:1.0.0",
				"ghcr.io/example/c:1.0.0",
			}},
		},
	}}

	tableSummary := renderCheckSummaryTable(result)
	if !strings.Contains(tableSummary, "Images in wrong registry  2") || !strings.Contains(tableSummary, "Images not found          3") {
		t.Fatalf("table summary missing image metrics:\n%s", tableSummary)
	}

	yamlSummary := renderCheckSummaryYAML(result)
	if !strings.Contains(yamlSummary, "wrongRegistryImages: 2") || !strings.Contains(yamlSummary, "missingImages: 3") {
		t.Fatalf("YAML summary missing image metrics:\n%s", yamlSummary)
	}

	githubSummary := renderGitHubCheckSummary(result)
	if !strings.Contains(githubSummary, "Images in wrong registry: 2") || !strings.Contains(githubSummary, "Images not found: 3") {
		t.Fatalf("GitHub summary missing image metrics:\n%s", githubSummary)
	}
}

func TestFinishSeparatesStdoutAndGitHubSummaryFormats(t *testing.T) {
	var stdout bytes.Buffer
	var githubSummary string
	runner := baseRunner{
		stdout: &stdout,
		logger: newTestLogger(false),
		deps: Dependencies{
			WriteOutput: func(string, string) error { return nil },
			WriteSummary: func(content string) error {
				githubSummary = content
				return nil
			},
		},
	}
	result := Result{
		MatchedPaths:   []string{"boms/failed.json"},
		ProcessedPaths: []string{"boms/failed.json"},
		Failures: []CheckFailure{
			{Path: "boms/failed.json", Err: fmt.Errorf("registry denied")},
		},
		SummaryFormat: "yaml",
	}

	if err := runner.finish("check", result); err != nil {
		t.Fatalf("finish returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "status: failed") || strings.Contains(stdout.String(), "| BOM") {
		t.Fatalf("stdout is not raw YAML:\n%s", stdout.String())
	}
	if !strings.Contains(githubSummary, "| BOM") || strings.Contains(githubSummary, "```yaml") {
		t.Fatalf("GitHub summary is not a Markdown table:\n%s", githubSummary)
	}
}

func TestNormalizeCheckSummaryFormat(t *testing.T) {
	for input, want := range map[string]string{
		"":      "table",
		"table": "table",
		"YAML":  "yaml",
		"yml":   "yaml",
	} {
		got, err := normalizeCheckSummaryFormat(input)
		if err != nil {
			t.Fatalf("normalizeCheckSummaryFormat(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Fatalf("normalizeCheckSummaryFormat(%q): want %q, got %q", input, want, got)
		}
	}

	if _, err := normalizeCheckSummaryFormat("json"); err == nil {
		t.Fatal("expected unsupported summary format to fail")
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
	if strings.Contains(got, "bom-action:") {
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
	if !strings.Contains(logger.String(), "repository root source  GITHUB_WORKSPACE") {
		t.Fatalf("expected startup log to include repo root source, got logs=%s", logger.String())
	}
}
