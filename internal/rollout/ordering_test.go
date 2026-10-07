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

func TestBuildOrderedManifestWorkNoOrdinalInResourceIdentifier(t *testing.T) {
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
	obj := buildOrderedManifestWork("spoke1", "test-ordinal", manifests)

	spec, _ := obj.Object["spec"].(map[string]interface{})
	configs, _ := spec["manifestConfigs"].([]interface{})
	for i, cfg := range configs {
		cm, ok := cfg.(map[string]interface{})
		if !ok {
			t.Fatalf("manifestConfigs[%d] is not a map", i)
		}
		ri, ok := cm["resourceIdentifier"].(map[string]interface{})
		if !ok {
			t.Fatalf("manifestConfigs[%d] has no resourceIdentifier", i)
		}
		if _, exists := ri["ordinal"]; exists {
			t.Errorf("manifestConfigs[%d].resourceIdentifier contains 'ordinal' field — this causes API warnings", i)
		}
		if _, exists := ri["name"]; !exists {
			t.Errorf("manifestConfigs[%d].resourceIdentifier missing 'name'", i)
		}
		if _, exists := ri["resource"]; !exists {
			t.Errorf("manifestConfigs[%d].resourceIdentifier missing 'resource'", i)
		}
	}
}

func TestBuildOrderedManifestWorkResourceIdentifierFields(t *testing.T) {
	tests := []struct {
		name       string
		manifest   OrderedManifest
		wantGroup  string
		wantRes    string
		wantName   string
		wantNS     string
	}{
		{
			name: "core namespace",
			manifest: OrderedManifest{Ordinal: 0, Object: map[string]interface{}{
				"apiVersion": "v1", "kind": "Namespace",
				"metadata": map[string]interface{}{"name": "app-ns"},
			}},
			wantGroup: "", wantRes: "namespaces", wantName: "app-ns", wantNS: "",
		},
		{
			name: "apps deployment",
			manifest: OrderedManifest{Ordinal: 1, Object: map[string]interface{}{
				"apiVersion": "apps/v1", "kind": "Deployment",
				"metadata": map[string]interface{}{"name": "web", "namespace": "app-ns"},
			}},
			wantGroup: "apps", wantRes: "deployments", wantName: "web", wantNS: "app-ns",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := buildOrderedManifestWork("spoke1", "ri-test", []OrderedManifest{tt.manifest})
			spec, _ := obj.Object["spec"].(map[string]interface{})
			configs, _ := spec["manifestConfigs"].([]interface{})
			cfg := configs[0].(map[string]interface{})
			ri := cfg["resourceIdentifier"].(map[string]interface{})

			if ri["group"] != tt.wantGroup {
				t.Errorf("group=%v, want %v", ri["group"], tt.wantGroup)
			}
			if ri["resource"] != tt.wantRes {
				t.Errorf("resource=%v, want %v", ri["resource"], tt.wantRes)
			}
			if ri["name"] != tt.wantName {
				t.Errorf("name=%v, want %v", ri["name"], tt.wantName)
			}
			if ri["namespace"] != tt.wantNS {
				t.Errorf("namespace=%v, want %v", ri["namespace"], tt.wantNS)
			}
		})
	}
}

func TestExtractGVK(t *testing.T) {
	tests := []struct {
		name      string
		obj       map[string]interface{}
		wantGroup string
		wantRes   string
	}{
		{
			name:      "core v1 namespace",
			obj:       map[string]interface{}{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]interface{}{"name": "ns"}},
			wantGroup: "", wantRes: "namespaces",
		},
		{
			name:      "apps deployment",
			obj:       map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": "d", "namespace": "ns"}},
			wantGroup: "apps", wantRes: "deployments",
		},
		{
			name:      "unknown kind",
			obj:       map[string]interface{}{"apiVersion": "custom.io/v1", "kind": "Widget", "metadata": map[string]interface{}{"name": "w"}},
			wantGroup: "custom.io", wantRes: "Widget",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := extractGVK(tt.obj)
			if info.group != tt.wantGroup {
				t.Errorf("group=%s, want %s", info.group, tt.wantGroup)
			}
			if info.resource != tt.wantRes {
				t.Errorf("resource=%s, want %s", info.resource, tt.wantRes)
			}
		})
	}
}

func TestSplitAPIVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"apps/v1", "apps"},
		{"v1", ""},
		{"custom.io/v1beta1", "custom.io"},
	}
	for _, tt := range tests {
		if got := splitAPIVersion(tt.input); got != tt.want {
			t.Errorf("splitAPIVersion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
