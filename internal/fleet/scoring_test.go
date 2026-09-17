package fleet

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

func fakeScoringClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRPlacement:         "PlacementList",
			client.GVRPlacementDecision: "PlacementDecisionList",
		}, objs...)
	return &client.Client{Dynamic: fake}
}

func scoringPlacement(name, namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1beta1", Kind: "Placement",
	})
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetLabels(map[string]string{"acmlab.redhat.com/scoring": "true"})
	return obj
}

func TestConfigureScoringCreatesPlacement(t *testing.T) {
	c := fakeScoringClient()
	insp := New(c, config.Config{}, discardLogger)

	err := insp.ConfigureScoring(context.Background(), ScoringOpts{
		Name: "gpu-scoring",
	})
	if err != nil {
		t.Fatalf("ConfigureScoring failed: %v", err)
	}
}

func TestConfigureScoringWithCustomPrioritizers(t *testing.T) {
	c := fakeScoringClient()
	insp := New(c, config.Config{}, discardLogger)

	err := insp.ConfigureScoring(context.Background(), ScoringOpts{
		Name:         "custom-scoring",
		Prioritizers: []string{"ResourceAllocatableCPU"},
		ClusterSet:   "gpu-clusters",
	})
	if err != nil {
		t.Fatalf("ConfigureScoring failed: %v", err)
	}
}

func TestGetScoringReturnsStatus(t *testing.T) {
	p := scoringPlacement("test-scoring", "open-cluster-management")
	c := fakeScoringClient(p)
	insp := New(c, config.Config{}, discardLogger)

	status, err := insp.GetScoring(context.Background(), "test-scoring", "")
	if err != nil {
		t.Fatalf("GetScoring failed: %v", err)
	}
	if status.Name != "test-scoring" {
		t.Errorf("Name = %q, want %q", status.Name, "test-scoring")
	}
}

func TestGetScoringReturnsErrorForMissing(t *testing.T) {
	c := fakeScoringClient()
	insp := New(c, config.Config{}, discardLogger)

	_, err := insp.GetScoring(context.Background(), "nonexistent", "")
	if err == nil {
		t.Error("expected error for missing scoring")
	}
}

func TestRemoveScoringDeletesPlacement(t *testing.T) {
	p := scoringPlacement("to-remove", "open-cluster-management")
	c := fakeScoringClient(p)
	insp := New(c, config.Config{}, discardLogger)

	err := insp.RemoveScoring(context.Background(), "to-remove", "")
	if err != nil {
		t.Fatalf("RemoveScoring failed: %v", err)
	}
}

func TestRemoveScoringReturnsErrorForMissing(t *testing.T) {
	c := fakeScoringClient()
	insp := New(c, config.Config{}, discardLogger)

	err := insp.RemoveScoring(context.Background(), "nonexistent", "")
	if err == nil {
		t.Error("expected error for missing scoring")
	}
}

func TestListScoringReturnsPlacementsWithLabel(t *testing.T) {
	p1 := scoringPlacement("scoring-1", "open-cluster-management")
	p2 := scoringPlacement("scoring-2", "open-cluster-management")
	c := fakeScoringClient(p1, p2)
	insp := New(c, config.Config{}, discardLogger)

	infos, err := insp.ListScoring(context.Background(), "")
	if err != nil {
		t.Fatalf("ListScoring failed: %v", err)
	}
	if len(infos) != 2 {
		t.Errorf("got %d scoring placements, want 2", len(infos))
	}
}

func TestBuildScoringPlacement(t *testing.T) {
	obj := buildScoringPlacement("test", "ns", []string{"ResourceAllocatableCPU"}, "gpu-set", nil)
	if obj.GetName() != "test" {
		t.Errorf("Name = %q, want %q", obj.GetName(), "test")
	}
	labels := obj.GetLabels()
	if labels["acmlab.redhat.com/scoring"] != "true" {
		t.Error("missing scoring label")
	}
}

func TestBuildScoringPlacementWithLabels(t *testing.T) {
	obj := buildScoringPlacement("test", "ns", []string{"ResourceAllocatableCPU"}, "", map[string]string{"gpu": "true"})
	spec, _ := obj.Object["spec"].(map[string]interface{})
	predicates, _ := spec["predicates"].([]interface{})
	if len(predicates) != 1 {
		t.Errorf("got %d predicates, want 1", len(predicates))
	}
}

func placementDecision(name, namespace, placementName string, clusters []string) *unstructured.Unstructured {
	decisions := make([]interface{}, len(clusters))
	for i, c := range clusters {
		decisions[i] = map[string]interface{}{
			"clusterName": c,
			"reason":      "score: 100",
		}
	}
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1beta1", Kind: "PlacementDecision",
	})
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetLabels(map[string]string{
		"cluster.open-cluster-management.io/placement": placementName,
	})
	obj.Object["status"] = map[string]interface{}{
		"decisions": decisions,
	}
	return obj
}

func TestGetScoringWithDecisions(t *testing.T) {
	p := scoringPlacement("test-scoring", "open-cluster-management")
	pd := placementDecision("test-scoring-decision-1", "open-cluster-management", "test-scoring", []string{"spoke1", "spoke2"})
	c := fakeScoringClient(p, pd)
	insp := New(c, config.Config{}, discardLogger)

	status, err := insp.GetScoring(context.Background(), "test-scoring", "")
	if err != nil {
		t.Fatalf("GetScoring failed: %v", err)
	}
	if len(status.Decisions) != 2 {
		t.Errorf("got %d decisions, want 2", len(status.Decisions))
	}
	if status.Decisions[0].Cluster != "spoke1" {
		t.Errorf("first decision cluster = %q, want %q", status.Decisions[0].Cluster, "spoke1")
	}
}

func TestGetScoringWithNoDecisions(t *testing.T) {
	p := scoringPlacement("test-scoring", "open-cluster-management")
	c := fakeScoringClient(p)
	insp := New(c, config.Config{}, discardLogger)

	status, err := insp.GetScoring(context.Background(), "test-scoring", "")
	if err != nil {
		t.Fatalf("GetScoring failed: %v", err)
	}
	if len(status.Decisions) != 0 {
		t.Errorf("got %d decisions, want 0", len(status.Decisions))
	}
}

func TestGetScoringWithCustomNamespace(t *testing.T) {
	p := scoringPlacement("test-scoring", "custom-ns")
	c := fakeScoringClient(p)
	insp := New(c, config.Config{}, discardLogger)

	status, err := insp.GetScoring(context.Background(), "test-scoring", "custom-ns")
	if err != nil {
		t.Fatalf("GetScoring failed: %v", err)
	}
	if status.Namespace != "custom-ns" {
		t.Errorf("Namespace = %q, want %q", status.Namespace, "custom-ns")
	}
}

func TestConfigureScoringWithLabels(t *testing.T) {
	c := fakeScoringClient()
	insp := New(c, config.Config{}, discardLogger)

	err := insp.ConfigureScoring(context.Background(), ScoringOpts{
		Name:   "labeled-scoring",
		Labels: map[string]string{"gpu": "true"},
	})
	if err != nil {
		t.Fatalf("ConfigureScoring with labels failed: %v", err)
	}
}

func TestListScoringEmpty(t *testing.T) {
	c := fakeScoringClient()
	insp := New(c, config.Config{}, discardLogger)

	infos, err := insp.ListScoring(context.Background(), "")
	if err != nil {
		t.Fatalf("ListScoring failed: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("got %d, want 0", len(infos))
	}
}
