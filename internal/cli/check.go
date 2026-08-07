package cli

import (
	checkworkflow "github.com/codesphere-cloud/bom/internal/check"
	"github.com/codesphere-cloud/bom/internal/sbom"
	"github.com/spf13/cobra"
)

type checkConfig struct {
	debug bool
	checkworkflow.Config
}

func (c *CLI) newCheckCommand() *cobra.Command {
	cfg := checkConfig{Config: checkworkflow.Config{BOMFormat: sbom.DefaultInputFormat}}
	cmd := &cobra.Command{
		Use:   "check <bom>",
		Short: "Validate that every image in a BOM exists in its upstream registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg.BOMPath = args[0]
			return checkworkflow.Run(c.logger(cfg.debug), cfg.Config)
		},
	}

	cmd.Flags().BoolVar(&cfg.debug, "debug", false, "Enable debug logging.")
	cmd.Flags().StringVar(&cfg.BOMFormat, "format", cfg.BOMFormat, "Input BOM format.")
	return cmd
}
