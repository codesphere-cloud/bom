package images

import (
	"slices"
	"testing"

	"github.com/codesphere-cloud/helm-bom/internal/bomrc"
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
			Image: `.data.primary`,
		},
		{
			Resource: bomrc.ResourceRef{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "extra-images",
			},
			Key:   "metrics",
			Image: `.data.sidecars[] | select(.name == "metrics") | .image`,
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
			Image: `.data.sidecars[] | select(.name == "logs") | .image`,
		},
	}, ExtractConfiguredOptions{ValidateExists: true})
	if err == nil {
		t.Fatal("expected error")
	}
}
