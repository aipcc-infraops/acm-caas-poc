package clusterset

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

func fakeGlobalClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRManagedClusterSet:        "ManagedClusterSetList",
			client.GVRManagedClusterSetBinding: "ManagedClusterSetBindingList",
			client.GVRManagedCluster:           "ManagedClusterList",
		}, objs...)
	return &client.Client{Dynamic: fake}
}

func globalSet() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta2",
			"kind":       "ManagedClusterSet",
			"metadata": map[string]interface{}{
				"name": "global",
				"labels": map[string]interface{}{
					"acmlab.redhat.com/global": "true",
				},
			},
		},
	}
}

func TestEnableGlobal(t *testing.T) {
	c := fakeGlobalClient()
	m := New(c, config.Config{}, discardLogger)

	if err := m.EnableGlobal(context.Background()); err != nil {
		t.Fatalf("EnableGlobal failed: %v", err)
	}
}

func TestEnableGlobalIdempotent(t *testing.T) {
	c := fakeGlobalClient()
	m := New(c, config.Config{}, discardLogger)

	if err := m.EnableGlobal(context.Background()); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := m.EnableGlobal(context.Background()); err != nil {
		t.Fatalf("second should be idempotent: %v", err)
	}
}

func TestBindGlobal(t *testing.T) {
	c := fakeGlobalClient(globalSet())
	m := New(c, config.Config{}, discardLogger)

	if err := m.BindGlobal(context.Background(), "team-alpha"); err != nil {
		t.Fatalf("BindGlobal failed: %v", err)
	}
}

func TestBindGlobalNoSet(t *testing.T) {
	c := fakeGlobalClient()
	m := New(c, config.Config{}, discardLogger)

	err := m.BindGlobal(context.Background(), "team-alpha")
	if err == nil {
		t.Fatal("expected error when global set does not exist")
	}
}

func TestUnbindGlobal(t *testing.T) {
	c := fakeGlobalClient(globalSet())
	m := New(c, config.Config{}, discardLogger)

	if err := m.BindGlobal(context.Background(), "team-alpha"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if err := m.UnbindGlobal(context.Background(), "team-alpha"); err != nil {
		t.Fatalf("UnbindGlobal failed: %v", err)
	}
}

func TestGlobalStatusDisabled(t *testing.T) {
	c := fakeGlobalClient()
	m := New(c, config.Config{}, discardLogger)

	status, err := m.GlobalStatus(context.Background())
	if err != nil {
		t.Fatalf("GlobalStatus failed: %v", err)
	}
	if status.Enabled {
		t.Error("expected disabled")
	}
}

func TestGlobalStatusEnabled(t *testing.T) {
	c := fakeGlobalClient(globalSet())
	m := New(c, config.Config{}, discardLogger)

	status, err := m.GlobalStatus(context.Background())
	if err != nil {
		t.Fatalf("GlobalStatus failed: %v", err)
	}
	if !status.Enabled {
		t.Error("expected enabled")
	}
}
