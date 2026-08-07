package images

import "testing"

func TestMapRepositoriesAppliesExactMappings(t *testing.T) {
	refs := []ImageRef{
		{
			Reference:  "ghcr.io/example/api:1.2.3",
			Repository: "ghcr.io/example/api",
			Tag:        "1.2.3",
		},
		{
			Reference:  "busybox:1.36",
			Repository: "busybox",
			Tag:        "1.36",
		},
	}

	got := MapRepositories(refs, map[string]string{
		"ghcr.io/example/api": "api",
	})

	if got[0].Repository != "api" {
		t.Fatalf("expected mapped repository, got %q", got[0].Repository)
	}
	if got[1].Repository != "busybox" {
		t.Fatalf("expected untouched repository, got %q", got[1].Repository)
	}
	if refs[0].Repository != "ghcr.io/example/api" {
		t.Fatalf("expected original refs to remain unchanged, got %q", refs[0].Repository)
	}
}
