package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/codesphere-cloud/helm-bom/internal/bomrc"
	"github.com/codesphere-cloud/helm-bom/internal/helm"
	"github.com/codesphere-cloud/helm-bom/internal/images"
	"github.com/codesphere-cloud/helm-bom/internal/sbom"
	dockerconfig "github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/types"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const version = "dev"

type config struct {
	chartPath                    string
	releaseName                  string
	namespace                    string
	format                       string
	outputPath                   string
	valuesFiles                  []string
	setValues                    []string
	setStrings                   []string
	helmArgs                     []string
	validateConfiguredImageExist bool
}

type checkConfig struct {
	bomPath string
}

type registryLoginConfig struct {
	server        string
	username      string
	password      string
	passwordStdin bool
}

func Run(args []string, stdout io.Writer, stderr io.Writer) error {
	cmd := NewRootCommand(stdout, stderr)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func NewRootCommand(stdout io.Writer, stderr io.Writer) *cobra.Command {
	var cfg config

	cmd := &cobra.Command{
		Use:   "helm-bom [chart]",
		Short: "Generate and validate SBOMs for OCI images referenced by Helm charts",
		Long: "helm-bom renders a Helm chart with helm template, extracts OCI image references " +
			"from supported Kubernetes workload resources, writes the result in either SPDX JSON " +
			"or the internal csbom JSON/YAML formats, and can validate a generated BOM against upstream registries.",
		Example: "" +
			"  helm-bom ./chart\n" +
			"  helm-bom generate ./chart --values values.yaml --set image.tag=1.2.3\n" +
			"  helm-bom generate ./chart --format csbom-json --output bom.json\n" +
			"  helm-bom check bom.json\n" +
			"  helm-bom generate ./chart --release-name my-release --namespace production",
		Args: cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}

			cfg.chartPath = args[0]
			return runGenerate(cmd.OutOrStdout(), cfg)
		},
	}

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetVersionTemplate("{{.Version}}\n")
	cmd.Version = version
	cmd.DisableFlagsInUseLine = true

	addGenerateFlags(cmd.Flags(), &cfg)

	cmd.AddCommand(newGenerateCommand(stdout, &cfg))
	cmd.AddCommand(newCheckCommand(stdout, &checkConfig{}))
	cmd.AddCommand(newRegistryCommand(stdout))

	return cmd
}

func addGenerateFlags(flags *pflag.FlagSet, cfg *config) {
	flags.StringVar(&cfg.releaseName, "release-name", "", "Helm release name. Defaults to the chart name from Chart.yaml.")
	flags.StringVar(&cfg.namespace, "namespace", "default", "Namespace passed to helm template.")
	flags.StringVar(&cfg.format, "format", "spdx-json", "Output format: spdx-json, spdx, csbom-json, csbom-yaml, or csbom.")
	flags.StringVarP(&cfg.outputPath, "output", "o", "", "Write output to a file instead of stdout.")
	flags.StringSliceVar(&cfg.valuesFiles, "values", nil, "Additional Helm values files. May be specified multiple times.")
	flags.StringSliceVar(&cfg.setValues, "set", nil, "Helm --set overrides. May be specified multiple times.")
	flags.StringSliceVar(&cfg.setStrings, "set-string", nil, "Helm --set-string overrides. May be specified multiple times.")
	flags.StringSliceVar(&cfg.helmArgs, "helm-arg", nil, "Additional raw arguments appended to helm template, for example --helm-arg=--include-crds.")
	flags.BoolVar(&cfg.validateConfiguredImageExist, "validate-configured-image-exists", false, "Fail when a configured additional image resource or selector does not resolve from the rendered manifest.")
}

func newGenerateCommand(stdout io.Writer, cfg *config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate <chart>",
		Short: "Render a Helm chart and generate an SBOM for referenced OCI images",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return cobra.ExactArgs(1)(cmd, args)
			}
			cfg.chartPath = args[0]
			return nil
		},
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerate(cmd.OutOrStdout(), *cfg)
		},
	}

	cmd.SetOut(stdout)
	addGenerateFlags(cmd.Flags(), cfg)

	return cmd
}

func newCheckCommand(stdout io.Writer, cfg *checkConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check <bom>",
		Short: "Validate that every image in a BOM exists in its upstream registry",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return cobra.ExactArgs(1)(cmd, args)
			}
			cfg.bomPath = args[0]
			return nil
		},
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCheck(cmd.OutOrStdout(), *cfg)
		},
	}

	cmd.SetOut(stdout)

	return cmd
}

func newRegistryCommand(stdout io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Manage registry authentication used for image validation",
	}

	cmd.SetOut(stdout)
	cmd.AddCommand(newRegistryLoginCommand(stdout, &registryLoginConfig{}))

	return cmd
}

func newRegistryLoginCommand(stdout io.Writer, cfg *registryLoginConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login <server>",
		Short: "Log in to a registry for subsequent image validation",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return cobra.ExactArgs(1)(cmd, args)
			}

			cfg.server = args[0]
			return nil
		},
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRegistryLogin(cmd.InOrStdin(), cmd.OutOrStdout(), *cfg, os.Getenv("DOCKER_CONFIG"))
		},
	}

	cmd.SetOut(stdout)
	flags := cmd.Flags()
	flags.StringVarP(&cfg.username, "username", "u", "", "Username")
	flags.StringVarP(&cfg.password, "password", "p", "", "Password")
	flags.BoolVar(&cfg.passwordStdin, "password-stdin", false, "Take the password from stdin")

	return cmd
}

func runGenerate(stdout io.Writer, cfg config) error {
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

	bomConfig, err := bomrc.Load(cfg.chartPath)
	if err != nil {
		return err
	}

	configuredRefs, err := images.ExtractConfigured(manifest, bomConfig.AdditionalImages, images.ExtractConfiguredOptions{
		ValidateExists: cfg.validateConfiguredImageExist,
	})
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
		Components: sbom.ComponentsFromImages(images.Merge(refs, configuredRefs)),
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

func runCheck(stdout io.Writer, cfg checkConfig) error {
	return runCheckWithValidator(stdout, cfg, images.ValidateReferencesExist)
}

func runCheckWithValidator(stdout io.Writer, cfg checkConfig, validator func([]images.ImageRef) error) error {
	file, err := os.Open(cfg.bomPath)
	if err != nil {
		return fmt.Errorf("open bom file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	document, err := sbom.Parse(file)
	if err != nil {
		return err
	}

	refs := sbom.ImageRefs(document)
	if err := validator(refs); err != nil {
		return err
	}

	_, err = fmt.Fprintf(stdout, "validated %d image reference(s)\n", len(refs))
	return err
}

func runRegistryLogin(stdin io.Reader, stdout io.Writer, cfg registryLoginConfig, dockerConfigDir string) error {
	if cfg.passwordStdin {
		contents, err := io.ReadAll(stdin)
		if err != nil {
			return fmt.Errorf("read password from stdin: %w", err)
		}

		cfg.password = strings.TrimRight(string(contents), "\r\n")
	}

	if cfg.username == "" || cfg.password == "" {
		return errors.New("username and password required")
	}

	serverAddress, err := normalizeRegistryServer(cfg.server)
	if err != nil {
		return err
	}

	cf, err := dockerconfig.Load(dockerConfigDir)
	if err != nil {
		return err
	}

	credentialKey := serverAddress
	if serverAddress == name.DefaultRegistry {
		credentialKey = authn.DefaultAuthKey
	}

	creds := cf.GetCredentialsStore(serverAddress)
	if err := creds.Store(types.AuthConfig{
		ServerAddress: credentialKey,
		Username:      cfg.username,
		Password:      cfg.password,
	}); err != nil {
		return err
	}

	if err := cf.Save(); err != nil {
		return err
	}

	_, err = fmt.Fprintf(stdout, "logged in to %s via %s\n", serverAddress, cf.Filename)
	return err
}

func normalizeRegistryServer(server string) (string, error) {
	registry, err := name.NewRegistry(server)
	if err != nil {
		return "", fmt.Errorf("parse registry %q: %w", server, err)
	}

	return registry.Name(), nil
}
