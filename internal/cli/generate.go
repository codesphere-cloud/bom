package cli

import (
	"github.com/codesphere-cloud/bom/internal/generate"
	"github.com/spf13/cobra"
)

func defaultGenerateConfig() generate.Config {
	return generate.Config{
		Format:      "spdx-json",
		Namespace:   "default",
		ToolVersion: version,
	}
}

func (c *CLI) newGenerateCommand() *cobra.Command {
	cfg := defaultGenerateConfig()
	cmd := &cobra.Command{
		Use:   "generate <chart>",
		Short: "Render a Helm chart and generate an SBOM for referenced OCI images",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg.ChartPath = args[0]
			return generate.Run(c.stdout, c.logger(cfg.Debug), cfg)
		},
	}

	addGenerateFlags(cmd, &cfg)
	return cmd
}

func addGenerateFlags(cmd *cobra.Command, cfg *generate.Config) {
	flags := cmd.Flags()
	flags.BoolVar(&cfg.Debug, "debug", false, "Enable debug logging.")
	flags.BoolVar(&cfg.SBOM, "sbom", false, "Generate CycloneDX and SPDX JSON SBOMs for every referenced image.")
	flags.BoolVar(&cfg.Cosign, "cosign", false, "Attest generated image SBOMs with keyless Cosign signing. Requires --sbom.")
	flags.BoolVar(&cfg.Force, "force", false, "Regenerate image SBOMs and upload attestations even when they already exist. Requires --sbom.")
	flags.StringVar(&cfg.ReleaseName, "release-name", "", "Helm release name. Defaults to the chart name from Chart.yaml.")
	flags.StringVar(&cfg.Namespace, "namespace", cfg.Namespace, "Namespace passed to helm template.")
	flags.StringVar(&cfg.Format, "format", cfg.Format, "Output format: spdx-json, spdx, csbom-json, csbom-yaml, csbom, csbom-v2-json, csbom-v2-yaml, or csbom-v2.")
	flags.StringVarP(&cfg.OutputPath, "output", "o", "", "Write output to a file instead of stdout.")
	flags.StringSliceVar(&cfg.ValuesFiles, "values", nil, "Additional Helm values files. May be specified multiple times.")
	flags.StringSliceVar(&cfg.SetValues, "set", nil, "Helm --set overrides. May be specified multiple times.")
	flags.StringSliceVar(&cfg.SetStrings, "set-string", nil, "Helm --set-string overrides. May be specified multiple times.")
	flags.StringSliceVar(&cfg.HelmArgs, "helm-arg", nil, "Additional raw arguments appended to helm template, for example --helm-arg=--include-crds.")
	flags.BoolVar(&cfg.ValidateConfiguredImageExist, "validate-configured-image-exists", false, "Fail when a configured additional image resource or selector does not resolve from the rendered manifest.")
}
