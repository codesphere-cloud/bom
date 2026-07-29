package images

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	yamlv3 "gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	sigsyaml "sigs.k8s.io/yaml"
)

type manifestHeader struct {
	APIVersion string            `yaml:"apiVersion"`
	Kind       string            `yaml:"kind"`
	Metadata   metav1.ObjectMeta `yaml:"metadata"`
}

type listObject struct {
	Items []json.RawMessage `json:"items"`
}

func Extract(manifest []byte) ([]ImageRef, error) {
	decoder := yamlv3.NewDecoder(bytes.NewReader(manifest))
	found := map[string]*ImageRef{}

	for {
		var document yamlv3.Node
		err := decoder.Decode(&document)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode manifest document: %w", err)
		}

		if len(document.Content) == 0 {
			continue
		}

		if err := extractDocument(&document, found); err != nil {
			return nil, err
		}
	}

	refs := make([]ImageRef, 0, len(found))
	for _, ref := range found {
		sort.Strings(ref.Sources)
		refs = append(refs, *ref)
	}

	sort.Slice(refs, func(i, j int) bool {
		return refs[i].Reference < refs[j].Reference
	})

	return refs, nil
}

func extractDocument(document *yamlv3.Node, found map[string]*ImageRef) error {
	documentBytes, err := yamlv3Marshal(document)
	if err != nil {
		return fmt.Errorf("marshal manifest document: %w", err)
	}

	var header manifestHeader
	if err := sigsyaml.Unmarshal(documentBytes, &header); err != nil {
		return fmt.Errorf("decode manifest header: %w", err)
	}

	switch header.Kind {
	case "Pod":
		var object corev1.Pod
		if err := sigsyaml.Unmarshal(documentBytes, &object); err != nil {
			return fmt.Errorf("decode %s: %w", header.Kind, err)
		}
		collectPodSpec(found, header.Kind, object.Name, object.Spec)
		return nil
	case "PodTemplate":
		var object corev1.PodTemplate
		if err := sigsyaml.Unmarshal(documentBytes, &object); err != nil {
			return fmt.Errorf("decode %s: %w", header.Kind, err)
		}
		collectPodSpec(found, header.Kind, object.Name, object.Template.Spec)
		return nil
	case "ReplicationController":
		var object corev1.ReplicationController
		if err := sigsyaml.Unmarshal(documentBytes, &object); err != nil {
			return fmt.Errorf("decode %s: %w", header.Kind, err)
		}
		if object.Spec.Template != nil {
			collectPodSpec(found, header.Kind, object.Name, object.Spec.Template.Spec)
		}
		return nil
	case "Deployment":
		var object appsv1.Deployment
		if err := sigsyaml.Unmarshal(documentBytes, &object); err != nil {
			return fmt.Errorf("decode %s: %w", header.Kind, err)
		}
		collectPodSpec(found, header.Kind, object.Name, object.Spec.Template.Spec)
		return nil
	case "ReplicaSet":
		var object appsv1.ReplicaSet
		if err := sigsyaml.Unmarshal(documentBytes, &object); err != nil {
			return fmt.Errorf("decode %s: %w", header.Kind, err)
		}
		collectPodSpec(found, header.Kind, object.Name, object.Spec.Template.Spec)
		return nil
	case "DaemonSet":
		var object appsv1.DaemonSet
		if err := sigsyaml.Unmarshal(documentBytes, &object); err != nil {
			return fmt.Errorf("decode %s: %w", header.Kind, err)
		}
		collectPodSpec(found, header.Kind, object.Name, object.Spec.Template.Spec)
		return nil
	case "StatefulSet":
		var object appsv1.StatefulSet
		if err := sigsyaml.Unmarshal(documentBytes, &object); err != nil {
			return fmt.Errorf("decode %s: %w", header.Kind, err)
		}
		collectPodSpec(found, header.Kind, object.Name, object.Spec.Template.Spec)
		return nil
	case "Job":
		var object batchv1.Job
		if err := sigsyaml.Unmarshal(documentBytes, &object); err != nil {
			return fmt.Errorf("decode %s: %w", header.Kind, err)
		}
		collectPodSpec(found, header.Kind, object.Name, object.Spec.Template.Spec)
		return nil
	case "CronJob":
		var object batchv1.CronJob
		if err := sigsyaml.Unmarshal(documentBytes, &object); err != nil {
			return fmt.Errorf("decode %s: %w", header.Kind, err)
		}
		collectPodSpec(found, header.Kind, object.Name, object.Spec.JobTemplate.Spec.Template.Spec)
		return nil
	case "List":
		var object listObject
		if err := sigsyaml.Unmarshal(documentBytes, &object); err != nil {
			return fmt.Errorf("decode List: %w", err)
		}
		for _, item := range object.Items {
			var child yamlv3.Node
			if err := yamlv3.Unmarshal(item, &child); err != nil {
				return fmt.Errorf("decode List item: %w", err)
			}
			if err := extractDocument(&child, found); err != nil {
				return err
			}
		}
	}

	return nil
}

func collectPodSpec(found map[string]*ImageRef, kind string, name string, spec corev1.PodSpec) {
	if name == "" {
		name = "<unknown>"
	}

	collectContainerImages(found, kind, name, "spec.containers", spec.Containers)
	collectContainerImages(found, kind, name, "spec.initContainers", spec.InitContainers)
	collectEphemeralContainerImages(found, kind, name, "spec.ephemeralContainers", spec.EphemeralContainers)
}

func collectContainerImages(found map[string]*ImageRef, kind string, name string, path string, containers []corev1.Container) {
	for idx, container := range containers {
		ref, ok := ParseImageRef(container.Image)
		if !ok {
			continue
		}
		recordRef(ref, fmt.Sprintf("%s/%s %s[%d]", kind, name, path, idx), found)
	}
}

func collectEphemeralContainerImages(found map[string]*ImageRef, kind string, name string, path string, containers []corev1.EphemeralContainer) {
	for idx, container := range containers {
		ref, ok := ParseImageRef(container.Image)
		if !ok {
			continue
		}
		recordRef(ref, fmt.Sprintf("%s/%s %s[%d]", kind, name, path, idx), found)
	}
}

func yamlv3Marshal(node *yamlv3.Node) ([]byte, error) {
	return yamlv3.Marshal(node)
}

func recordRef(ref ImageRef, source string, found map[string]*ImageRef) {
	existing, ok := found[ref.Reference]
	if !ok {
		ref.Sources = []string{source}
		found[ref.Reference] = &ref
		return
	}

	for _, current := range existing.Sources {
		if current == source {
			return
		}
	}

	existing.Sources = append(existing.Sources, source)
}
