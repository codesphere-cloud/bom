package images

import (
	"fmt"
	"slices"
	"strings"

	"github.com/distribution/reference"
)

type DisallowedRegistriesError struct {
	References []string
	Allowed    []string
}

func (e *DisallowedRegistriesError) Error() string {
	return fmt.Sprintf(
		"%d OCI reference(s) use a registry outside the allowed registries (%s): %s",
		len(e.References),
		strings.Join(e.Allowed, ", "),
		strings.Join(e.References, ", "),
	)
}

func ValidateAllowedRegistries(refs []ImageRef, allowedRegistries []string) error {
	if len(allowedRegistries) == 0 {
		return nil
	}

	allowed := make(map[string]struct{}, len(allowedRegistries))
	normalizedAllowed := make([]string, 0, len(allowedRegistries))
	for _, value := range allowedRegistries {
		registry := strings.ToLower(strings.TrimSpace(value))
		if registry == "" {
			continue
		}
		if strings.Contains(registry, "://") || strings.Contains(registry, "/") {
			return fmt.Errorf("invalid allowed registry %q: expected a registry host with an optional port", value)
		}
		if _, exists := allowed[registry]; exists {
			continue
		}
		allowed[registry] = struct{}{}
		normalizedAllowed = append(normalizedAllowed, registry)
	}
	if len(allowed) == 0 {
		return fmt.Errorf("allowedRegistries must contain at least one registry")
	}
	slices.Sort(normalizedAllowed)

	disallowed := make([]string, 0)
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		named, err := reference.ParseNormalizedNamed(ref.Reference)
		if err != nil {
			return fmt.Errorf("parse OCI reference %q: %w", ref.Reference, err)
		}
		registry := strings.ToLower(reference.Domain(named))
		if _, ok := allowed[registry]; ok {
			continue
		}
		if _, exists := seen[ref.Reference]; exists {
			continue
		}
		seen[ref.Reference] = struct{}{}
		disallowed = append(disallowed, fmt.Sprintf("%s (registry %s)", ref.Reference, registry))
	}

	if len(disallowed) == 0 {
		return nil
	}
	slices.Sort(disallowed)
	return &DisallowedRegistriesError{References: disallowed, Allowed: normalizedAllowed}
}
