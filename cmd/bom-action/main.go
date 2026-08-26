package main

import (
	"context"
	"fmt"
	"os"

	"github.com/codesphere-cloud/bom/internal/ghaction"
	"github.com/codesphere-cloud/bom/internal/sbom"
	"github.com/spf13/cobra"
)

func main() {
	cmd := &cobra.Command{
		Use:           "bom-action",
		Short:         "GitHub Action wrapper for bom generate and check flows",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(newGenerateCommand())
	cmd.AddCommand(newCheckCommand())
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

	if err := cmd.Execute(); err != nil {
		if !ghaction.IsCheckFailuresError(err) {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func newGenerateCommand() *cobra.Command {
	cfg := ghaction.GenerateConfig{
		BaseConfig: ghaction.BaseConfig{},
		Format:     "spdx-json",
		Namespace:  "default",
	}

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Run the generate GitHub Action flow",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ghaction.RunGenerate(context.Background(), cfg, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}

	addBaseFlags(cmd, &cfg.BaseConfig)
	flags := cmd.Flags()
	flags.StringVar(&cfg.Format, "format", cfg.Format, "Generate output format.")
	flags.StringVar(&cfg.Namespace, "namespace", cfg.Namespace, "Helm namespace used for generation.")
	flags.StringVar(&cfg.ReleaseName, "release-name", "", "Helm release name override.")
	flags.BoolVar(&cfg.SBOM, "sbom", cfg.SBOM, "Generate a CycloneDX SBOM for every referenced image.")
	flags.BoolVar(&cfg.Cosign, "cosign", cfg.Cosign, "Attest generated image SBOMs with keyless Cosign signing.")
	flags.BoolVar(&cfg.ValidateConfiguredImageExist, "validate-configured-image-exists", false, "Fail when configured additional image selectors do not resolve.")
	flags.StringVar(&cfg.ExcludePaths, "exclude-paths", "", "Newline- or comma-separated chart paths to exclude. Glob patterns are supported.")
	return cmd
}

func newCheckCommand() *cobra.Command {
	cfg := ghaction.CheckConfig{
		BaseConfig:    ghaction.BaseConfig{},
		BOMFormat:     sbom.DefaultInputFormat,
		SummaryFormat: "table",
	}

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run the check GitHub Action flow",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ghaction.RunCheck(context.Background(), cfg, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}

	addBaseFlags(cmd, &cfg.BaseConfig)
	flags := cmd.Flags()
	flags.StringVar(&cfg.ExcludePaths, "exclude-paths", "", "Newline- or comma-separated BOM paths to exclude. Glob patterns are supported.")
	flags.StringVar(&cfg.BOMFormat, "bom-format", cfg.BOMFormat, "Input BOM format.")
	flags.StringVar(&cfg.SummaryFormat, "format", cfg.SummaryFormat, "Check summary format: table or yaml.")
	return cmd
}

func addBaseFlags(cmd *cobra.Command, cfg *ghaction.BaseConfig) {
	flags := cmd.Flags()
	flags.StringVar(&cfg.IncludePaths, "include-paths", "", "Newline- or comma-separated chart or BOM paths to include. Glob patterns are supported.")
	flags.BoolVar(&cfg.ChangedOnly, "changed-only", false, "Restrict processing to paths touched by the current PR or push.")
	flags.BoolVar(&cfg.Debug, "debug", false, "Enable debug logging and pass --debug through to bom subcommands.")
	flags.BoolVar(&cfg.FailOnNoMatches, "fail-on-no-matches", false, "Exit with an error when no paths match the configured filters.")
}
