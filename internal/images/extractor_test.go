package images

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Extract", func() {
	It("extracts images from containers and init containers across documents, sorted", func() {
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
		Expect(err).NotTo(HaveOccurred())

		Expect(refs).To(HaveLen(3))
		Expect(refs[0].Reference).To(Equal("busybox:1.36"))
		Expect(refs[1].Reference).To(Equal("ghcr.io/acme/api:1.2.3"))
		Expect(refs[2].Reference).To(Equal("quay.io/acme/sidecar@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"))
	})

	It("extracts images from a List of resources", func() {
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
		Expect(err).NotTo(HaveOccurred())

		Expect(refs).To(HaveLen(2))
		Expect(refs[0].Reference).To(Equal("alpine:3.20"))
		Expect(refs[1].Reference).To(Equal("busybox"))
	})
})

var _ = Describe("ParseImageRef", func() {
	DescribeTable("rejects non-image values",
		func(candidate string) {
			_, ok := ParseImageRef(candidate)
			Expect(ok).To(BeFalse())
		},
		Entry("a URL", "https://example.com"),
		Entry("a template expression", "{{ .Values.image }}"),
		Entry("an empty string", ""),
	)
})
