package check

import (
	"fmt"
	"os"

	"github.com/codesphere-cloud/bom/internal/bomlint"
	"github.com/codesphere-cloud/bom/internal/images"
	"github.com/codesphere-cloud/bom/internal/logging"
	"github.com/codesphere-cloud/bom/internal/sbom"
)

type Config struct {
	BOMPath   string
	BOMFormat string
}

type Validator func([]images.ImageRef) error

func Run(logger logging.Logger, cfg Config) error {
	return RunWithValidator(logger, cfg, images.ValidateReferencesExist)
}

func RunWithValidator(logger logging.Logger, cfg Config, validator Validator) error {
	if cfg.BOMFormat == "" {
		cfg.BOMFormat = sbom.DefaultInputFormat
	}

	lintConfig, configRoot, foundConfig, err := bomlint.Find(cfg.BOMPath)
	if err != nil {
		return err
	}
	if foundConfig {
		logger.Debugf("loaded %s from %s", bomlint.FileName, configRoot)
	}

	file, err := os.Open(cfg.BOMPath)
	if err != nil {
		return fmt.Errorf("open bom file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	document, err := sbom.Parse(file, cfg.BOMFormat)
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
	logger.Debugf("validated %d image reference(s) and %d Helm chart reference(s) in %s", imageCount, chartCount, cfg.BOMPath)
	return nil
}
