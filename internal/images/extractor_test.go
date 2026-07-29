package images

import (
	"testing"
)

func TestExtractorExtract(t *testing.T) {
	manifest := []byte(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  template:
    spec:
      containers:
        - name: api
          image: ghcr.io/acme/api:1.2.3
      initContainers:
        - name: migrate
          image: docker.io/library/busybox:1.36
---
apiVersion: batch/v1
kind: CronJob
metadata:
  name: nightly
spec:
  schedule: "0 0 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          containers:
            - name: sidecar
              image: quay.io/acme/sidecar@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
          restartPolicy: OnFailure
`)

	refs, err := Extract(manifest)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}

	if len(refs) != 3 {
		t.Fatalf("expected 3 images, got %d", len(refs))
	}

	if refs[0].Reference != "busybox:1.36" {
		t.Fatalf("unexpected first reference: %s", refs[0].Reference)
	}

	if refs[1].Reference != "ghcr.io/acme/api:1.2.3" {
		t.Fatalf("unexpected second reference: %s", refs[1].Reference)
	}

	if refs[2].Reference != "quay.io/acme/sidecar@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("unexpected third reference: %s", refs[2].Reference)
	}
}

func TestParseImageRefRejectsNonImages(t *testing.T) {
	cases := []string{
		"https://example.com",
		"{{ .Values.image }}",
		"",
	}

	for _, candidate := range cases {
		if _, ok := ParseImageRef(candidate); ok {
			t.Fatalf("expected %q to be rejected", candidate)
		}
	}
}

func TestExtractorExtractList(t *testing.T) {
	manifest := []byte(`
apiVersion: v1
kind: List
items:
  - apiVersion: v1
    kind: Pod
    metadata:
      name: shell
    spec:
      containers:
        - name: shell
          image: busybox
      initContainers:
        - name: init
          image: alpine:3.20
`)

	refs, err := Extract(manifest)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}

	if len(refs) != 2 {
		t.Fatalf("expected 2 images, got %d", len(refs))
	}

	if refs[0].Reference != "alpine:3.20" {
		t.Fatalf("unexpected first reference: %s", refs[0].Reference)
	}

	if refs[1].Reference != "busybox" {
		t.Fatalf("unexpected second reference: %s", refs[1].Reference)
	}
}
