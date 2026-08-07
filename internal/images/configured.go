package images

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/codesphere-cloud/helm-bom/internal/bomrc"
	yamlv3 "gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	sigsyaml "sigs.k8s.io/yaml"
)

var errConfiguredImageNotFound = errors.New("configured image not found")

type ExtractConfiguredOptions struct {
	ValidateExists bool
}

func ExtractConfigured(manifest []byte, entries []bomrc.AdditionalImage, opts ExtractConfiguredOptions) ([]ImageRef, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	documents, err := manifestDocuments(manifest)
	if err != nil {
		return nil, err
	}

	refs := make([]ImageRef, 0, len(entries))
	for _, entry := range entries {
		ref, err := extractConfiguredImage(documents, entry)
		if err != nil {
			if !opts.ValidateExists && errors.Is(err, errConfiguredImageNotFound) {
				continue
			}
			return nil, err
		}
		refs = append(refs, ref)
	}

	return refs, nil
}

func manifestDocuments(manifest []byte) ([]manifestDocument, error) {
	decoder := yamlv3.NewDecoder(bytes.NewReader(manifest))
	documents := []manifestDocument{}

	for {
		var document yamlv3.Node
		err := decoder.Decode(&document)
		if err != nil {
			if err == io.EOF {
				return documents, nil
			}
			return nil, fmt.Errorf("decode manifest document: %w", err)
		}

		if len(document.Content) == 0 {
			continue
		}

		header, object, err := parseManifestDocument(&document)
		if err != nil {
			return nil, err
		}

		documents = append(documents, manifestDocument{
			header: header,
			object: object,
		})
	}
}

type manifestDocument struct {
	header manifestHeader
	object any
}

func parseManifestDocument(document *yamlv3.Node) (manifestHeader, any, error) {
	documentBytes, err := yamlv3Marshal(document)
	if err != nil {
		return manifestHeader{}, nil, fmt.Errorf("marshal manifest document: %w", err)
	}

	var header manifestHeader
	if err := sigsyaml.Unmarshal(documentBytes, &header); err != nil {
		return manifestHeader{}, nil, fmt.Errorf("decode manifest header: %w", err)
	}

	var object any
	if err := sigsyaml.Unmarshal(documentBytes, &object); err != nil {
		return manifestHeader{}, nil, fmt.Errorf("decode manifest object: %w", err)
	}

	return header, object, nil
}

func extractConfiguredImage(documents []manifestDocument, entry bomrc.AdditionalImage) (ImageRef, error) {
	if strings.TrimSpace(entry.Resource.APIVersion) == "" || strings.TrimSpace(entry.Resource.Kind) == "" || strings.TrimSpace(entry.Resource.Name) == "" {
		return ImageRef{}, fmt.Errorf("configured image resource must set apiVersion, kind, and name")
	}
	if strings.TrimSpace(entry.Image) == "" {
		return ImageRef{}, fmt.Errorf("configured image for %s/%s must define an image selector", entry.Resource.Kind, entry.Resource.Name)
	}

	document := findManifestDocument(documents, entry.Resource)
	if document == nil {
		return ImageRef{}, fmt.Errorf("%w: resource %s %s %s", errConfiguredImageNotFound, entry.Resource.APIVersion, entry.Resource.Kind, entry.Resource.Name)
	}

	value, err := evalYQSelect(document.object, entry.Image)
	if err != nil {
		if isMissingValueError(err) {
			return ImageRef{}, fmt.Errorf("%w: %s/%s with %q", errConfiguredImageNotFound, entry.Resource.Kind, entry.Resource.Name, entry.Image)
		}
		return ImageRef{}, fmt.Errorf("resolve configured image for %s/%s with %q: %w", entry.Resource.Kind, entry.Resource.Name, entry.Image, err)
	}

	ref, ok := ParseImageRef(value)
	if !ok {
		return ImageRef{}, fmt.Errorf("configured image selector %q returned non-image value %q", entry.Image, value)
	}
	if key := strings.TrimSpace(entry.Key); key != "" {
		ref.Repository = key
	}

	ref.Sources = []string{
		fmt.Sprintf("%s/%s configured by .bomrc.yaml/.yml: %s", entry.Resource.Kind, entry.Resource.Name, entry.Image),
	}
	return ref, nil
}

func isMissingValueError(err error) bool {
	message := err.Error()
	return message == "expression returned no values" || strings.HasPrefix(message, "field ")
}

func findManifestDocument(documents []manifestDocument, resource bomrc.ResourceRef) *manifestDocument {
	for idx := range documents {
		header := documents[idx].header
		if header.APIVersion == resource.APIVersion && header.Kind == resource.Kind && header.Metadata.Name == resource.Name {
			return &documents[idx]
		}
	}

	return nil
}

func evalYQSelect(object any, expression string) (string, error) {
	stages := splitPipeline(expression)
	if len(stages) == 0 {
		return "", fmt.Errorf("empty expression")
	}

	current := []any{object}
	for _, stage := range stages {
		var err error
		switch {
		case strings.HasPrefix(stage, "."):
			current, err = evalPathStage(current, stage)
		case strings.HasPrefix(stage, "select("):
			current, err = evalSelectStage(current, stage)
		default:
			return "", fmt.Errorf("unsupported stage %q", stage)
		}
		if err != nil {
			return "", err
		}
	}

	if len(current) == 0 {
		return "", fmt.Errorf("expression returned no values")
	}
	if len(current) != 1 {
		return "", fmt.Errorf("expression returned %d values; expected exactly 1", len(current))
	}

	value, ok := current[0].(string)
	if !ok {
		return "", fmt.Errorf("expression returned %T; expected string", current[0])
	}

	return strings.TrimSpace(value), nil
}

func splitPipeline(expression string) []string {
	parts := strings.Split(expression, "|")
	stages := make([]string, 0, len(parts))
	for _, part := range parts {
		stage := strings.TrimSpace(part)
		if stage == "" {
			continue
		}
		stages = append(stages, stage)
	}
	return stages
}

func evalPathStage(values []any, stage string) ([]any, error) {
	if stage == "." {
		return values, nil
	}

	segments := strings.Split(strings.TrimPrefix(stage, "."), ".")
	current := values

	for _, segment := range segments {
		if segment == "" {
			return nil, fmt.Errorf("invalid path stage %q", stage)
		}

		iterate := strings.HasSuffix(segment, "[]")
		key := strings.TrimSuffix(segment, "[]")
		if key == "" {
			return nil, fmt.Errorf("invalid path segment %q", segment)
		}

		next := []any{}
		for _, value := range current {
			object, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("cannot access .%s on %T", key, value)
			}

			child, ok := object[key]
			if !ok {
				return nil, fmt.Errorf("field %q not found", key)
			}

			if !iterate {
				next = append(next, child)
				continue
			}

			items, ok := child.([]any)
			if !ok {
				return nil, fmt.Errorf("field %q is %T, expected array", key, child)
			}
			next = append(next, items...)
		}

		current = next
	}

	return current, nil
}

func evalSelectStage(values []any, stage string) ([]any, error) {
	path, want, err := parseSelectStage(stage)
	if err != nil {
		return nil, err
	}
	filtered := []any{}

	for _, value := range values {
		resolved, err := evalYQSelectValue(value, path)
		if err != nil {
			return nil, err
		}
		text, ok := resolved.(string)
		if ok && text == want {
			filtered = append(filtered, value)
		}
	}

	return filtered, nil
}

func parseSelectStage(stage string) (string, string, error) {
	body := strings.TrimSpace(stage)
	if !strings.HasPrefix(body, "select(") || !strings.HasSuffix(body, ")") {
		return "", "", fmt.Errorf("unsupported select stage %q", stage)
	}

	body = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(body, "select("), ")"))
	parts := strings.SplitN(body, "==", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unsupported select stage %q", stage)
	}

	path := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	if !strings.HasPrefix(path, ".") {
		return "", "", fmt.Errorf("unsupported select path %q", path)
	}
	if len(value) < 2 {
		return "", "", fmt.Errorf("unsupported select value %q", value)
	}

	quote := value[0]
	if (quote != '"' && quote != '\'') || value[len(value)-1] != quote {
		return "", "", fmt.Errorf("unsupported select value %q", value)
	}

	return path, value[1 : len(value)-1], nil
}

func evalYQSelectValue(value any, path string) (any, error) {
	resolved, err := evalPathStage([]any{value}, path)
	if err != nil {
		return nil, err
	}
	if len(resolved) != 1 {
		return nil, fmt.Errorf("path %q returned %d values; expected exactly 1", path, len(resolved))
	}
	return resolved[0], nil
}

func Merge(refs ...[]ImageRef) []ImageRef {
	found := map[string]*ImageRef{}

	for _, group := range refs {
		for _, ref := range group {
			for _, source := range ref.Sources {
				recordRef(ref, source, found)
			}
		}
	}

	merged := make([]ImageRef, 0, len(found))
	for _, ref := range found {
		merged = append(merged, *ref)
	}

	sortImageRefs(merged)
	return merged
}

func sortImageRefs(refs []ImageRef) {
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].Reference < refs[j].Reference
	})
}

type manifestHeader struct {
	APIVersion string            `yaml:"apiVersion"`
	Kind       string            `yaml:"kind"`
	Metadata   metav1.ObjectMeta `yaml:"metadata"`
}
