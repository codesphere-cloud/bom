package images

func MapRepositories(refs []ImageRef, mappings map[string]string) []ImageRef {
	if len(refs) == 0 || len(mappings) == 0 {
		return refs
	}

	mapped := make([]ImageRef, len(refs))
	copy(mapped, refs)

	for idx := range mapped {
		repository := mapped[idx].Repository
		if replacement, ok := mappings[repository]; ok && replacement != "" {
			mapped[idx].Repository = replacement
		}
	}

	return mapped
}
