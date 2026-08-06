package images

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateReferencesExistDeduplicatesAndSorts(t *testing.T) {
	refs := []ImageRef{
		{Reference: "ghcr.io/example/z:1.0.0"},
		{Reference: "ghcr.io/example/a:1.0.0"},
		{Reference: "ghcr.io/example/z:1.0.0"},
	}

	lookups := make([]string, 0, 2)
	err := ValidateReferencesExistWithLookup(refs, func(ref string) error {
		lookups = append(lookups, ref)
		return nil
	})
	if err != nil {
		t.Fatalf("ValidateReferencesExistWithLookup returned error: %v", err)
	}

	if got, want := strings.Join(lookups, ","), "ghcr.io/example/a:1.0.0,ghcr.io/example/z:1.0.0"; got != want {
		t.Fatalf("unexpected lookup order:\nwant: %s\ngot:  %s", want, got)
	}
}

func TestValidateReferencesExistReturnsMissingImages(t *testing.T) {
	refs := []ImageRef{
		{Reference: "ghcr.io/example/a:1.0.0"},
		{Reference: "ghcr.io/example/b:2.0.0"},
	}

	err := ValidateReferencesExistWithLookup(refs, func(ref string) error {
		if ref == "ghcr.io/example/b:2.0.0" {
			return errors.New("manifest unknown")
		}

		return nil
	})
	if err == nil {
		t.Fatal("expected error")
	}

	var missing *MissingImageError
	if !errors.As(err, &missing) {
		t.Fatalf("expected MissingImageError, got %T", err)
	}

	if got, want := missing.References[0], "ghcr.io/example/b:2.0.0"; got != want {
		t.Fatalf("unexpected missing reference: %q", got)
	}
}
