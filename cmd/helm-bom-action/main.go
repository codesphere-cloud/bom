package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/codesphere-cloud/helm-bom/internal/ghaction"
)

func main() {
	var cfg ghaction.Config

	flag.StringVar(&cfg.Mode, "mode", "", "Action mode: generate or check.")
	flag.StringVar(&cfg.Paths, "paths", "", "Newline- or comma-separated chart or BOM paths. Glob patterns are supported.")
	flag.BoolVar(&cfg.ChangedOnly, "changed-only", false, "Restrict processing to paths touched by the current PR or push.")
	flag.StringVar(&cfg.Format, "format", "spdx-json", "Generate output format.")
	flag.StringVar(&cfg.Namespace, "namespace", "default", "Helm namespace used for generation.")
	flag.StringVar(&cfg.ReleaseName, "release-name", "", "Helm release name override.")
	flag.BoolVar(&cfg.ValidateConfiguredImageExist, "validate-configured-image-exists", false, "Fail when configured additional image selectors do not resolve.")
	flag.StringVar(&cfg.RegistryServer, "registry-server", "", "Registry server to log in to before checking images.")
	flag.StringVar(&cfg.RegistryUsername, "registry-username", "", "Registry username used with registry-server.")
	flag.StringVar(&cfg.RegistryPassword, "registry-password", "", "Registry password used with registry-server.")
	flag.BoolVar(&cfg.FailOnNoMatches, "fail-on-no-matches", false, "Exit with an error when no paths match the configured filters.")
	flag.Parse()

	if err := ghaction.Run(context.Background(), cfg, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
