package ghaction

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("checkRunner.resolveTargets", func() {
	It("expands directories into BOM files", func() {
		repoRoot := GinkgoT().TempDir()
		bomDir := filepath.Join(repoRoot, "boms")
		Expect(os.MkdirAll(bomDir, 0o755)).To(Succeed())

		for _, path := range []string{
			filepath.Join(bomDir, "app.json"),
			filepath.Join(bomDir, "worker.yaml"),
		} {
			Expect(os.WriteFile(path, []byte("{}\n"), 0o600)).To(Succeed())
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
		Expect(err).NotTo(HaveOccurred())
		Expect(slices.Equal(targets, []string{"boms/app.json", "boms/worker.yaml"})).To(BeTrue())
	})

	It("supports prefix matches", func() {
		repoRoot := GinkgoT().TempDir()
		for _, path := range []string{
			filepath.Join(repoRoot, "boms", "team-a", "app.json"),
			filepath.Join(repoRoot, "boms", "team-a", "worker.yaml"),
			filepath.Join(repoRoot, "boms", "team-b", "web.yaml"),
		} {
			Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
			Expect(os.WriteFile(path, []byte("{}\n"), 0o600)).To(Succeed())
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
		Expect(err).NotTo(HaveOccurred())
		Expect(slices.Equal(targets, []string{"boms/team-a/app.json", "boms/team-a/worker.yaml"})).To(BeTrue())
	})

	It("excludes configured paths", func() {
		repoRoot := GinkgoT().TempDir()
		bomDir := filepath.Join(repoRoot, "boms")
		Expect(os.MkdirAll(bomDir, 0o755)).To(Succeed())

		for _, path := range []string{
			filepath.Join(bomDir, "app.json"),
			filepath.Join(bomDir, "worker.yaml"),
			filepath.Join(bomDir, "skip", "nested.yaml"),
		} {
			Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
			Expect(os.WriteFile(path, []byte("{}\n"), 0o600)).To(Succeed())
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
		Expect(err).NotTo(HaveOccurred())
		Expect(slices.Equal(targets, []string{"boms/app.json"})).To(BeTrue())
	})

	It("excludes prefixes", func() {
		repoRoot := GinkgoT().TempDir()
		for _, path := range []string{
			filepath.Join(repoRoot, "boms", "team-a", "app.json"),
			filepath.Join(repoRoot, "boms", "team-a", "worker.yaml"),
			filepath.Join(repoRoot, "boms", "team-b", "web.yaml"),
		} {
			Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
			Expect(os.WriteFile(path, []byte("{}\n"), 0o600)).To(Succeed())
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
		Expect(err).NotTo(HaveOccurred())
		Expect(slices.Equal(targets, []string{"boms/team-b/web.yaml"})).To(BeTrue())
	})

	It("discovers the repo root when no include paths are configured", func() {
		repoRoot := GinkgoT().TempDir()
		for _, path := range []string{
			filepath.Join(repoRoot, "charts", "api", "bom.json"),
			filepath.Join(repoRoot, "charts", "worker", "bom.json"),
			filepath.Join(repoRoot, "charts", "worker", "values.yaml"),
			filepath.Join(repoRoot, "manifests.json"),
		} {
			Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
			Expect(os.WriteFile(path, []byte("{}\n"), 0o600)).To(Succeed())
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
		Expect(err).NotTo(HaveOccurred())
		Expect(slices.Equal(targets, []string{"charts/api/bom.json", "charts/worker/bom.json"})).To(BeTrue())
	})

	It("applies excludes when include paths are empty", func() {
		repoRoot := GinkgoT().TempDir()
		for _, path := range []string{
			filepath.Join(repoRoot, "charts", "api", "bom.json"),
			filepath.Join(repoRoot, "charts", "pc-applications", "bom.json"),
			filepath.Join(repoRoot, "charts", "worker", "bom.json"),
			filepath.Join(repoRoot, "charts", "worker", "values.yaml"),
		} {
			Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
			Expect(os.WriteFile(path, []byte("{}\n"), 0o600)).To(Succeed())
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
		Expect(err).NotTo(HaveOccurred())
		Expect(slices.Equal(targets, []string{"charts/api/bom.json", "charts/worker/bom.json"})).To(BeTrue())
	})
})

var _ = Describe("IsCheckFailuresError", func() {
	It("recognizes wrapped check-failures errors and rejects other errors", func() {
		validationErr := newCheckFailuresError([]CheckFailure{{
			Path: "boms/failed.json",
			Err:  fmt.Errorf("invalid image"),
		}})
		Expect(IsCheckFailuresError(validationErr)).To(BeTrue())
		Expect(IsCheckFailuresError(fmt.Errorf("write summary"))).To(BeFalse())
	})
})

var _ = Describe("normalizeCheckSummaryFormat", func() {
	DescribeTable("normalizing supported values",
		func(input string, want string) {
			got, err := normalizeCheckSummaryFormat(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(want))
		},
		Entry("empty defaults to table", "", "table"),
		Entry("table stays table", "table", "table"),
		Entry("uppercase YAML normalizes to lowercase", "YAML", "yaml"),
		Entry("yml alias normalizes to yaml", "yml", "yaml"),
	)

	It("rejects unsupported formats", func() {
		_, err := normalizeCheckSummaryFormat("json")
		Expect(err).To(HaveOccurred())
	})
})
