package ghaction

import (
	"fmt"
	"path/filepath"

	"github.com/codesphere-cloud/bom/internal/images"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("buildGenerateOutputPath", func() {
	want := filepath.Join("charts", "api", "bom.yaml")

	DescribeTable("building the output path for a target and format",
		func(target string, format string) {
			got := buildGenerateOutputPath(target, format)
			Expect(got).To(Equal(want))
		},
		Entry("csbom-yaml", "charts/api", "csbom-yaml"),
		Entry("csbom-v2-yaml", "charts/api", "csbom-v2-yaml"),
	)
})

var _ = Describe("renderGitHubSummary", func() {
	It("renders the generate summary fragments", func() {
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
			Expect(summary).To(ContainSubstring(fragment))
		}
	})
})

var _ = Describe("renderCheckOutputSummary", func() {
	It("renders as YAML", func() {
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
			Expect(summary).To(ContainSubstring(fragment))
		}
		Expect(summary).NotTo(ContainSubstring("| BOM"))
		Expect(summary).NotTo(ContainSubstring("```yaml"))
	})

	It("renders as a table", func() {
		summary := renderCheckOutputSummary(Result{
			MatchedPaths:   []string{"boms/passed.json"},
			ProcessedPaths: []string{"boms/passed.json"},
			SummaryFormat:  "table",
		})

		for _, fragment := range []string{"METRIC", "VALUE", "Processed BOMs", "BOM", "STATUS", "boms/passed.json", "passed"} {
			Expect(summary).To(ContainSubstring(fragment))
		}
		Expect(summary).NotTo(ContainSubstring("| BOM"))
		Expect(summary).NotTo(ContainSubstring("status:"))
	})
})

var _ = Describe("check summaries", func() {
	It("include image failure metrics across every renderer", func() {
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
		Expect(tableSummary).To(ContainSubstring("Images in wrong registry  2"))
		Expect(tableSummary).To(ContainSubstring("Images not found          3"))

		yamlSummary := renderCheckSummaryYAML(result)
		Expect(yamlSummary).To(ContainSubstring("wrongRegistryImages: 2"))
		Expect(yamlSummary).To(ContainSubstring("missingImages: 3"))

		githubSummary := renderGitHubCheckSummary(result)
		Expect(githubSummary).To(ContainSubstring("Images in wrong registry: 2"))
		Expect(githubSummary).To(ContainSubstring("Images not found: 3"))
	})
})
