package images

import (
	"bytes"
	"container/list"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"

	"github.com/codesphere-cloud/bom/internal/bomrc"
	"github.com/mikefarah/yq/v4/pkg/yqlib"
	yamlv3 "gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	sigsyaml "sigs.k8s.io/yaml"
)

var (
	errConfiguredImageNotFound = errors.New("configured image not found")
	// errMissingValue reports that an expression resolved to nothing (no
	// matches, or an explicit null), as opposed to failing to evaluate.
	errMissingValue = errors.New("expression returned no value")
)

func init() {
	// yq logs decoding and traversal warnings to stderr by default.
	yqlib.GetLogger().SetLevel(slog.LevelError)
}

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

		parsed, err := parseManifestDocument(&document)
		if err != nil {
			return nil, err
		}

		documents = append(documents, parsed)
	}
}

type manifestDocument struct {
	header manifestHeader
	// raw is the YAML representation of the document, used to build the yq
	// node on demand.
	raw  []byte
	node *yqlib.CandidateNode
}

func parseManifestDocument(document *yamlv3.Node) (manifestDocument, error) {
	documentBytes, err := yamlv3Marshal(document)
	if err != nil {
		return manifestDocument{}, fmt.Errorf("marshal manifest document: %w", err)
	}

	var header manifestHeader
	if err := sigsyaml.Unmarshal(documentBytes, &header); err != nil {
		return manifestDocument{}, fmt.Errorf("decode manifest header: %w", err)
	}

	return manifestDocument{
		header: header,
		raw:    documentBytes,
	}, nil
}

// yqNode returns the document as a yq candidate node, parsing it on first use.
func (d *manifestDocument) yqNode() (*yqlib.CandidateNode, error) {
	if d.node != nil {
		return d.node, nil
	}

	documents, err := yqlib.ReadDocuments(bytes.NewReader(d.raw), yqlib.NewYamlDecoder(yqlib.ConfiguredYamlPreferences))
	if err != nil {
		return nil, fmt.Errorf("parse manifest document: %w", err)
	}
	if documents.Len() == 0 {
		return nil, fmt.Errorf("parse manifest document: no content")
	}

	node, ok := documents.Front().Value.(*yqlib.CandidateNode)
	if !ok {
		return nil, fmt.Errorf("parse manifest document: unexpected node %T", documents.Front().Value)
	}

	d.node = node
	return node, nil
}

func extractConfiguredImage(documents []manifestDocument, entry bomrc.AdditionalImage) (ImageRef, error) {
	apiVersion := strings.TrimSpace(entry.Resource.APIVersion)
	kind := strings.TrimSpace(entry.Resource.Kind)
	name := strings.TrimSpace(entry.Resource.Name)

	if apiVersion == "" && kind == "" && name == "" {
		return directConfiguredImage(entry.Image, entry.Key)
	}

	if apiVersion == "" || kind == "" || name == "" {
		return ImageRef{}, fmt.Errorf("configured image resource must set apiVersion, kind, and name")
	}

	document := findManifestDocument(documents, entry.Resource)
	if document == nil {
		return ImageRef{}, fmt.Errorf("%w: resource %s %s %s", errConfiguredImageNotFound, entry.Resource.APIVersion, entry.Resource.Kind, entry.Resource.Name)
	}

	var ref ImageRef
	var err error
	if entry.Image.Repository != "" {
		ref, err = resolveStructuredConfiguredImage(document, entry.Resource, entry.Image)
	} else {
		ref, err = resolveSelectorConfiguredImage(document, entry.Resource, entry.Image.Literal)
	}
	if err != nil {
		return ImageRef{}, err
	}

	if key := strings.TrimSpace(entry.Key); key != "" {
		ref.Repository = key
	}
	return ref, nil
}

func resolveSelectorConfiguredImage(document *manifestDocument, resource bomrc.ResourceRef, image string) (ImageRef, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return ImageRef{}, fmt.Errorf("configured image for %s/%s must define an image selector", resource.Kind, resource.Name)
	}

	value, err := evalYQString(document, image)
	if err != nil {
		if errors.Is(err, errMissingValue) {
			return ImageRef{}, fmt.Errorf("%w: %s/%s with %q", errConfiguredImageNotFound, resource.Kind, resource.Name, image)
		}
		return ImageRef{}, fmt.Errorf("resolve configured image for %s/%s with %q: %w", resource.Kind, resource.Name, image, err)
	}

	ref, ok := ParseImageRef(value)
	if !ok {
		return ImageRef{}, fmt.Errorf("configured image selector %q returned non-image value %q", image, value)
	}

	ref.Sources = []string{
		fmt.Sprintf("%s/%s configured by .bomrc.yaml/.yml: %s", resource.Kind, resource.Name, image),
	}
	return ref, nil
}

// resolveStructuredConfiguredImage builds an image reference from a
// repository/tag/digest object where each field is either a literal value
// or, when it starts with ".", a yq-style selector evaluated against the
// resource identified by resource.
func resolveStructuredConfiguredImage(document *manifestDocument, resource bomrc.ResourceRef, image bomrc.ImageValue) (ImageRef, error) {
	repository, err := resolveImageField(document, resource, "repository", image.Repository)
	if err != nil {
		return ImageRef{}, err
	}
	if repository == "" {
		return ImageRef{}, fmt.Errorf("configured image for %s/%s must define a repository", resource.Kind, resource.Name)
	}

	tag, err := resolveImageField(document, resource, "tag", image.Tag)
	if err != nil {
		return ImageRef{}, err
	}
	digest, err := resolveImageField(document, resource, "digest", image.Digest)
	if err != nil {
		return ImageRef{}, err
	}
	if tag == "" && digest == "" {
		return ImageRef{}, fmt.Errorf("configured image for %s/%s must resolve a tag and/or digest", resource.Kind, resource.Name)
	}

	value := repository
	if tag != "" {
		value += ":" + tag
	}
	if digest != "" {
		value += "@" + digest
	}

	ref, ok := ParseImageRef(value)
	if !ok {
		return ImageRef{}, fmt.Errorf("configured image %q is not a valid OCI image reference", value)
	}

	ref.Sources = []string{
		fmt.Sprintf("%s/%s configured by .bomrc.yaml/.yml: %s", resource.Kind, resource.Name, value),
	}
	return ref, nil
}

func resolveImageField(document *manifestDocument, resource bomrc.ResourceRef, field string, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !isYQSelector(value) {
		return value, nil
	}

	resolved, err := evalYQString(document, value)
	if err != nil {
		if errors.Is(err, errMissingValue) {
			return "", fmt.Errorf("%w: %s/%s %s selector %q", errConfiguredImageNotFound, resource.Kind, resource.Name, field, value)
		}
		return "", fmt.Errorf("resolve configured image %s for %s/%s with %q: %w", field, resource.Kind, resource.Name, value, err)
	}

	return resolved, nil
}

func isYQSelector(value string) bool {
	return strings.HasPrefix(value, ".")
}

func directConfiguredImage(image bomrc.ImageValue, key string) (ImageRef, error) {
	value, ok := image.Ref()
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return ImageRef{}, fmt.Errorf("configured image must define an image reference")
	}

	ref, ok := ParseImageRef(value)
	if !ok {
		return ImageRef{}, fmt.Errorf("configured image %q is not a valid OCI image reference", value)
	}
	if key = strings.TrimSpace(key); key != "" {
		ref.Repository = key
	}

	ref.Sources = []string{
		fmt.Sprintf("configured directly by .bomrc.yaml/.yml: %s", value),
	}
	return ref, nil
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

// evalYQString evaluates a yq expression against the document and requires it
// to resolve to exactly one scalar value. Any yq expression is supported; the
// evaluation is delegated to yq itself.
func evalYQString(document *manifestDocument, expression string) (string, error) {
	results, err := evalYQ(document, expression)
	if err != nil {
		return "", err
	}

	if len(results) == 0 {
		return "", errMissingValue
	}
	if len(results) != 1 {
		return "", fmt.Errorf("expression returned %d values; expected exactly 1", len(results))
	}

	node := results[0]
	if node.Tag == "!!null" {
		return "", errMissingValue
	}
	if node.Kind != yqlib.ScalarNode {
		return "", fmt.Errorf("expression returned a non-scalar value")
	}

	return strings.TrimSpace(node.Value), nil
}

func evalYQ(document *manifestDocument, expression string) ([]*yqlib.CandidateNode, error) {
	if strings.TrimSpace(expression) == "" {
		return nil, fmt.Errorf("empty expression")
	}

	node, err := document.yqNode()
	if err != nil {
		return nil, err
	}

	// yq operators may mutate the nodes they run against, so evaluate a copy to
	// keep the parsed document reusable across entries.
	matches, err := yqlib.NewAllAtOnceEvaluator().EvaluateNodes(expression, node.Copy())
	if err != nil {
		return nil, fmt.Errorf("evaluate expression: %w", err)
	}

	return candidateNodes(matches), nil
}

func candidateNodes(matches *list.List) []*yqlib.CandidateNode {
	if matches == nil {
		return nil
	}

	nodes := make([]*yqlib.CandidateNode, 0, matches.Len())
	for element := matches.Front(); element != nil; element = element.Next() {
		node, ok := element.Value.(*yqlib.CandidateNode)
		if !ok {
			continue
		}
		nodes = append(nodes, node)
	}

	return nodes
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
