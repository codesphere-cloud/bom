package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/codesphere-cloud/helm-bom/internal/helm"
	"github.com/codesphere-cloud/helm-bom/internal/images"
	"github.com/codesphere-cloud/helm-bom/internal/sbom"
	"github.com/spf13/cobra"
)

const version = "dev"

type config struct {
	chartPath   string
	releaseName string
	namespace   string
	format      string
	outputPath  string
	valuesFiles []string
	setValues   []string
	setStrings  []string
	helmArgs    []string
}

func Run(args []string, stdout io.Writer, stderr io.Writer) error {
	cmd := NewRootCommand(stdout, stderr)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func NewRootCommand(stdout io.Writer, stderr io.Writer) *cobra.Command {
	var cfg config

	cmd := &cobra.Command{
		Use:   "helm-bom <chart>",
		Short: "Template Helm charts and emit an SPDX SBOM for referenced OCI images",
		Long: "helm-bom renders a Helm chart with helm template, extracts OCI image references " +
			"from supported Kubernetes workload resources, and writes the result in either SPDX JSON " +
			"or the internal csbom JSON/YAML formats.",
		Example: "" +
			"  helm-bom ./chart\n" +
			"  helm-bom ./chart --values values.yaml --set image.tag=1.2.3\n" +
			"  helm-bom ./chart --format csbom-json --output bom.json\n" +
			"  helm-bom ./chart --format csbom-yaml --output bom.yaml\n" +
			"  helm-bom ./chart --release-name my-release --namespace production\n" +
			"  helm-bom ./chart --helm-arg=--include-crds",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return cobra.ExactArgs(1)(cmd, args)
			}
			cfg.chartPath = args[0]
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.OutOrStdout(), cfg)
		},
	}

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetVersionTemplate("{{.Version}}\n")
	cmd.Version = version
	cmd.DisableFlagsInUseLine = true

	flags := cmd.Flags()
	flags.StringVar(&cfg.releaseName, "release-name", "", "Helm release name. Defaults to the chart name from Chart.yaml.")
	flags.StringVar(&cfg.namespace, "namespace", "default", "Namespace passed to helm template.")
	flags.StringVar(&cfg.format, "format", "spdx-json", "Output format: spdx-json, spdx, csbom-json, csbom-yaml, or csbom.")
	flags.StringVarP(&cfg.outputPath, "output", "o", "", "Write output to a file instead of stdout.")
	flags.StringSliceVar(&cfg.valuesFiles, "values", nil, "Additional Helm values files. May be specified multiple times.")
	flags.StringSliceVar(&cfg.setValues, "set", nil, "Helm --set overrides. May be specified multiple times.")
	flags.StringSliceVar(&cfg.setStrings, "set-string", nil, "Helm --set-string overrides. May be specified multiple times.")
	flags.StringSliceVar(&cfg.helmArgs, "helm-arg", nil, "Additional raw arguments appended to helm template, for example --helm-arg=--include-crds.")

	return cmd
}

func run(stdout io.Writer, cfg config) error {
	chartName, err := helm.ChartName(cfg.chartPath)
	if err != nil {
		return err
	}

	if cfg.releaseName == "" {
		cfg.releaseName = chartName
	}

	renderer := helm.Renderer{}
	manifest, err := renderer.Template(helm.TemplateRequest{
		ChartPath:   cfg.chartPath,
		ReleaseName: cfg.releaseName,
		Namespace:   cfg.namespace,
		ValuesFiles: cfg.valuesFiles,
		SetValues:   cfg.setValues,
		SetStrings:  cfg.setStrings,
		ExtraArgs:   cfg.helmArgs,
	})
	if err != nil {
		return err
	}

	refs, err := images.Extract(manifest)
	if err != nil {
		return err
	}

	document := sbom.Document{
		Metadata: sbom.Metadata{
			Tool: sbom.ToolMetadata{
				Name:    "helm-bom",
				Version: version,
			},
			Source: sbom.SourceMetadata{
				Chart:       cfg.chartPath,
				ChartName:   chartName,
				ReleaseName: cfg.releaseName,
				Namespace:   cfg.namespace,
				ValuesFiles: cfg.valuesFiles,
				SetValues:   cfg.setValues,
				SetStrings:  cfg.setStrings,
				HelmArgs:    cfg.helmArgs,
			},
		},
		Components: sbom.ComponentsFromImages(refs),
	}

	formatter, err := sbom.NewFormatter(cfg.format)
	if err != nil {
		return err
	}

	if cfg.outputPath != "" {
		file, createErr := os.Create(cfg.outputPath)
		if createErr != nil {
			return fmt.Errorf("create output file: %w", createErr)
		}
		defer func() {
			_ = file.Close()
		}()

		return formatter.Format(file, document)
	}

	return formatter.Format(stdout, document)
}
