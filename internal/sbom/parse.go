package sbom

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"strings"

	intcsbom "github.com/codesphere-cloud/bom/internal/csbom"
	intcsbomv2 "github.com/codesphere-cloud/bom/internal/csbom/v2"
	"github.com/codesphere-cloud/bom/internal/images"
	spdxjson "github.com/spdx/tools-golang/json"
	"sigs.k8s.io/yaml"
)

const DefaultInputFormat = "csbom-v2"

func Parse(r io.Reader, format string) (Document, error) {
	content, err := io.ReadAll(r)
	if err != nil {
		return Document{}, fmt.Errorf("read bom: %w", err)
	}

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "spdx", "spdx-json":
		return parseSPDXJSON(content)
	case "csbom", "csbom-json", "csbom-yaml":
		return parseCSBOM(content)
	case "csbom-v2", "csbom-v2-json", "csbom-v2-yaml":
		return parseCSBOMV2(content)
	default:
		return Document{}, fmt.Errorf("unsupported bom format %q", format)
	}
}

func ImageRefs(document Document) []images.ImageRef {
	refs := make([]images.ImageRef, 0, len(document.Components))
	for _, component := range document.Components {
		if component.Type == ComponentTypeHelmChart {
			continue
		}

		ref, ok := images.ParseImageRef(component.Reference)
		if !ok {
			continue
		}

		refs = append(refs, ref)
	}

	return refs
}

func OCIRefs(document Document) []images.ImageRef {
	refs := make([]images.ImageRef, 0, len(document.Components))
	for _, component := range document.Components {
		ref, ok := images.ParseImageRef(component.Reference)
		if !ok {
			continue
		}
		refs = append(refs, ref)
	}
	return refs
}

func parseSPDXJSON(content []byte) (Document, error) {
	doc, err := spdxjson.Read(bytes.NewReader(content))
	if err != nil {
		return Document{}, err
	}

	components := make([]Component, 0, len(doc.Packages))
	for _, pkg := range doc.Packages {
		if pkg == nil {
			continue
		}

		ref, ok := images.ParseImageRef(spdxPackageReference(pkg.PackageName, pkg.PackageVersion, pkg.PackageSummary))
		if !ok {
			continue
		}

		components = append(components, Component{
			Type:       ComponentTypeOCIImage,
			Repository: ref.Repository,
			Reference:  ref.Reference,
			Tag:        ref.Tag,
			Digest:     ref.Digest,
		})
	}

	if len(components) == 0 {
		return Document{}, fmt.Errorf("spdx bom does not contain OCI image packages")
	}

	return Document{Components: components}, nil
}

func spdxPackageReference(name string, version string, summary string) string {
	if summary != "" {
		return summary
	}

	if version == "" {
		return name
	}

	if strings.Contains(version, ":") {
		return name + "@" + version
	}

	return name + ":" + version
}

func parseCSBOM(content []byte) (Document, error) {
	var payload intcsbom.Config
	if err := yaml.Unmarshal(content, &payload); err != nil {
		return Document{}, err
	}

	if len(payload.Components) == 0 {
		return Document{}, fmt.Errorf("csbom does not contain components")
	}

	components := make([]Component, 0)
	for _, componentConfig := range payload.Components {
		for _, value := range componentConfig.ContainerImages {
			ref, ok := images.ParseImageRef(value)
			if !ok {
				return Document{}, fmt.Errorf("invalid image reference %q in csbom", value)
			}

			components = append(components, Component{
				Type:       ComponentTypeOCIImage,
				Repository: ref.Repository,
				Reference:  ref.Reference,
				Tag:        ref.Tag,
				Digest:     ref.Digest,
			})
		}

		for _, file := range componentConfig.Files {
			chartReference, isOCI := helmChartOCIReference(file.OciRef)
			if !isOCI {
				continue
			}
			ref, ok := images.ParseImageRef(chartReference)
			if !ok {
				return Document{}, fmt.Errorf("invalid Helm chart OCI reference %q in csbom", file.OciRef)
			}
			components = append(components, Component{
				Type:       ComponentTypeHelmChart,
				Repository: ref.Repository,
				Reference:  ref.Reference,
				Tag:        ref.Tag,
				Digest:     ref.Digest,
			})
		}
	}

	slices.SortFunc(components, func(left Component, right Component) int {
		return strings.Compare(left.Reference, right.Reference)
	})

	if len(components) == 0 {
		return Document{}, fmt.Errorf("csbom does not contain OCI images or Helm charts")
	}

	return Document{Components: components}, nil
}

func parseCSBOMV2(content []byte) (Document, error) {
	var payload intcsbomv2.BOM
	if err := yaml.Unmarshal(content, &payload); err != nil {
		return Document{}, err
	}

	if payload.Version != csbomV2Version {
		return Document{}, fmt.Errorf("unsupported csbom v2 version %q", payload.Version)
	}

	components := make([]Component, 0, len(payload.ContainerImages)+len(payload.HelmCharts))
	for repository, image := range payload.ContainerImages {
		ref, ok := images.ParseImageRef(image.Ref)
		if !ok {
			return Document{}, fmt.Errorf("invalid image reference %q in csbom v2", image.Ref)
		}

		components = append(components, Component{
			Type:       ComponentTypeOCIImage,
			Repository: repository,
			Reference:  ref.Reference,
			Tag:        ref.Tag,
			Digest:     ref.Digest,
			Evidence:   image.Sources,
		})
	}
	for repository, chart := range payload.HelmCharts {
		chartReference, isOCI := helmChartOCIReference(chart.Ref)
		if !isOCI {
			continue
		}
		ref, ok := images.ParseImageRef(chartReference)
		if !ok {
			return Document{}, fmt.Errorf("invalid Helm chart OCI reference %q in csbom v2", chart.Ref)
		}
		components = append(components, Component{
			Type:       ComponentTypeHelmChart,
			Repository: repository,
			Reference:  ref.Reference,
			Tag:        ref.Tag,
			Digest:     ref.Digest,
			Evidence:   []string{},
		})
	}

	slices.SortFunc(components, func(left Component, right Component) int {
		return strings.Compare(left.Reference, right.Reference)
	})

	return Document{Components: components}, nil
}

func helmChartOCIReference(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "oci://") {
		return "", false
	}
	return strings.TrimPrefix(value, "oci://"), true
}
