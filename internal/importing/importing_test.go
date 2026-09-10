package importing

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestExtractConditionsReturnsAvailableAndJoined(t *testing.T) {
	mc := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "ManagedClusterConditionAvailable",
						"status": "True",
					},
					map[string]interface{}{
						"type":   "ManagedClusterJoined",
						"status": "True",
					},
				},
			},
		},
	}

	available, joined := extractConditions(mc)
	if available != "True" {
		t.Errorf("expected available=True, got %s", available)
	}
	if joined != "True" {
		t.Errorf("expected joined=True, got %s", joined)
	}
}

func TestExtractConditionsReturnsEmptyWhenMissing(t *testing.T) {
	mc := &unstructured.Unstructured{
		Object: map[string]interface{}{},
	}

	available, joined := extractConditions(mc)
	if available != "" {
		t.Errorf("expected available='', got %s", available)
	}
	if joined != "" {
		t.Errorf("expected joined='', got %s", joined)
	}
}

func TestExtractConditionsUnknownState(t *testing.T) {
	mc := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "ManagedClusterConditionAvailable",
						"status": "Unknown",
					},
					map[string]interface{}{
						"type":   "ManagedClusterJoined",
						"status": "True",
					},
				},
			},
		},
	}

	available, joined := extractConditions(mc)
	if available != "Unknown" {
		t.Errorf("expected available=Unknown, got %s", available)
	}
	if joined != "True" {
		t.Errorf("expected joined=True, got %s", joined)
	}
}

func TestBuildManagedClusterLabels(t *testing.T) {
	opts := ImportOptions{
		Name: "test-cluster",
		Labels: map[string]string{
			"cloud":  "IBM",
			"vendor": "OpenShift",
		},
		ClusterSet: "production",
	}

	allLabels := map[string]interface{}{
		"name": opts.Name,
	}
	if opts.ClusterSet != "" {
		allLabels["cluster.open-cluster-management.io/clusterset"] = opts.ClusterSet
	}
	for k, v := range opts.Labels {
		allLabels[k] = v
	}

	if allLabels["name"] != "test-cluster" {
		t.Errorf("expected name=test-cluster, got %v", allLabels["name"])
	}
	if allLabels["cluster.open-cluster-management.io/clusterset"] != "production" {
		t.Errorf("expected clusterset=production, got %v", allLabels["cluster.open-cluster-management.io/clusterset"])
	}
	if allLabels["cloud"] != "IBM" {
		t.Errorf("expected cloud=IBM, got %v", allLabels["cloud"])
	}
}

func TestBuildManagedClusterLabelsDefaultClusterSet(t *testing.T) {
	opts := ImportOptions{
		Name:       "test-cluster",
		ClusterSet: "",
	}

	allLabels := map[string]interface{}{
		"name": opts.Name,
	}
	if opts.ClusterSet != "" {
		allLabels["cluster.open-cluster-management.io/clusterset"] = opts.ClusterSet
	} else {
		allLabels["cluster.open-cluster-management.io/clusterset"] = "default"
	}

	if allLabels["cluster.open-cluster-management.io/clusterset"] != "default" {
		t.Errorf("expected default clusterset, got %v", allLabels["cluster.open-cluster-management.io/clusterset"])
	}
}

func TestImportResultAutoImportMessage(t *testing.T) {
	result := &ImportResult{
		Name:       "test",
		AutoImport: true,
		Message:    "Cluster test registered for import (auto-import enabled)",
	}
	if !result.AutoImport {
		t.Error("expected AutoImport=true")
	}

	resultManual := &ImportResult{
		Name:       "test",
		AutoImport: false,
		Message:    "Cluster test registered for import — apply import manifests manually on the spoke",
	}
	if resultManual.AutoImport {
		t.Error("expected AutoImport=false")
	}
}

func TestImportStatusFields(t *testing.T) {
	status := ImportStatus{
		Name:       "external-cluster",
		Available:  "True",
		Joined:     "True",
		CreatedVia: "discovery",
		AutoImport: true,
		Labels: map[string]string{
			"cloud":  "Other",
			"vendor": "Other",
		},
	}

	if status.Name != "external-cluster" {
		t.Errorf("expected name=external-cluster, got %s", status.Name)
	}
	if status.CreatedVia != "discovery" {
		t.Errorf("expected createdVia=discovery, got %s", status.CreatedVia)
	}
	if status.Labels["cloud"] != "Other" {
		t.Errorf("expected cloud=Other, got %s", status.Labels["cloud"])
	}
}
