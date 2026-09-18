package rollout

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

func fakeOrderingClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRManifestWork: "ManifestWorkList",
		}, objs...)
	return &client.Client{Dynamic: fake}
}

func orderedMW(name, cluster string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/ordered-work": "true",
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{
						map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "Namespace",
							"metadata":   map[string]interface{}{"name": "app-ns"},
						},
						map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "ConfigMap",
							"metadata":   map[string]interface{}{"name": "app-config", "namespace": "app-ns"},
						},
					},
				},
			},
		},
	}
}

func TestCreateOrderedWork(t *testing.T) {
	c := fakeOrderingClient()
	m := New(c, config.Config{}, discardLogger)

	manifests := []OrderedManifest{
		{Ordinal: 0, Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Namespace",
			"metadata": map[string]interface{}{"name": "app-ns"},
		}},
		{Ordinal: 1, Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "ConfigMap",
			"metadata": map[string]interface{}{"name": "app-config", "namespace": "app-ns"},
		}},
	}
	if err := m.CreateOrderedWork(context.Background(), "spoke1", "app-stack", manifests); err != nil {
		t.Fatalf("CreateOrderedWork failed: %v", err)
	}
}

func TestCreateOrderedWorkSorts(t *testing.T) {
	c := fakeOrderingClient()
	m := New(c, config.Config{}, discardLogger)

	manifests := []OrderedManifest{
		{Ordinal: 2, Object: map[string]interface{}{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]interface{}{"name": "app", "namespace": "app-ns"},
		}},
		{Ordinal: 0, Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Namespace",
			"metadata": map[string]interface{}{"name": "app-ns"},
		}},
	}
	if err := m.CreateOrderedWork(context.Background(), "spoke1", "sorted-stack", manifests); err != nil {
		t.Fatalf("CreateOrderedWork failed: %v", err)
	}
}

func TestGetOrderedWork(t *testing.T) {
	mw := orderedMW("app-stack", "spoke1")
	c := fakeOrderingClient(mw)
	m := New(c, config.Config{}, discardLogger)

	info, err := m.GetOrderedWork(context.Background(), "spoke1", "app-stack")
	if err != nil {
		t.Fatalf("GetOrderedWork failed: %v", err)
	}
	if info.Manifests != 2 {
		t.Errorf("expected 2 manifests, got %d", info.Manifests)
	}
}

func TestGetOrderedWorkNotFound(t *testing.T) {
	c := fakeOrderingClient()
	m := New(c, config.Config{}, discardLogger)

	_, err := m.GetOrderedWork(context.Background(), "spoke1", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent work")
	}
}

func TestListOrderedWork(t *testing.T) {
	mw1 := orderedMW("stack-a", "spoke1")
	mw2 := orderedMW("stack-b", "spoke1")
	c := fakeOrderingClient(mw1, mw2)
	m := New(c, config.Config{}, discardLogger)

	list, err := m.ListOrderedWork(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("ListOrderedWork failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 items, got %d", len(list))
	}
}

func TestRemoveOrderedWork(t *testing.T) {
	mw := orderedMW("app-stack", "spoke1")
	c := fakeOrderingClient(mw)
	m := New(c, config.Config{}, discardLogger)

	if err := m.RemoveOrderedWork(context.Background(), "spoke1", "app-stack"); err != nil {
		t.Fatalf("RemoveOrderedWork failed: %v", err)
	}
}

func TestBuildOrderedManifestWork(t *testing.T) {
	manifests := []OrderedManifest{
		{Ordinal: 0, Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Namespace",
			"metadata": map[string]interface{}{"name": "ns"},
		}},
	}
	obj := buildOrderedManifestWork("spoke1", "test", manifests)
	if obj.GetName() != "test" {
		t.Errorf("expected name test, got %s", obj.GetName())
	}
	labels := obj.GetLabels()
	if labels["acmlab.redhat.com/ordered-work"] != "true" {
		t.Error("expected ordered-work label")
	}
}
