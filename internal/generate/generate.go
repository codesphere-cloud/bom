package generate

import (
	"fmt"
	"io"
	"os"

	"github.com/codesphere-cloud/bom/internal/bomrc"
	"github.com/codesphere-cloud/bom/internal/helm"
	"github.com/codesphere-cloud/bom/internal/images"
	"github.com/codesphere-cloud/bom/internal/imagesbom"
	"github.com/codesphere-cloud/bom/internal/logging"
	"github.com/codesphere-cloud/bom/internal/sbom"
	"sigs.k8s.io/yaml"
)

const defaultToolVersion = "dev"

type Config struct {
	ChartPath                    string
	ReleaseName                  string
	Namespace                    string
	Format                       string
	OutputPath                   string
	ValuesFiles                  []string
	SetValues                    []string
	SetStrings                   []string
	HelmArgs                     []string
	Debug                        bool
	SBOM                         bool
	Cosign                       bool
	ValidateConfiguredImageExist bool
	ToolVersion                  string
}

func Run(stdout io.Writer, logger logging.Logger, cfg Config) error {
	bomConfig, err := bomrc.Load(cfg.ChartPath)
	if err != nil {
		return err
	}

	chartName, err := helm.ChartName(cfg.ChartPath)
	if err != nil {
		return err
	}

	if cfg.ReleaseName == "" {
		cfg.ReleaseName = chartName
	}
	if cfg.ToolVersion == "" {
		cfg.ToolVersion = defaultToolVersion
	}

	valuesFiles := append([]string(nil), cfg.ValuesFiles...)
	cleanup, err := prependBOMGenerationValuesFile(&valuesFiles, bomConfig.BOMGenerationValues)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	renderer := helm.Renderer{}
	manifest, err := renderer.Template(helm.TemplateRequest{
		ChartPath:   cfg.ChartPath,
		ReleaseName: cfg.ReleaseName,
		Namespace:   cfg.Namespace,
		ValuesFiles: valuesFiles,
		SetValues:   cfg.SetValues,
		SetStrings:  cfg.SetStrings,
		ExtraArgs:   cfg.HelmArgs,
		Debug:       cfg.Debug,
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
		ValidateExists: cfg.ValidateConfiguredImageExist,
	})
	if err != nil {
		return err
	}

	mergedRefs := images.Merge(refs, configuredRefs)
	if cfg.SBOM {
		if err := imagesbom.Generate(logger, mergedRefs, cfg.ChartPath, cfg.Cosign); err != nil {
			return err
		}
	}

	document := sbom.Document{
		Metadata: sbom.Metadata{
			Tool: sbom.ToolMetadata{
				Name:    "bom",
				Version: cfg.ToolVersion,
			},
			Source: sbom.SourceMetadata{
				Chart:       cfg.ChartPath,
				ChartName:   chartName,
				ReleaseName: cfg.ReleaseName,
				Namespace:   cfg.Namespace,
				ValuesFiles: valuesFiles,
				SetValues:   cfg.SetValues,
				SetStrings:  cfg.SetStrings,
				HelmArgs:    cfg.HelmArgs,
			},
		},
		Components: sbom.ComponentsFromImages(mergedRefs),
	}

	formatter, err := sbom.NewFormatter(cfg.Format)
	if err != nil {
		return err
	}

	if cfg.OutputPath == "" {
		return formatter.Format(stdout, document)
	}

	logger.Debugf("writing SBOM output to %s", cfg.OutputPath)
	file, err := os.Create(cfg.OutputPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	return formatter.Format(file, document)
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
