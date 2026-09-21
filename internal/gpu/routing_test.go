package gpu

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

func TestCreateGPUPlacementCreatesWithPredicates(t *testing.T) {
	mgr := newTestManager()
	err := mgr.CreateGPUPlacement(context.Background(), "gpu-h100", "H100", "")
	if err != nil {
		t.Fatalf("CreateGPUPlacement failed: %v", err)
	}

	placement, err := mgr.client.Get(context.Background(), client.GVRPlacement, DefaultNamespace, "gpu-h100")
	if err != nil {
		t.Fatalf("Placement not found: %v", err)
	}

	predicates, found, _ := unstructured.NestedSlice(placement.Object, "spec", "predicates")
	if !found || len(predicates) == 0 {
		t.Fatal("expected predicates in placement spec")
	}

	pred := predicates[0].(map[string]interface{})
	selector := pred["requiredClusterSelector"].(map[string]interface{})
	labelSel := selector["labelSelector"].(map[string]interface{})
	exprs := labelSel["matchExpressions"].([]interface{})

	if len(exprs) != 2 {
		t.Errorf("expected 2 matchExpressions (gpu-type, gpu-available), got %d", len(exprs))
	}
}

func TestCreateGPUPlacementWithRegion(t *testing.T) {
	mgr := newTestManager()
	err := mgr.CreateGPUPlacement(context.Background(), "gpu-h100-eu", "H100", "eu-gb")
	if err != nil {
		t.Fatalf("CreateGPUPlacement failed: %v", err)
	}

	placement, err := mgr.client.Get(context.Background(), client.GVRPlacement, DefaultNamespace, "gpu-h100-eu")
	if err != nil {
		t.Fatalf("Placement not found: %v", err)
	}

	predicates, _, _ := unstructured.NestedSlice(placement.Object, "spec", "predicates")
	pred := predicates[0].(map[string]interface{})
	selector := pred["requiredClusterSelector"].(map[string]interface{})
	labelSel := selector["labelSelector"].(map[string]interface{})
	exprs := labelSel["matchExpressions"].([]interface{})

	if len(exprs) != 3 {
		t.Errorf("expected 3 matchExpressions (gpu-type, gpu-available, region), got %d", len(exprs))
	}

	regionExpr := exprs[2].(map[string]interface{})
	if regionExpr["key"] != "region" {
		t.Errorf("third expression key = %v, want region", regionExpr["key"])
	}
	vals := regionExpr["values"].([]interface{})
	if len(vals) != 1 || vals[0] != "eu-gb" {
		t.Errorf("region values = %v, want [eu-gb]", vals)
	}
}

func TestCreateGPUPlacementWithoutRegion(t *testing.T) {
	mgr := newTestManager()
	err := mgr.CreateGPUPlacement(context.Background(), "gpu-a100", "A100", "")
	if err != nil {
		t.Fatalf("CreateGPUPlacement failed: %v", err)
	}

	placement, err := mgr.client.Get(context.Background(), client.GVRPlacement, DefaultNamespace, "gpu-a100")
	if err != nil {
		t.Fatalf("Placement not found: %v", err)
	}

	predicates, _, _ := unstructured.NestedSlice(placement.Object, "spec", "predicates")
	pred := predicates[0].(map[string]interface{})
	selector := pred["requiredClusterSelector"].(map[string]interface{})
	labelSel := selector["labelSelector"].(map[string]interface{})
	exprs := labelSel["matchExpressions"].([]interface{})

	if len(exprs) != 2 {
		t.Errorf("expected 2 matchExpressions without region, got %d", len(exprs))
	}
}

func placementDecisionObj(placementName string, clusters ...string) *unstructured.Unstructured {
	decisions := []interface{}{}
	for _, c := range clusters {
		decisions = append(decisions, map[string]interface{}{
			"clusterName": c,
			"reason":      "matched",
		})
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta1",
			"kind":       "PlacementDecision",
			"metadata": map[string]interface{}{
				"name":      placementName + "-decision-1",
				"namespace": DefaultNamespace,
				"labels": map[string]interface{}{
					"cluster.open-cluster-management.io/placement": placementName,
				},
			},
			"status": map[string]interface{}{
				"decisions": decisions,
			},
		},
	}
}

func TestGetPlacementDecisionReturnsClusters(t *testing.T) {
	pd := placementDecisionObj("gpu-h100", "gpu-cluster-1", "gpu-cluster-2")
	mgr := newTestManager(pd)

	results, err := mgr.GetPlacementDecision(context.Background(), "gpu-h100")
	if err != nil {
		t.Fatalf("GetPlacementDecision failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ClusterName != "gpu-cluster-1" {
		t.Errorf("first cluster = %s, want gpu-cluster-1", results[0].ClusterName)
	}
	if results[1].ClusterName != "gpu-cluster-2" {
		t.Errorf("second cluster = %s, want gpu-cluster-2", results[1].ClusterName)
	}
}

func TestGetPlacementDecisionEmptyWhenNone(t *testing.T) {
	mgr := newTestManager()

	results, err := mgr.GetPlacementDecision(context.Background(), "gpu-nonexistent")
	if err != nil {
		t.Fatalf("GetPlacementDecision failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestMarkSaturatedPatchesLabel(t *testing.T) {
	cluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "gpu-h100-1",
				"labels": map[string]interface{}{
					"gpu-type":      "H100",
					"gpu-available": "true",
				},
			},
		},
	}
	mgr := newTestManager(cluster)

	err := mgr.MarkSaturated(context.Background(), "gpu-h100-1", true)
	if err != nil {
		t.Fatalf("MarkSaturated failed: %v", err)
	}

	updated, err := mgr.client.Get(context.Background(), client.GVRManagedCluster, "", "gpu-h100-1")
	if err != nil {
		t.Fatalf("Get cluster failed: %v", err)
	}
	labels, _, _ := unstructured.NestedStringMap(updated.Object, "metadata", "labels")
	if labels["gpu-available"] != "false" {
		t.Errorf("gpu-available = %s, want false", labels["gpu-available"])
	}
}

func TestMarkSaturatedClearsLabel(t *testing.T) {
	cluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "gpu-h100-1",
				"labels": map[string]interface{}{
					"gpu-type":      "H100",
					"gpu-available": "false",
				},
			},
		},
	}
	mgr := newTestManager(cluster)

	err := mgr.MarkSaturated(context.Background(), "gpu-h100-1", false)
	if err != nil {
		t.Fatalf("MarkSaturated failed: %v", err)
	}

	updated, err := mgr.client.Get(context.Background(), client.GVRManagedCluster, "", "gpu-h100-1")
	if err != nil {
		t.Fatalf("Get cluster failed: %v", err)
	}
	labels, _, _ := unstructured.NestedStringMap(updated.Object, "metadata", "labels")
	if labels["gpu-available"] != "true" {
		t.Errorf("gpu-available = %s, want true", labels["gpu-available"])
	}
}

func TestBestClusterUsesUniqueName(t *testing.T) {
	mgr := newTestManager()
	ctx := context.Background()

	go func() {
		time.Sleep(10 * time.Millisecond)
		placements, _ := mgr.client.List(ctx, client.GVRPlacement, DefaultNamespace, "")
		for _, p := range placements.Items {
			name := p.GetName()
			pd := placementDecisionObj(name, "gpu-cluster-best")
			_ = mgr.client.CreateIfNotExists(ctx, client.GVRPlacementDecision, DefaultNamespace, pd)
		}
	}()

	best, err := mgr.BestClusterWithTimeout(ctx, "H100", 2*time.Second)
	if err != nil {
		t.Fatalf("BestCluster failed: %v", err)
	}
	if best != "gpu-cluster-best" {
		t.Errorf("best = %s, want gpu-cluster-best", best)
	}
}

func TestBestClusterConcurrentCallsUseDifferentNames(t *testing.T) {
	mgr := newTestManager()
	ctx := context.Background()

	go func() {
		for i := 0; i < 20; i++ {
			time.Sleep(5 * time.Millisecond)
			placements, _ := mgr.client.List(ctx, client.GVRPlacement, DefaultNamespace, "")
			for _, p := range placements.Items {
				name := p.GetName()
				pd := placementDecisionObj(name, "gpu-cluster-1")
				_ = mgr.client.CreateIfNotExists(ctx, client.GVRPlacementDecision, DefaultNamespace, pd)
			}
		}
	}()

	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := mgr.BestClusterWithTimeout(ctx, "H100", 2*time.Second)
			errs <- err
		}()
	}

	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent BestCluster call %d failed: %v", i, err)
		}
	}
}

func TestBestClusterTimeoutReturnsPending(t *testing.T) {
	mgr := newTestManager()

	_, err := mgr.BestClusterWithTimeout(context.Background(), "H100", 100*time.Millisecond)
	if !errors.Is(err, ErrGPUPending) {
		t.Fatalf("expected ErrGPUPending, got %v", err)
	}
}

func TestBestClusterEmptyDecisionReturnsNoMatch(t *testing.T) {
	mgr := newTestManager()
	ctx := context.Background()

	go func() {
		time.Sleep(10 * time.Millisecond)
		placements, _ := mgr.client.List(ctx, client.GVRPlacement, DefaultNamespace, "")
		for _, p := range placements.Items {
			name := p.GetName()
			emptyPD := placementDecisionObj(name)
			_ = mgr.client.CreateIfNotExists(ctx, client.GVRPlacementDecision, DefaultNamespace, emptyPD)
		}
	}()

	_, err := mgr.BestClusterWithTimeout(ctx, "A100", 2*time.Second)
	if !errors.Is(err, ErrGPUNoMatch) {
		t.Fatalf("expected ErrGPUNoMatch, got %v", err)
	}
}

func TestParsePlacementDecisions(t *testing.T) {
	tests := []struct {
		name     string
		items    []unstructured.Unstructured
		wantLen  int
		wantName string
	}{
		{
			name:    "empty list",
			items:   nil,
			wantLen: 0,
		},
		{
			name: "single decision with one cluster",
			items: []unstructured.Unstructured{
				{Object: map[string]interface{}{
					"status": map[string]interface{}{
						"decisions": []interface{}{
							map[string]interface{}{"clusterName": "c1"},
						},
					},
				}},
			},
			wantLen:  1,
			wantName: "c1",
		},
		{
			name: "decision without status",
			items: []unstructured.Unstructured{
				{Object: map[string]interface{}{
					"metadata": map[string]interface{}{"name": "pd1"},
				}},
			},
			wantLen: 0,
		},
		{
			name: "multiple decisions across items",
			items: []unstructured.Unstructured{
				{Object: map[string]interface{}{
					"status": map[string]interface{}{
						"decisions": []interface{}{
							map[string]interface{}{"clusterName": "c1"},
						},
					},
				}},
				{Object: map[string]interface{}{
					"status": map[string]interface{}{
						"decisions": []interface{}{
							map[string]interface{}{"clusterName": "c2"},
							map[string]interface{}{"clusterName": "c3"},
						},
					},
				}},
			},
			wantLen:  3,
			wantName: "c1",
		},
		{
			name: "decision with empty clusterName skipped",
			items: []unstructured.Unstructured{
				{Object: map[string]interface{}{
					"status": map[string]interface{}{
						"decisions": []interface{}{
							map[string]interface{}{"clusterName": ""},
							map[string]interface{}{"clusterName": "valid"},
						},
					},
				}},
			},
			wantLen:  1,
			wantName: "valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := &unstructured.UnstructuredList{Items: tt.items}
			results := parsePlacementDecisions(list)
			if len(results) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(results), tt.wantLen)
			}
			if tt.wantName != "" && len(results) > 0 && results[0].ClusterName != tt.wantName {
				t.Errorf("first cluster = %s, want %s", results[0].ClusterName, tt.wantName)
			}
		})
	}
}
