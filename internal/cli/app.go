package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/codesphere-cloud/bom/internal/bomlint"
	"github.com/codesphere-cloud/bom/internal/bomrc"
	"github.com/codesphere-cloud/bom/internal/helm"
	"github.com/codesphere-cloud/bom/internal/images"
	"github.com/codesphere-cloud/bom/internal/logging"
	"github.com/codesphere-cloud/bom/internal/sbom"
	cranecmd "github.com/google/go-containerregistry/cmd/crane/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"sigs.k8s.io/yaml"
)

const version = "dev"

type config struct {
	chartPath                    string
	debug                        bool
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
	debug   bool
}

type registryLoginConfig struct {
	server        string
	debug         bool
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

			cfg.chartPath = args[0]
			return runGenerate(cmd.OutOrStdout(), logging.NewWriterLogger(cmd.ErrOrStderr(), cfg.debug), cfg)
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
	flags.BoolVar(&cfg.debug, "debug", false, "Enable debug logging.")
	flags.StringVar(&cfg.releaseName, "release-name", "", "Helm release name. Defaults to the chart name from Chart.yaml.")
	flags.StringVar(&cfg.namespace, "namespace", "default", "Namespace passed to helm template.")
	flags.StringVar(&cfg.format, "format", "spdx-json", "Output format: spdx-json, spdx, csbom-json, csbom-yaml, csbom, csbom-v2-json, csbom-v2-yaml, or csbom-v2.")
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
			return runGenerate(cmd.OutOrStdout(), logging.NewWriterLogger(cmd.ErrOrStderr(), cfg.debug), *cfg)
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
			return runCheck(cmd.OutOrStdout(), logging.NewWriterLogger(cmd.ErrOrStderr(), cfg.debug), *cfg)
		},
	}

	cmd.SetOut(stdout)
	cmd.Flags().BoolVar(&cfg.debug, "debug", false, "Enable debug logging.")

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
			return runRegistryLogin(cmd.InOrStdin(), cmd.OutOrStdout(), logging.NewWriterLogger(cmd.ErrOrStderr(), cfg.debug), *cfg)
		},
	}

	cmd.SetOut(stdout)
	flags := cmd.Flags()
	flags.BoolVar(&cfg.debug, "debug", false, "Enable debug logging.")
	flags.StringVarP(&cfg.username, "username", "u", "", "Username")
	flags.StringVarP(&cfg.password, "password", "p", "", "Password")
	flags.BoolVar(&cfg.passwordStdin, "password-stdin", false, "Take the password from stdin")

	return cmd
}

func runGenerate(stdout io.Writer, logger logging.Logger, cfg config) error {
	bomConfig, err := bomrc.Load(cfg.chartPath)
	if err != nil {
		return err
	}

	chartName, err := helm.ChartName(cfg.chartPath)
	if err != nil {
		return err
	}

	if cfg.releaseName == "" {
		cfg.releaseName = chartName
	}

	valuesFiles := append([]string(nil), cfg.valuesFiles...)
	cleanup, err := prependBOMGenerationValuesFile(&valuesFiles, bomConfig.BOMGenerationValues)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	renderer := helm.Renderer{}
	manifest, err := renderer.Template(helm.TemplateRequest{
		ChartPath:   cfg.chartPath,
		ReleaseName: cfg.releaseName,
		Namespace:   cfg.namespace,
		ValuesFiles: valuesFiles,
		SetValues:   cfg.setValues,
		SetStrings:  cfg.setStrings,
		ExtraArgs:   cfg.helmArgs,
		Debug:       cfg.debug,
	})
	if err != nil {
		return err
	}

	refs, err := images.Extract(manifest)
	if err != nil {
		return err
	}
	refs = images.MapRepositories(refs, bomConfig.ImageKeyMappings)

	configuredRefs, err := images.ExtractConfigured(manifest, bomConfig.AdditionalImages, images.ExtractConfiguredOptions{
		ValidateExists: cfg.validateConfiguredImageExist,
	})
	if err != nil {
		return err
	}

	document := sbom.Document{
		Metadata: sbom.Metadata{
			Tool: sbom.ToolMetadata{
				Name:    "bom",
				Version: version,
			},
			Source: sbom.SourceMetadata{
				Chart:       cfg.chartPath,
				ChartName:   chartName,
				ReleaseName: cfg.releaseName,
				Namespace:   cfg.namespace,
				ValuesFiles: valuesFiles,
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
		logger.Debugf("writing SBOM output to %s", cfg.outputPath)
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

func prependBOMGenerationValuesFile(valuesFiles *[]string, bomGenerationValues map[string]any) (func(), error) {
	if len(bomGenerationValues) == 0 {
		return nil, nil
	}

	content, err := yaml.Marshal(bomGenerationValues)
	if err != nil {
		return nil, fmt.Errorf("marshal .bomrc bomGenerationValues: %w", err)
	}

	file, err := os.CreateTemp("", "bom-generation-values-*.yaml")
	if err != nil {
		return nil, fmt.Errorf("create bom generation values file: %w", err)
	}

	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, fmt.Errorf("write bom generation values file: %w", err)
	}

	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return nil, fmt.Errorf("close bom generation values file: %w", err)
	}

	*valuesFiles = append([]string{file.Name()}, *valuesFiles...)

	return func() {
		_ = os.Remove(file.Name())
	}, nil
}

func runCheck(stdout io.Writer, logger logging.Logger, cfg checkConfig) error {
	return runCheckWithValidator(stdout, logger, cfg, images.ValidateReferencesExist)
}

func runCheckWithValidator(stdout io.Writer, logger logging.Logger, cfg checkConfig, validator func([]images.ImageRef) error) error {
	lintConfig, configRoot, foundConfig, err := bomlint.Find(cfg.bomPath)
	if err != nil {
		return err
	}
	if foundConfig {
		logger.Debugf("loaded %s from %s", bomlint.FileName, configRoot)
	}

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

	refs := sbom.OCIRefs(document)
	if err := images.ValidateAllowedRegistries(refs, lintConfig.AllowedRegistries); err != nil {
		return err
	}
	if err := validator(refs); err != nil {
		return err
	}

	imageCount := 0
	chartCount := 0
	for _, component := range document.Components {
		if component.Type == sbom.ComponentTypeHelmChart {
			chartCount++
		} else {
			imageCount++
		}
	}
	logger.Debugf("validated %d image reference(s) and %d Helm chart reference(s) in %s", imageCount, chartCount, file)
	return nil
}

func runRegistryLogin(stdin io.Reader, stdout io.Writer, logger logging.Logger, cfg registryLoginConfig) error {
	restore, err := prepareCraneLoginStdin(stdin, cfg.passwordStdin)
	if err != nil {
		return err
	}
	if restore != nil {
		defer restore()
	}

	cmd := cranecmd.NewCmdAuthLogin("bom registry")
	cmd.SetOut(stdout)
	cmd.SetErr(logger.Writer())
	cmd.SetIn(stdin)
	logger.Debugf("logging in to registry %s", cfg.server)

	args := []string{cfg.server, "--username", cfg.username}
	if cfg.passwordStdin {
		args = append(args, "--password-stdin")
	} else {
		args = append(args, "--password", cfg.password)
	}

	cmd.SetArgs(args)
	return cmd.Execute()
}

func prepareCraneLoginStdin(stdin io.Reader, passwordStdin bool) (func(), error) {
	if !passwordStdin {
		return nil, nil
	}

	file, err := os.CreateTemp("", "bom-registry-login-*")
	if err != nil {
		return nil, err
	}

	if _, err := io.Copy(file, stdin); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, err
	}

	if _, err := file.Seek(0, 0); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, err
	}

	original := os.Stdin
	os.Stdin = file

	return func() {
		os.Stdin = original
		_ = file.Close()
		_ = os.Remove(file.Name())
	}, nil
}
