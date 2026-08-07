package generate

import (
	"os"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestPrependBOMGenerationValuesFile(t *testing.T) {
	valuesFiles := []string{"values.yaml"}
	cleanup, err := prependBOMGenerationValuesFile(&valuesFiles, map[string]any{
		"image": map[string]any{
			"repository": "ghcr.io/example/api",
			"tag":        "latest",
		},
	})
	if err != nil {
		t.Fatalf("prependBOMGenerationValuesFile returned error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("expected cleanup function")
	}

	if len(valuesFiles) != 2 {
		t.Fatalf("expected 2 values files, got %d", len(valuesFiles))
	}
	if got, want := valuesFiles[1], "values.yaml"; got != want {
		t.Fatalf("unexpected values file order:\nwant second: %s\ngot:  %s", want, got)
	}

	content, err := os.ReadFile(valuesFiles[0])
	if err != nil {
		t.Fatalf("read bom generation values file: %v", err)
	}

	var payload map[string]any
	if err := yaml.Unmarshal(content, &payload); err != nil {
		t.Fatalf("unmarshal bom generation values file: %v", err)
	}

	imageValues, ok := payload["image"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested image payload, got %#v", payload["image"])
	}
	if imageValues["repository"] != "ghcr.io/example/api" {
		t.Fatalf("unexpected repository: %#v", imageValues["repository"])
	}
	if imageValues["tag"] != "latest" {
		t.Fatalf("unexpected tag: %#v", imageValues["tag"])
	}

	path := valuesFiles[0]
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected bom generation values file to be removed, stat err=%v", err)
	}
}
