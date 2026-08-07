package main

import (
	"context"
	"fmt"
	"os"

	"github.com/codesphere-cloud/helm-bom/internal/ghaction"
	"github.com/spf13/cobra"
)

func main() {
	var cfg ghaction.Config

	cmd := &cobra.Command{
		Use:           "helm-bom-action",
		Short:         "GitHub Action wrapper for helm-bom generate and check flows",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ghaction.Run(context.Background(), cfg, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&cfg.Mode, "mode", "", "Action mode: generate or check.")
	flags.StringVar(&cfg.Paths, "paths", "", "Newline- or comma-separated chart or BOM paths. Glob patterns are supported.")
	flags.BoolVar(&cfg.ChangedOnly, "changed-only", false, "Restrict processing to paths touched by the current PR or push.")
	flags.BoolVar(&cfg.Debug, "debug", false, "Enable debug logging and pass --debug through to helm-bom subcommands.")
	flags.StringVar(&cfg.Format, "format", "spdx-json", "Generate output format.")
	flags.StringVar(&cfg.Namespace, "namespace", "default", "Helm namespace used for generation.")
	flags.StringVar(&cfg.ReleaseName, "release-name", "", "Helm release name override.")
	flags.BoolVar(&cfg.ValidateConfiguredImageExist, "validate-configured-image-exists", false, "Fail when configured additional image selectors do not resolve.")
	flags.StringVar(&cfg.RegistryServer, "registry-server", "", "Registry server to log in to before checking images.")
	flags.StringVar(&cfg.RegistryUsername, "registry-username", "", "Registry username used with registry-server.")
	flags.StringVar(&cfg.RegistryPassword, "registry-password", "", "Registry password used with registry-server.")
	flags.BoolVar(&cfg.FailOnNoMatches, "fail-on-no-matches", false, "Exit with an error when no paths match the configured filters.")
	_ = cmd.MarkFlagRequired("mode")

	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
