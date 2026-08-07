package images

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateAllowedRegistriesChecksEveryReference(t *testing.T) {
	refs := []ImageRef{
		{Reference: "ghcr.io/example/api:1.2.3"},
		{Reference: "busybox:1.36.1"},
		{Reference: "quay.io/example/chart:2.0.0"},
	}

	err := ValidateAllowedRegistries(refs, []string{"ghcr.io"})
	var disallowed *DisallowedRegistriesError
	if !errors.As(err, &disallowed) {
		t.Fatalf("expected DisallowedRegistriesError, got %v", err)
	}
	if len(disallowed.References) != 2 {
		t.Fatalf("expected both disallowed references, got %#v", disallowed.References)
	}
	if !strings.Contains(err.Error(), "docker.io") || !strings.Contains(err.Error(), "quay.io") {
		t.Fatalf("error does not identify all registries: %v", err)
	}
}

func TestValidateAllowedRegistriesAcceptsHostsAndPorts(t *testing.T) {
	refs := []ImageRef{
		{Reference: "registry.example.com:5000/team/api:1.0.0"},
		{Reference: "busybox:1.36.1"},
	}

	if err := ValidateAllowedRegistries(refs, []string{"registry.example.com:5000", "docker.io"}); err != nil {
		t.Fatalf("ValidateAllowedRegistries returned error: %v", err)
	}
}
