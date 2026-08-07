package ghaction

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/codesphere-cloud/helm-bom/internal/cli"
	"github.com/codesphere-cloud/helm-bom/internal/logging"
)

type BaseConfig struct {
	Paths           string
	ChangedOnly     bool
	Debug           bool
	FailOnNoMatches bool
}

type GenerateConfig struct {
	BaseConfig
	Format                       string
	Namespace                    string
	ReleaseName                  string
	ValidateConfiguredImageExist bool
}

type CheckConfig struct {
	BaseConfig
	RegistryServer   string
	RegistryUsername string
	RegistryPassword string
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

type Result struct {
	ChangedPaths        []string
	MatchedPaths        []string
	ProcessedPaths      []string
	ChangedOutputPaths  []string
	ChangedTargetPaths  []string
	AnyProcessedChanged bool
}

type baseRunner struct {
	ctx             context.Context
	deps            Dependencies
	stdout          io.Writer
	repoRoot        string
	repoRootSource  string
	configuredPaths []string
	logger          logging.Logger
}

type generateRunner struct {
	baseRunner
	cfg GenerateConfig
}

type checkRunner struct {
	baseRunner
	cfg CheckConfig
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

func RunGenerate(ctx context.Context, cfg GenerateConfig, stdout io.Writer, stderr io.Writer) error {
	return RunGenerateWithDependencies(ctx, cfg, stdout, stderr, defaultDependencies())
}

func RunGenerateWithDependencies(ctx context.Context, cfg GenerateConfig, stdout io.Writer, stderr io.Writer, deps Dependencies) error {
	base, err := newBaseRunner(ctx, cfg.BaseConfig, stdout, stderr, deps)
	if err != nil {
		return err
	}
	return generateRunner{baseRunner: base, cfg: cfg}.run()
}

func RunCheck(ctx context.Context, cfg CheckConfig, stdout io.Writer, stderr io.Writer) error {
	return RunCheckWithDependencies(ctx, cfg, stdout, stderr, defaultDependencies())
}

func RunCheckWithDependencies(ctx context.Context, cfg CheckConfig, stdout io.Writer, stderr io.Writer, deps Dependencies) error {
	base, err := newBaseRunner(ctx, cfg.BaseConfig, stdout, stderr, deps)
	if err != nil {
		return err
	}
	return checkRunner{baseRunner: base, cfg: cfg}.run()
}

func newBaseRunner(ctx context.Context, cfg BaseConfig, stdout io.Writer, stderr io.Writer, deps Dependencies) (baseRunner, error) {
	repoRoot, repoRootSource, err := resolveRepoRoot(deps.Getwd, deps.LookupEnv)
	if err != nil {
		return baseRunner{}, err
	}

	return baseRunner{
		ctx:             ctx,
		deps:            deps,
		stdout:          stdout,
		repoRoot:        repoRoot,
		repoRootSource:  repoRootSource,
		configuredPaths: parseList(cfg.Paths),
		logger:          logging.NewWriterLogger(stderr, cfg.Debug),
	}, nil
}
