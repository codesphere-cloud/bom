package cli

import (
	"github.com/codesphere-cloud/bom/internal/login"
	"github.com/spf13/cobra"
)

type registryLoginConfig struct {
	debug bool
	login.Config
}

func (c *CLI) newRegistryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Manage registry authentication used for image validation",
	}
	cmd.AddCommand(c.newRegistryLoginCommand())
	return cmd
}

func (c *CLI) newRegistryLoginCommand() *cobra.Command {
	cfg := registryLoginConfig{}
	cmd := &cobra.Command{
		Use:   "login <server>",
		Short: "Log in to a registry for subsequent image validation",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg.Server = args[0]
			return login.Run(c.stdin, c.stdout, c.logger(cfg.debug), cfg.Config)
		},
	}

	flags := cmd.Flags()
	flags.BoolVar(&cfg.debug, "debug", false, "Enable debug logging.")
	flags.StringVarP(&cfg.Username, "username", "u", "", "Username")
	flags.StringVarP(&cfg.Password, "password", "p", "", "Password")
	flags.BoolVar(&cfg.PasswordStdin, "password-stdin", false, "Take the password from stdin")
	return cmd
}
