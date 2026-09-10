package registry

import (
	"testing"
)

func TestExtractImagesFromObject(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "klusterlet",
							"image": "registry.redhat.io/multicluster-engine/registration-operator-rhel9@sha256:abc123",
						},
						map[string]interface{}{
							"name":  "sidecar",
							"image": "registry.redhat.io/rhacm2/search-collector-rhel9@sha256:def456",
						},
					},
				},
			},
		},
	}

	images := extractImagesFromObject(obj)

	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d: %v", len(images), images)
	}
	if images[0] != "registry.redhat.io/multicluster-engine/registration-operator-rhel9@sha256:abc123" {
		t.Errorf("unexpected first image: %s", images[0])
	}
	if images[1] != "registry.redhat.io/rhacm2/search-collector-rhel9@sha256:def456" {
		t.Errorf("unexpected second image: %s", images[1])
	}
}

func TestExtractImagesSkipsNonImageStrings(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "klusterlet",
		},
		"apiVersion": "v1",
	}

	images := extractImagesFromObject(obj)
	if len(images) != 0 {
		t.Errorf("expected no images, got %v", images)
	}
}

func TestDefaultRegistryMappings(t *testing.T) {
	mappings := DefaultRegistryMappings("quay.io/myorg")

	if len(mappings) != 2 {
		t.Fatalf("expected 2 mappings, got %d", len(mappings))
	}

	if mappings[0].Source != "registry.redhat.io/multicluster-engine" {
		t.Errorf("unexpected source: %s", mappings[0].Source)
	}
	if mappings[0].Mirror != "quay.io/myorg/multicluster-engine" {
		t.Errorf("unexpected mirror: %s", mappings[0].Mirror)
	}
	if mappings[1].Source != "registry.redhat.io/rhacm2" {
		t.Errorf("unexpected source: %s", mappings[1].Source)
	}
	if mappings[1].Mirror != "quay.io/myorg/rhacm2" {
		t.Errorf("unexpected mirror: %s", mappings[1].Mirror)
	}
}

func TestGenerateMirrorScript(t *testing.T) {
	images := []RequiredImage{
		{Image: "registry.redhat.io/multicluster-engine/registration-operator-rhel9@sha256:abc", ManifestWork: "test-klusterlet"},
		{Image: "registry.redhat.io/rhacm2/search-collector-rhel9@sha256:def", ManifestWork: "addon-search"},
	}

	script := GenerateMirrorScript(images, "quay.io/myorg")

	if !contains(script, "skopeo copy") {
		t.Error("script should contain skopeo copy commands")
	}
	if !contains(script, "${TARGET_REGISTRY}/multicluster-engine/registration-operator-rhel9@sha256:abc") {
		t.Error("script should contain target image for MCE")
	}
	if !contains(script, "${TARGET_REGISTRY}/rhacm2/search-collector-rhel9@sha256:def") {
		t.Error("script should contain target image for RHACM")
	}
	if !contains(script, "#!/bin/bash") {
		t.Error("script should have bash shebang")
	}
}

func TestGenerateMirrorScriptSkipsInvalidImages(t *testing.T) {
	images := []RequiredImage{
		{Image: "no-slash-here", ManifestWork: "test"},
	}

	script := GenerateMirrorScript(images, "quay.io/myorg")

	if contains(script, "skopeo copy") {
		t.Error("script should not contain skopeo for images without registry path")
	}
}

func TestMirrorStatusDefaults(t *testing.T) {
	status := &MirrorStatus{ClusterName: "test", Configured: false}

	if status.Configured {
		t.Error("should not be configured by default")
	}
	if len(status.Registries) != 0 {
		t.Error("should have no registries by default")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
