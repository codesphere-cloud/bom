package cli

import (
	"io"

	"github.com/codesphere-cloud/bom/internal/generate"
	"github.com/codesphere-cloud/bom/internal/logging"
	"github.com/spf13/cobra"
)

const version = "dev"

type CLI struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func New(stdin io.Reader, stdout io.Writer, stderr io.Writer) *CLI {
	return &CLI{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}
}

func (c *CLI) Run(args []string) error {
	cmd := c.RootCommand()
	cmd.SetArgs(args)
	return cmd.Execute()
}

func (c *CLI) RootCommand() *cobra.Command {
	cfg := defaultGenerateConfig()

	cmd := &cobra.Command{
		Use:   "bom [chart]",
		Short: "Generate and validate SBOMs for OCI images referenced by Helm charts",
		Long: "bom renders a Helm chart with helm template, extracts OCI image references " +
			"from supported Kubernetes workload resources, writes the result in either SPDX JSON " +
			"or the internal csbom/csbom-v2 JSON/YAML formats, and can validate a generated BOM against upstream registries.",
		Example: "" +
			"  bom ./chart\n" +
			"  bom generate ./chart --values values.yaml --set image.tag=1.2.3\n" +
			"  bom generate ./chart --format csbom-json --output bom.json\n" +
			"  bom generate ./chart --format csbom-v2-yaml --output bom.yaml\n" +
			"  bom check bom.json\n" +
			"  bom generate ./chart --release-name my-release --namespace production",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}

			cfg.ChartPath = args[0]
			return generate.Run(c.stdout, c.logger(cfg.Debug), cfg)
		},
	}

	cmd.SetIn(c.stdin)
	cmd.SetOut(c.stdout)
	cmd.SetErr(c.stderr)
	cmd.SetVersionTemplate("{{.Version}}\n")
	cmd.Version = version
	cmd.DisableFlagsInUseLine = true

	addGenerateFlags(cmd, &cfg)
	cmd.AddCommand(c.newGenerateCommand())
	cmd.AddCommand(c.newCheckCommand())
	cmd.AddCommand(c.newRegistryCommand())

	return cmd
}

func (c *CLI) logger(debug bool) logging.Logger {
	return logging.NewWriterLogger(c.stderr, debug)
}
