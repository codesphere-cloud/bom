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

func TestValidateAllowedRegistriesAcceptsRepositoryPathPrefixes(t *testing.T) {
	refs := []ImageRef{
		{Reference: "ghcr.io/example/api:1.0.0"},
		{Reference: "ghcr.io/example/platform/worker:1.0.0"},
		{Reference: "registry.example.com:5000/team/api:1.0.0"},
	}

	if err := ValidateAllowedRegistries(refs, []string{"ghcr.io/example", "registry.example.com:5000/team"}); err != nil {
		t.Fatalf("ValidateAllowedRegistries returned error: %v", err)
	}
}

func TestValidateAllowedRegistriesPathPrefixesMatchWholeSegments(t *testing.T) {
	refs := []ImageRef{
		{Reference: "ghcr.io/example/api:1.0.0"},
		{Reference: "ghcr.io/example-other/api:1.0.0"},
		{Reference: "ghcr.io/other/api:1.0.0"},
	}

	err := ValidateAllowedRegistries(refs, []string{"ghcr.io/example"})
	var disallowed *DisallowedRegistriesError
	if !errors.As(err, &disallowed) {
		t.Fatalf("expected DisallowedRegistriesError, got %v", err)
	}
	if len(disallowed.References) != 2 ||
		!strings.Contains(err.Error(), "ghcr.io/example-other/api:1.0.0") ||
		!strings.Contains(err.Error(), "ghcr.io/other/api:1.0.0") {
		t.Fatalf("unexpected disallowed references: %#v", disallowed.References)
	}
}

func TestValidateAllowedRegistriesRejectsInvalidPathPrefixes(t *testing.T) {
	for _, value := range []string{
		"https://ghcr.io/example",
		"ghcr.io/example:latest",
		"ghcr.io/example/",
	} {
		t.Run(value, func(t *testing.T) {
			if err := ValidateAllowedRegistries(nil, []string{value}); err == nil {
				t.Fatalf("expected %q to be rejected", value)
			}
		})
	}
}
