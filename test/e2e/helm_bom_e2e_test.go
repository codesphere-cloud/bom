package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

type spdxDocument struct {
	Name     string        `json:"name"`
	Packages []spdxPackage `json:"packages"`
}

type spdxPackage struct {
	Name           string            `json:"name"`
	VersionInfo    string            `json:"versionInfo"`
	ExternalRefs   []spdxExternalRef `json:"externalRefs"`
	PrimaryPurpose string            `json:"primaryPackagePurpose"`
}

type spdxExternalRef struct {
	Category string `json:"referenceCategory"`
	Type     string `json:"referenceType"`
	Locator  string `json:"referenceLocator"`
}

func TestHelmBOMExtractsImagesFromChart(t *testing.T) {
	t.Parallel()

	repoRoot := repoRoot(t)
	binary := buildBinary(t, repoRoot)
	chartPath := filepath.Join(repoRoot, "testdata", "charts", "e2e")
	outputPath := filepath.Join(t.TempDir(), "sbom.json")

	cmd := exec.Command(binary, chartPath, "--output", outputPath)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run helm-bom: %v\n%s", err, output)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}

	var doc spdxDocument
	if err := json.Unmarshal(content, &doc); err != nil {
		t.Fatalf("decode SPDX output: %v", err)
	}

	if doc.Name != "e2e-chart" {
		t.Fatalf("expected document name e2e-chart, got %q", doc.Name)
	}

	got := make([]string, 0, len(doc.Packages))
	for _, pkg := range doc.Packages {
		got = append(got, pkg.Name+"@"+pkg.VersionInfo)
		if pkg.PrimaryPurpose != "CONTAINER" {
			t.Fatalf("expected package %s to have CONTAINER purpose, got %q", pkg.Name, pkg.PrimaryPurpose)
		}
	}
	slices.Sort(got)

	want := []string{
		"busybox@1.36.1",
		"ghcr.io/example/api@1.2.3",
		"quay.io/example/worker@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected extracted packages:\nwant: %v\ngot:  %v", want, got)
	}

	assertPURL(t, doc.Packages, "ghcr.io/example/api", "pkg:oci/ghcr.io/example/api@1.2.3")
	assertPURL(t, doc.Packages, "busybox", "pkg:oci/busybox@1.36.1")
	assertPURL(t, doc.Packages, "quay.io/example/worker", "pkg:oci/quay.io/example/worker@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
}

func buildBinary(t *testing.T, repoRoot string) string {
	t.Helper()

	binaryName := "helm-bom"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}

	binaryPath := filepath.Join(t.TempDir(), binaryName)
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/helm-bom")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build helm-bom binary: %v\n%s", err, output)
	}

	return binaryPath
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve caller path")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func assertPURL(t *testing.T, packages []spdxPackage, name string, want string) {
	t.Helper()

	for _, pkg := range packages {
		if pkg.Name != name {
			continue
		}
		for _, ref := range pkg.ExternalRefs {
			if ref.Category == "PACKAGE-MANAGER" && ref.Type == "purl" && ref.Locator == want {
				return
			}
		}
		t.Fatalf("package %s missing purl %q", name, want)
	}

	t.Fatalf("package %s not found in SPDX output", name)
}
