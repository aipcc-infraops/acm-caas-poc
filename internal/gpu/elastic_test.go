package gpu

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)



func TestDetectSaturationBelowThreshold(t *testing.T) {
	mc := managedCluster("gpu-h100-01", map[string]string{
		"gpu-type":        "H100",
		"gpu-utilization": "45.5",
	})
	m := newTestManager(mc)

	status, err := m.DetectSaturation(context.Background(), "gpu-h100-01", 85.0)
	if err != nil {
		t.Fatalf("DetectSaturation: %v", err)
	}
	if status.Saturated {
		t.Error("expected not saturated at 45.5% with threshold 85%")
	}
	if status.Utilization != 45.5 {
		t.Errorf("utilization = %v, want 45.5", status.Utilization)
	}
	if status.GPUType != "H100" {
		t.Errorf("gpuType = %q, want H100", status.GPUType)
	}
}

func TestDetectSaturationAboveThreshold(t *testing.T) {
	mc := managedCluster("gpu-h100-02", map[string]string{
		"gpu-type":        "H100",
		"gpu-utilization": "92.3",
	})
	m := newTestManager(mc)

	status, err := m.DetectSaturation(context.Background(), "gpu-h100-02", 85.0)
	if err != nil {
		t.Fatalf("DetectSaturation: %v", err)
	}
	if !status.Saturated {
		t.Error("expected saturated at 92.3% with threshold 85%")
	}
	if status.Threshold != 85.0 {
		t.Errorf("threshold = %v, want 85.0", status.Threshold)
	}
}

func TestDetectSaturationNoUtilizationLabel(t *testing.T) {
	mc := managedCluster("gpu-l4-01", map[string]string{
		"gpu-type": "L4",
	})
	m := newTestManager(mc)

	status, err := m.DetectSaturation(context.Background(), "gpu-l4-01", 85.0)
	if err != nil {
		t.Fatalf("DetectSaturation: %v", err)
	}
	if status.Utilization != 0 {
		t.Errorf("utilization = %v, want 0 when label missing", status.Utilization)
	}
	if status.Saturated {
		t.Error("expected not saturated when utilization is 0")
	}
}

func TestProvisionOnDemandLabelsCluster(t *testing.T) {
	mc := managedCluster("gpu-burst-01", map[string]string{})
	m := newTestManager(mc)

	err := m.ProvisionOnDemand(context.Background(), OnDemandOpts{
		Cluster: "gpu-burst-01",
		GPUType: "L4",
	})
	if err != nil {
		t.Fatalf("ProvisionOnDemand: %v", err)
	}

	updated, err := m.client.Get(context.Background(), client.GVRManagedCluster, "", "gpu-burst-01")
	if err != nil {
		t.Fatalf("Get updated cluster: %v", err)
	}
	labels := updated.GetLabels()
	if labels["gpu-elastic"] != "true" {
		t.Errorf("gpu-elastic = %q, want true", labels["gpu-elastic"])
	}
	if labels["gpu-type"] != "L4" {
		t.Errorf("gpu-type = %q, want L4", labels["gpu-type"])
	}
	if labels["gpu-cost-tier"] != "on-demand" {
		t.Errorf("gpu-cost-tier = %q, want on-demand", labels["gpu-cost-tier"])
	}
	if labels["gpu-available"] != "true" {
		t.Errorf("gpu-available = %q, want true", labels["gpu-available"])
	}
}

func TestProvisionOnDemandCreatesManifestWork(t *testing.T) {
	mc := managedCluster("gpu-burst-02", map[string]string{})
	ns := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Namespace",
			"metadata": map[string]interface{}{"name": "gpu-burst-02"},
		},
	}
	m := newTestManager(mc, ns)

	err := m.ProvisionOnDemand(context.Background(), OnDemandOpts{
		Cluster: "gpu-burst-02",
		GPUType: "H100",
	})
	if err != nil {
		t.Fatalf("ProvisionOnDemand: %v", err)
	}

	mw, err := m.client.Get(context.Background(), client.GVRManifestWork, "gpu-burst-02", "gpu-elastic-stack-gpu-burst-02")
	if err != nil {
		t.Fatalf("Get ManifestWork: %v", err)
	}
	labels := mw.GetLabels()
	if labels["gpu-type"] != "H100" {
		t.Errorf("ManifestWork gpu-type = %q, want H100", labels["gpu-type"])
	}
}

func TestHibernateIdlePatchesLabels(t *testing.T) {
	mc := managedCluster("gpu-ondemand-01", map[string]string{
		"gpu-elastic":   "true",
		"gpu-available": "true",
	})
	m := newTestManager(mc)

	err := m.HibernateIdle(context.Background(), "gpu-ondemand-01")
	if err != nil {
		t.Fatalf("HibernateIdle: %v", err)
	}

	updated, err := m.client.Get(context.Background(), client.GVRManagedCluster, "", "gpu-ondemand-01")
	if err != nil {
		t.Fatalf("Get updated cluster: %v", err)
	}
	labels := updated.GetLabels()
	if labels["gpu-available"] != "false" {
		t.Errorf("gpu-available = %q, want false", labels["gpu-available"])
	}
	if labels["gpu-elastic-state"] != "hibernated" {
		t.Errorf("gpu-elastic-state = %q, want hibernated", labels["gpu-elastic-state"])
	}
}

func TestListElasticClustersReturnsTagged(t *testing.T) {
	mc1 := managedCluster("gpu-elastic-01", map[string]string{
		"gpu-elastic":   "true",
		"gpu-type":      "H100",
		"gpu-cost-tier": "on-demand",
		"gpu-available": "true",
	})
	mc2 := managedCluster("gpu-elastic-02", map[string]string{
		"gpu-elastic":       "true",
		"gpu-type":          "L4",
		"gpu-cost-tier":     "spot",
		"gpu-available":     "false",
		"gpu-elastic-state": "hibernated",
	})
	m := newTestManager(mc1, mc2)

	clusters, err := m.ListElasticClusters(context.Background())
	if err != nil {
		t.Fatalf("ListElasticClusters: %v", err)
	}
	if len(clusters) != 2 {
		t.Fatalf("got %d clusters, want 2", len(clusters))
	}

	found := map[string]ElasticCluster{}
	for _, c := range clusters {
		found[c.Name] = c
	}

	c1 := found["gpu-elastic-01"]
	if c1.GPUType != "H100" || c1.CostTier != "on-demand" || !c1.Available {
		t.Errorf("gpu-elastic-01: got %+v", c1)
	}

	c2 := found["gpu-elastic-02"]
	if c2.GPUType != "L4" || c2.CostTier != "spot" || c2.Available {
		t.Errorf("gpu-elastic-02: got %+v", c2)
	}
}

func TestListElasticClustersEmptyWhenNone(t *testing.T) {
	mc := managedCluster("regular-spoke", map[string]string{"env": "dev"})
	m := newTestManager(mc)

	clusters, err := m.ListElasticClusters(context.Background())
	if err != nil {
		t.Fatalf("ListElasticClusters: %v", err)
	}
	if len(clusters) != 0 {
		t.Errorf("got %d clusters, want 0", len(clusters))
	}
}

func TestParseElasticCluster(t *testing.T) {
	tests := []struct {
		name   string
		obj    map[string]interface{}
		want   ElasticCluster
	}{
		{
			name: "full labels",
			obj: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "gpu-e1",
					"labels": map[string]interface{}{
						"gpu-type":      "H100",
						"gpu-cost-tier": "on-demand",
						"gpu-available": "true",
						"gpu-elastic":   "true",
					},
				},
			},
			want: ElasticCluster{Name: "gpu-e1", GPUType: "H100", CostTier: "on-demand", Available: true},
		},
		{
			name: "unavailable cluster",
			obj: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "gpu-e2",
					"labels": map[string]interface{}{
						"gpu-type":      "L4",
						"gpu-cost-tier": "spot",
						"gpu-available": "false",
					},
				},
			},
			want: ElasticCluster{Name: "gpu-e2", GPUType: "L4", CostTier: "spot", Available: false},
		},
		{
			name: "no labels",
			obj: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "gpu-e3",
				},
			},
			want: ElasticCluster{Name: "gpu-e3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseElasticCluster(tt.obj)
			if got != tt.want {
				t.Errorf("parseElasticCluster = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestBuildElasticLabels(t *testing.T) {
	patch := buildElasticLabels("H100", "on-demand")
	metadata := patch["metadata"].(map[string]interface{})
	labels := metadata["labels"].(map[string]interface{})

	if labels["gpu-type"] != "H100" {
		t.Errorf("gpu-type = %v, want H100", labels["gpu-type"])
	}
	if labels["gpu-cost-tier"] != "on-demand" {
		t.Errorf("gpu-cost-tier = %v, want on-demand", labels["gpu-cost-tier"])
	}
	if labels["gpu-elastic"] != "true" {
		t.Errorf("gpu-elastic = %v, want true", labels["gpu-elastic"])
	}
	if labels["gpu-available"] != "true" {
		t.Errorf("gpu-available = %v, want true", labels["gpu-available"])
	}
}

func TestBuildHibernateLabels(t *testing.T) {
	patch := buildHibernateLabels()
	metadata := patch["metadata"].(map[string]interface{})
	labels := metadata["labels"].(map[string]interface{})

	if labels["gpu-available"] != "false" {
		t.Errorf("gpu-available = %v, want false", labels["gpu-available"])
	}
	if labels["gpu-elastic-state"] != "hibernated" {
		t.Errorf("gpu-elastic-state = %v, want hibernated", labels["gpu-elastic-state"])
	}
}
