package ghaction

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	checkworkflow "github.com/codesphere-cloud/bom/internal/check"
	"github.com/codesphere-cloud/bom/internal/logging"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("defaultDependencies WriteSummary", func() {
	It("does nothing outside of GitHub Actions", func() {
		GinkgoT().Setenv("GITHUB_STEP_SUMMARY", "")

		deps := defaultDependencies()
		Expect(deps.WriteSummary("check summary\n")).To(Succeed())
	})

	It("writes the GitHub summary file when configured", func() {
		summaryPath := filepath.Join(GinkgoT().TempDir(), "summary.md")
		GinkgoT().Setenv("GITHUB_STEP_SUMMARY", summaryPath)

		deps := defaultDependencies()
		Expect(deps.WriteSummary("check summary\n")).To(Succeed())

		content, err := os.ReadFile(summaryPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(content)).To(Equal("check summary\n"))
	})
})

var _ = Describe("RunCheckWithDependencies", func() {
	It("merges bomlint excludes into the resolved targets", func() {
		repoRoot := GinkgoT().TempDir()
		for _, path := range []string{
			filepath.Join(repoRoot, "boms", "api.json"),
			filepath.Join(repoRoot, "boms", "legacy", "worker.yaml"),
		} {
			Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
			Expect(os.WriteFile(path, []byte("{}\n"), 0o600)).To(Succeed())
		}
		Expect(os.WriteFile(filepath.Join(repoRoot, ".bomlint.yml"), []byte(`
excludePaths:
  - boms/legacy
allowedRegistries:
  - ghcr.io
`), 0o600)).To(Succeed())

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
		Expect(err).NotTo(HaveOccurred())
		Expect(checked).To(HaveLen(1))
		Expect(checked[0]).To(HaveSuffix("/boms/api.json"))
		Expect(checkedFormat).To(Equal("csbom-v2"))
	})

	It("collects all BOM failures and writes a summary", func() {
		repoRoot := GinkgoT().TempDir()
		for _, path := range []string{
			filepath.Join(repoRoot, "boms", "fail-api.json"),
			filepath.Join(repoRoot, "boms", "good.json"),
			filepath.Join(repoRoot, "boms", "fail-worker.yaml"),
		} {
			Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
			Expect(os.WriteFile(path, []byte("{}\n"), 0o600)).To(Succeed())
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
				Expect(cfg.BOMFormat).To(Equal("spdx-json"))
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
		Expect(err).To(HaveOccurred())
		Expect(checked).To(HaveLen(3))

		for _, fragment := range []string{"2 BOM file(s) failed validation", "boms/fail-api.json", "boms/fail-worker.yaml"} {
			Expect(err.Error()).To(ContainSubstring(fragment))
			if fragment != "2 BOM file(s) failed validation" {
				Expect(summary).To(ContainSubstring(fragment))
			}
		}
		Expect(summary).To(ContainSubstring("Failed BOMs: 2"))
		Expect(summary).To(ContainSubstring("Check results"))

		for _, fragment := range []string{"| BOM", "| Status", "| Error", "| failed", "| passed"} {
			Expect(summary).To(ContainSubstring(fragment))
		}

		wantFailedPaths := "boms/fail-api.json\nboms/fail-worker.yaml"
		Expect(outputs["failed-boms"]).To(Equal(wantFailedPaths))

		wantProcessedBOMs := "boms/fail-api.json\nboms/fail-worker.yaml\nboms/good.json"
		Expect(outputs["matched-boms"]).To(Equal(wantProcessedBOMs))
		Expect(outputs["processed-boms"]).To(Equal(wantProcessedBOMs))

		_, exists := outputs["failed-paths"]
		Expect(exists).To(BeFalse())
	})
})
