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

func fakeTaintClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRManagedCluster: "ManagedClusterList",
			client.GVRPlacement:     "PlacementList",
		}, objs...)
	return &client.Client{Dynamic: fake}
}

func taintedCluster(name string, taints []interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": map[string]interface{}{},
		},
	}
	if len(taints) > 0 {
		spec := obj.Object["spec"].(map[string]interface{})
		spec["taints"] = taints
	}
	return obj
}

func TestAddTaint(t *testing.T) {
	mc := taintedCluster("spoke1", nil)
	c := fakeTaintClient(mc)
	insp := New(c, config.Config{}, discardLogger)

	err := insp.AddTaint(context.Background(), "spoke1", "gpu-reserved", "true", "NoSchedule")
	if err != nil {
		t.Fatalf("AddTaint failed: %v", err)
	}
}

func TestAddTaintDuplicate(t *testing.T) {
	mc := taintedCluster("spoke1", []interface{}{
		map[string]interface{}{"key": "gpu-reserved", "value": "true", "effect": "NoSchedule"},
	})
	c := fakeTaintClient(mc)
	insp := New(c, config.Config{}, discardLogger)

	err := insp.AddTaint(context.Background(), "spoke1", "gpu-reserved", "true", "NoSchedule")
	if err == nil {
		t.Fatal("expected error for duplicate taint")
	}
}

func TestRemoveTaint(t *testing.T) {
	mc := taintedCluster("spoke1", []interface{}{
		map[string]interface{}{"key": "gpu-reserved", "value": "true", "effect": "NoSchedule"},
	})
	c := fakeTaintClient(mc)
	insp := New(c, config.Config{}, discardLogger)

	err := insp.RemoveTaint(context.Background(), "spoke1", "gpu-reserved")
	if err != nil {
		t.Fatalf("RemoveTaint failed: %v", err)
	}
}

func TestRemoveTaintNotFound(t *testing.T) {
	mc := taintedCluster("spoke1", nil)
	c := fakeTaintClient(mc)
	insp := New(c, config.Config{}, discardLogger)

	err := insp.RemoveTaint(context.Background(), "spoke1", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent taint")
	}
}

func TestListTaints(t *testing.T) {
	mc := taintedCluster("spoke1", []interface{}{
		map[string]interface{}{"key": "gpu-reserved", "value": "true", "effect": "NoSchedule"},
		map[string]interface{}{"key": "maintenance", "value": "", "effect": "NoExecute"},
	})
	c := fakeTaintClient(mc)
	insp := New(c, config.Config{}, discardLogger)

	taints, err := insp.ListTaints(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("ListTaints failed: %v", err)
	}
	if len(taints) != 2 {
		t.Fatalf("expected 2 taints, got %d", len(taints))
	}
	if taints[0].Key != "gpu-reserved" {
		t.Errorf("expected key gpu-reserved, got %s", taints[0].Key)
	}
}

func TestListTaintsEmpty(t *testing.T) {
	mc := taintedCluster("spoke1", nil)
	c := fakeTaintClient(mc)
	insp := New(c, config.Config{}, discardLogger)

	taints, err := insp.ListTaints(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("ListTaints failed: %v", err)
	}
	if len(taints) != 0 {
		t.Fatalf("expected 0 taints, got %d", len(taints))
	}
}

func TestCreateTolerantPlacement(t *testing.T) {
	c := fakeTaintClient()
	insp := New(c, config.Config{}, discardLogger)

	tolerations := []Taint{
		{Key: "gpu-reserved", Value: "true", Effect: "NoSchedule"},
	}
	err := insp.CreateTolerantPlacement(context.Background(), "ml-workloads", "default", tolerations, []string{"gpu-clusters"})
	if err != nil {
		t.Fatalf("CreateTolerantPlacement failed: %v", err)
	}
}

func TestCreateTolerantPlacementDefaultNamespace(t *testing.T) {
	c := fakeTaintClient()
	insp := New(c, config.Config{}, discardLogger)

	err := insp.CreateTolerantPlacement(context.Background(), "test", "", []Taint{{Key: "k", Value: "v"}}, nil)
	if err != nil {
		t.Fatalf("CreateTolerantPlacement failed: %v", err)
	}
}

func TestBuildTolerantPlacement(t *testing.T) {
	tolerations := []Taint{
		{Key: "gpu-reserved", Value: "true", Effect: "NoSchedule"},
	}
	obj := buildTolerantPlacement("test", "ns", tolerations, []string{"set1"})
	if obj.GetName() != "test" {
		t.Errorf("expected name test, got %s", obj.GetName())
	}
	labels := obj.GetLabels()
	if labels["acmlab.redhat.com/tolerant-placement"] != "true" {
		t.Error("expected tolerant-placement label")
	}
}
