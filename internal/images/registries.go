package images

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/distribution/reference"
)

var allowedRegistryDomainRegexp = regexp.MustCompile("^(?:" + reference.DomainRegexp.String() + ")$")

type DisallowedRegistriesError struct {
	References []string
	Allowed    []string
}

type allowedRegistry struct {
	domain     string
	pathPrefix string
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

	allowed := make(map[string]allowedRegistry, len(allowedRegistries))
	normalizedAllowed := make([]string, 0, len(allowedRegistries))
	for _, value := range allowedRegistries {
		registryPath := strings.ToLower(strings.TrimSpace(value))
		if registryPath == "" {
			continue
		}
		if strings.Contains(registryPath, "://") {
			return fmt.Errorf("invalid allowed registry %q: expected a registry host with an optional port and path prefix", value)
		}

		parts := strings.SplitN(registryPath, "/", 2)
		entry := allowedRegistry{domain: parts[0]}
		if len(parts) == 2 {
			entry.pathPrefix = parts[1]
		}
		if !allowedRegistryDomainRegexp.MatchString(entry.domain) || !validRepositoryPath(entry) {
			return fmt.Errorf("invalid allowed registry %q: expected a registry host with an optional port and path prefix", value)
		}
		if _, exists := allowed[registryPath]; exists {
			continue
		}
		allowed[registryPath] = entry
		normalizedAllowed = append(normalizedAllowed, registryPath)
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
		path := strings.ToLower(reference.Path(named))
		if slices.ContainsFunc(normalizedAllowed, func(value string) bool {
			entry := allowed[value]
			return entry.domain == registry && (entry.pathPrefix == "" || path == entry.pathPrefix || strings.HasPrefix(path, entry.pathPrefix+"/"))
		}) {
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

func validRepositoryPath(entry allowedRegistry) bool {
	if entry.pathPrefix == "" {
		return true
	}
	named, err := reference.WithName(entry.domain + "/" + entry.pathPrefix)
	return err == nil && reference.Domain(named) == entry.domain && reference.Path(named) == entry.pathPrefix
}
