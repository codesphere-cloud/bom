package ghaction

import (
	"bytes"
	"fmt"

	"github.com/codesphere-cloud/bom/internal/logging"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("baseRunner.finish", func() {
	It("separates stdout and GitHub summary formats", func() {
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

		Expect(runner.finish("check", result)).To(Succeed())
		Expect(stdout.String()).To(ContainSubstring("status: failed"))
		Expect(stdout.String()).NotTo(ContainSubstring("| BOM"))
		Expect(githubSummary).To(ContainSubstring("| BOM"))
		Expect(githubSummary).NotTo(ContainSubstring("```yaml"))
	})
})

var _ = Describe("logging.LogList", func() {
	It("logs the list under the new prefix", func() {
		logger := newTestLogger(false)
		logging.LogList(logger, "changed paths", []string{"charts/api/Chart.yaml", "charts/api/values.yaml"})

		got := logger.String()
		for _, fragment := range []string{
			"changed paths (2):",
			"charts/api/Chart.yaml",
			"charts/api/values.yaml",
		} {
			Expect(got).To(ContainSubstring(fragment))
		}
		Expect(got).NotTo(ContainSubstring("bom-action:"))
	})
})
