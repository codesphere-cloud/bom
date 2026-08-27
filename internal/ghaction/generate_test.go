package ghaction

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	generateworkflow "github.com/codesphere-cloud/bom/internal/generate"
	"github.com/codesphere-cloud/bom/internal/logging"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("generateRunner.resolveTargets", func() {
	It("discovers charts and skips nested subcharts", func() {
		repoRoot := GinkgoT().TempDir()
		for _, path := range []string{
			filepath.Join(repoRoot, "charts", "api", "Chart.yaml"),
			filepath.Join(repoRoot, "charts", "api", "charts", "dependency", "Chart.yaml"),
			filepath.Join(repoRoot, "charts", "worker", "Chart.yaml"),
		} {
			Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
			Expect(os.WriteFile(path, []byte("name: test\n"), 0o600)).To(Succeed())
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
		Expect(err).NotTo(HaveOccurred())
		Expect(slices.Equal(targets, []string{"charts/api", "charts/worker"})).To(BeTrue())
	})

	It("applies excludes when the configured include paths are empty", func() {
		repoRoot := GinkgoT().TempDir()
		for _, path := range []string{
			filepath.Join(repoRoot, "charts", "api", "Chart.yaml"),
			filepath.Join(repoRoot, "charts", "pc-applications", "Chart.yaml"),
			filepath.Join(repoRoot, "charts", "worker", "Chart.yaml"),
		} {
			Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
			Expect(os.WriteFile(path, []byte("name: test\n"), 0o600)).To(Succeed())
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
		Expect(err).NotTo(HaveOccurred())
		Expect(slices.Equal(targets, []string{"charts/api", "charts/worker"})).To(BeTrue())
	})

	It("supports globs in the configured paths", func() {
		repoRoot := GinkgoT().TempDir()
		for _, dir := range []string{
			filepath.Join(repoRoot, "charts", "api"),
			filepath.Join(repoRoot, "charts", "worker"),
		} {
			Expect(os.MkdirAll(dir, 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte("name: test\n"), 0o600)).To(Succeed())
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
		Expect(err).NotTo(HaveOccurred())
		Expect(slices.Equal(targets, []string{"charts/api", "charts/worker"})).To(BeTrue())
	})

	It("supports prefix matches in the configured paths", func() {
		repoRoot := GinkgoT().TempDir()
		for _, dir := range []string{
			filepath.Join(repoRoot, "charts", "team-a", "api"),
			filepath.Join(repoRoot, "charts", "team-a", "worker"),
			filepath.Join(repoRoot, "charts", "team-b", "web"),
		} {
			Expect(os.MkdirAll(dir, 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(dir, "Chart.yaml"), []byte("name: test\n"), 0o600)).To(Succeed())
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
		Expect(err).NotTo(HaveOccurred())
		Expect(slices.Equal(targets, []string{"charts/team-a/api", "charts/team-a/worker"})).To(BeTrue())
	})

	It("excludes configured paths", func() {
		repoRoot := GinkgoT().TempDir()
		for _, path := range []string{
			filepath.Join(repoRoot, "charts", "api", "Chart.yaml"),
			filepath.Join(repoRoot, "charts", "worker", "Chart.yaml"),
			filepath.Join(repoRoot, "charts", "skip", "Chart.yaml"),
		} {
			Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
			Expect(os.WriteFile(path, []byte("name: test\n"), 0o600)).To(Succeed())
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
		Expect(err).NotTo(HaveOccurred())
		Expect(slices.Equal(targets, []string{"charts/api"})).To(BeTrue())
	})

	It("excludes prefixes", func() {
		repoRoot := GinkgoT().TempDir()
		for _, path := range []string{
			filepath.Join(repoRoot, "charts", "team-a", "api", "Chart.yaml"),
			filepath.Join(repoRoot, "charts", "team-a", "worker", "Chart.yaml"),
			filepath.Join(repoRoot, "charts", "team-b", "web", "Chart.yaml"),
		} {
			Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
			Expect(os.WriteFile(path, []byte("name: test\n"), 0o600)).To(Succeed())
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
		Expect(err).NotTo(HaveOccurred())
		Expect(slices.Equal(targets, []string{"charts/team-b/web"})).To(BeTrue())
	})
})

var _ = Describe("filterGenerateTargetsByChangedPaths", func() {
	It("keeps only targets under a changed path", func() {
		targets := []string{"charts/api", "charts/worker"}
		changed := []string{"charts/api/values.yaml", "README.md"}

		got := filterGenerateTargetsByChangedPaths(targets, changed)
		Expect(slices.Equal(got, []string{"charts/api"})).To(BeTrue())
	})
})

var _ = Describe("generateRunner.runTargets", func() {
	It("reports changed outputs", func() {
		repoRoot := GinkgoT().TempDir()
		chartDir := filepath.Join(repoRoot, "charts", "api")
		Expect(os.MkdirAll(chartDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte("name: api\n"), 0o600)).To(Succeed())
		outputPath := filepath.Join(chartDir, "bom.yaml")
		Expect(os.WriteFile(outputPath, []byte("before\n"), 0o600)).To(Succeed())

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
						Expect(cfg.SBOM).To(BeTrue())
						Expect(cfg.Cosign).To(BeTrue())
						Expect(cfg.Force).To(BeTrue())
						return os.WriteFile(outputPath, []byte("after\n"), 0o600)
					},
				},
			},
			cfg: GenerateConfig{Format: "csbom-yaml", Namespace: "default", SBOM: true, Cosign: true, Force: true},
		}

		processed, changedOutputs, changedTargets, err := runner.runTargets([]string{"charts/api"})
		Expect(err).NotTo(HaveOccurred())
		Expect(slices.Equal(processed, []string{"charts/api/bom.yaml"})).To(BeTrue())
		Expect(slices.Equal(changedOutputs, []string{"charts/api/bom.yaml"})).To(BeTrue())
		Expect(slices.Equal(changedTargets, []string{"charts/api"})).To(BeTrue())
		Expect(stdout.String()).To(Equal("after\n"))
	})

	It("reports unchanged outputs", func() {
		repoRoot := GinkgoT().TempDir()
		chartDir := filepath.Join(repoRoot, "charts", "api")
		Expect(os.MkdirAll(chartDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte("name: api\n"), 0o600)).To(Succeed())
		outputPath := filepath.Join(chartDir, "bom.yaml")
		Expect(os.WriteFile(outputPath, []byte("stable\n"), 0o600)).To(Succeed())

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
		Expect(err).NotTo(HaveOccurred())
		Expect(slices.Equal(processed, []string{"charts/api/bom.yaml"})).To(BeTrue())
		Expect(changedOutputs).To(BeEmpty())
		Expect(changedTargets).To(BeEmpty())
		Expect(stdout.String()).To(Equal("stable\n"))
	})
})

var _ = Describe("generateRunner.run", func() {
	It("logs the repo root source on startup", func() {
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

		Expect(runner.run()).To(Succeed())
		Expect(logger.String()).To(ContainSubstring("repository root source"))
		Expect(logger.String()).To(ContainSubstring("GITHUB_WORKSPACE"))
	})
})
