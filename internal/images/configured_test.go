package images

import (
	"slices"
	"testing"

	"github.com/codesphere-cloud/bom/internal/bomrc"
)

func TestExtractConfigured(t *testing.T) {
	manifest := []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: extra-images
data:
  primary: ghcr.io/acme/primary:1.2.3
  sidecars:
    - name: metrics
      image: ghcr.io/acme/metrics:4.5.6
    - name: logs
      image: ghcr.io/acme/logs:7.8.9
`)

	refs, err := ExtractConfigured(manifest, []bomrc.AdditionalImage{
		{
			Resource: bomrc.ResourceRef{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "extra-images",
			},
			Image: bomrc.ImageValue{Literal: `.data.primary`},
		},
		{
			Resource: bomrc.ResourceRef{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "extra-images",
			},
			Key:   "metrics",
			Image: bomrc.ImageValue{Literal: `.data.sidecars[] | select(.name == "metrics") | .image`},
		},
	}, ExtractConfiguredOptions{})
	if err != nil {
		t.Fatalf("ExtractConfigured returned error: %v", err)
	}

	got := []string{refs[0].Reference, refs[1].Reference}
	slices.Sort(got)

	want := []string{
		"ghcr.io/acme/metrics:4.5.6",
		"ghcr.io/acme/primary:1.2.3",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected refs:\nwant: %v\ngot:  %v", want, got)
	}
	for _, ref := range refs {
		if ref.Reference == "ghcr.io/acme/metrics:4.5.6" && ref.Repository != "metrics" {
			t.Fatalf("expected configured key override, got repository %q", ref.Repository)
		}
	}
}

func TestExtractConfiguredSupportsDirectImageReferences(t *testing.T) {
	refs, err := ExtractConfigured(nil, []bomrc.AdditionalImage{
		{
			Image: bomrc.ImageValue{Literal: "ghcr.io/acme/external:1.2.3"},
		},
		{
			Key:   "support-tool",
			Image: bomrc.ImageValue{Literal: "docker.io/acme/support-tool:4.5.6"},
		},
	}, ExtractConfiguredOptions{ValidateExists: true})
	if err != nil {
		t.Fatalf("ExtractConfigured returned error: %v", err)
	}

	if len(refs) != 2 {
		t.Fatalf("expected 2 image references, got %d", len(refs))
	}
	if refs[0].Reference != "ghcr.io/acme/external:1.2.3" || refs[0].Repository != "ghcr.io/acme/external" {
		t.Fatalf("unexpected direct image reference: %#v", refs[0])
	}
	if refs[1].Reference != "acme/support-tool:4.5.6" || refs[1].Repository != "support-tool" {
		t.Fatalf("unexpected keyed direct image reference: %#v", refs[1])
	}
	if len(refs[0].Sources) != 1 || refs[0].Sources[0] != "configured directly by .bomrc.yaml/.yml: ghcr.io/acme/external:1.2.3" {
		t.Fatalf("unexpected direct image sources: %#v", refs[0].Sources)
	}
}

func TestExtractConfiguredSupportsStructuredDirectImageReference(t *testing.T) {
	refs, err := ExtractConfigured(nil, []bomrc.AdditionalImage{
		{
			Image: bomrc.ImageValue{
				Repository: "ghcr.io/acme/external",
				Tag:        "1.2.3",
			},
		},
	}, ExtractConfiguredOptions{ValidateExists: true})
	if err != nil {
		t.Fatalf("ExtractConfigured returned error: %v", err)
	}

	if len(refs) != 1 {
		t.Fatalf("expected 1 image reference, got %d", len(refs))
	}
	if refs[0].Reference != "ghcr.io/acme/external:1.2.3" || refs[0].Repository != "ghcr.io/acme/external" {
		t.Fatalf("unexpected structured image reference: %#v", refs[0])
	}
}

func TestExtractConfiguredSupportsStructuredImageWithLiteralFieldsAndResource(t *testing.T) {
	refs, err := ExtractConfigured([]byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: extra-images
data:
  primary: ghcr.io/acme/primary:1.2.3
`), []bomrc.AdditionalImage{
		{
			Resource: bomrc.ResourceRef{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "extra-images",
			},
			Image: bomrc.ImageValue{
				Repository: "ghcr.io/acme/external",
				Tag:        "1.2.3",
			},
		},
	}, ExtractConfiguredOptions{})
	if err != nil {
		t.Fatalf("ExtractConfigured returned error: %v", err)
	}
	if len(refs) != 1 || refs[0].Reference != "ghcr.io/acme/external:1.2.3" {
		t.Fatalf("unexpected refs: %#v", refs)
	}
}

func TestExtractConfiguredSupportsStructuredImageWithSelectorFields(t *testing.T) {
	manifest := []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: extra-images
data:
  tag: 1.2.3
  digest: sha256:1234567890123456789012345678901234567890123456789012345678901234
`)

	refs, err := ExtractConfigured(manifest, []bomrc.AdditionalImage{
		{
			Resource: bomrc.ResourceRef{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "extra-images",
			},
			Image: bomrc.ImageValue{
				Repository: "ghcr.io/acme/external",
				Tag:        ".data.tag",
			},
		},
		{
			Resource: bomrc.ResourceRef{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "extra-images",
			},
			Key: "by-digest",
			Image: bomrc.ImageValue{
				Repository: "ghcr.io/acme/external",
				Digest:     ".data.digest",
			},
		},
	}, ExtractConfiguredOptions{})
	if err != nil {
		t.Fatalf("ExtractConfigured returned error: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d: %#v", len(refs), refs)
	}

	if refs[0].Reference != "ghcr.io/acme/external:1.2.3" {
		t.Fatalf("unexpected tag-selector ref: %#v", refs[0])
	}
	if refs[1].Reference != "ghcr.io/acme/external@sha256:1234567890123456789012345678901234567890123456789012345678901234" {
		t.Fatalf("unexpected digest-selector ref: %#v", refs[1])
	}
	if refs[1].Repository != "by-digest" {
		t.Fatalf("expected key override, got repository %q", refs[1].Repository)
	}
}

func TestExtractConfiguredErrorsWhenStructuredSelectorFieldMissing(t *testing.T) {
	manifest := []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: extra-images
data: {}
`)

	_, err := ExtractConfigured(manifest, []bomrc.AdditionalImage{
		{
			Resource: bomrc.ResourceRef{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "extra-images",
			},
			Image: bomrc.ImageValue{
				Repository: "ghcr.io/acme/external",
				Tag:        ".data.missing",
			},
		},
	}, ExtractConfiguredOptions{ValidateExists: true})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExtractConfiguredRejectsInvalidDirectImageReference(t *testing.T) {
	_, err := ExtractConfigured(nil, []bomrc.AdditionalImage{
		{Image: bomrc.ImageValue{Literal: "https://example.com/image"}},
	}, ExtractConfiguredOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExtractConfiguredRejectsPartialResource(t *testing.T) {
	_, err := ExtractConfigured(nil, []bomrc.AdditionalImage{
		{
			Resource: bomrc.ResourceRef{Kind: "ConfigMap"},
			Image:    bomrc.ImageValue{Literal: "ghcr.io/acme/external:1.2.3"},
		},
	}, ExtractConfiguredOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExtractConfiguredErrorsWhenSelectorMatchesNothing(t *testing.T) {
	manifest := []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: extra-images
data:
  sidecars:
    - name: metrics
      image: ghcr.io/acme/metrics:4.5.6
`)

	_, err := ExtractConfigured(manifest, []bomrc.AdditionalImage{
		{
			Resource: bomrc.ResourceRef{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "extra-images",
			},
			Image: bomrc.ImageValue{Literal: `.data.sidecars[] | select(.name == "logs") | .image`},
		},
	}, ExtractConfiguredOptions{ValidateExists: true})
	if err == nil {
		t.Fatal("expected error")
	}
}
