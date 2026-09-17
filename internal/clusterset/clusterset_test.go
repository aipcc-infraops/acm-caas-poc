package clusterset

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRManagedClusterSet:        "ManagedClusterSetList",
			client.GVRManagedClusterSetBinding: "ManagedClusterSetBindingList",
			client.GVRManagedCluster:           "ManagedClusterList",
		}, objs...)
	return &client.Client{Dynamic: fake}
}

func TestCreateClusterSet(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	if err := mgr.Create(context.Background(), "team-serving", "serving-ns"); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	cs, err := c.Get(context.Background(), client.GVRManagedClusterSet, "", "team-serving")
	if err != nil {
		t.Fatalf("ManagedClusterSet not created: %v", err)
	}
	selectorType, _, _ := unstructured.NestedString(cs.Object, "spec", "clusterSelector", "selectorType")
	if selectorType != "ExclusiveClusterSetLabel" {
		t.Errorf("selectorType = %q, want ExclusiveClusterSetLabel", selectorType)
	}

	binding, err := c.Get(context.Background(), client.GVRManagedClusterSetBinding, "serving-ns", "team-serving")
	if err != nil {
		t.Fatalf("ManagedClusterSetBinding not created: %v", err)
	}
	boundSet, _, _ := unstructured.NestedString(binding.Object, "spec", "clusterSet")
	if boundSet != "team-serving" {
		t.Errorf("binding.spec.clusterSet = %q, want team-serving", boundSet)
	}
}

func TestCreateIsIdempotent(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	if err := mgr.Create(context.Background(), "team-x", "ns-x"); err != nil {
		t.Fatalf("first Create failed: %v", err)
	}
	if err := mgr.Create(context.Background(), "team-x", "ns-x"); err != nil {
		t.Fatalf("second Create failed (not idempotent): %v", err)
	}
}

func TestCreateSetError(t *testing.T) {
	c := fakeClient()
	fake := c.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("get", "managedclustersets", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("api unavailable")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.Create(context.Background(), "fail", "ns")
	if err == nil {
		t.Fatal("expected error when API fails")
	}
}

func TestCreateBindingError(t *testing.T) {
	c := fakeClient()
	fake := c.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("get", "managedclustersetbindings", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("api unavailable")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.Create(context.Background(), "fail", "ns")
	if err == nil {
		t.Fatal("expected error when binding creation fails")
	}
}

func TestRemoveClusterSet(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	if err := mgr.Create(context.Background(), "team-rm", "rm-ns"); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := mgr.Remove(context.Background(), "team-rm", "rm-ns"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if _, err := c.Get(context.Background(), client.GVRManagedClusterSet, "", "team-rm"); err == nil {
		t.Error("ManagedClusterSet still exists after remove")
	}
	if _, err := c.Get(context.Background(), client.GVRManagedClusterSetBinding, "rm-ns", "team-rm"); err == nil {
		t.Error("ManagedClusterSetBinding still exists after remove")
	}
}

func TestRemoveNonexistentReturnsError(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.Remove(context.Background(), "nonexistent", "ns")
	if err == nil {
		t.Fatal("Remove of nonexistent ClusterSet should return an error")
	}
}

func TestRemoveBindingError(t *testing.T) {
	c := fakeClient()
	fake := c.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("delete", "managedclustersetbindings", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("delete blocked")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.Remove(context.Background(), "fail", "ns")
	if err == nil {
		t.Fatal("expected error when delete fails")
	}
}

func TestRemoveSetError(t *testing.T) {
	c := fakeClient()
	fake := c.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("delete", "managedclustersets", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("delete blocked")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.Remove(context.Background(), "fail", "ns")
	if err == nil {
		t.Fatal("expected error when set delete fails")
	}
}

func TestListClusterSets(t *testing.T) {
	cs := &unstructured.Unstructured{}
	cs.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1beta2", Kind: "ManagedClusterSet",
	})
	cs.SetName("team-serving")

	mc1 := &unstructured.Unstructured{}
	mc1.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	mc1.SetName("spoke1")
	mc1.SetLabels(map[string]string{clusterSetLabel: "team-serving"})

	mc2 := &unstructured.Unstructured{}
	mc2.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	mc2.SetName("spoke2")
	mc2.SetLabels(map[string]string{clusterSetLabel: "team-serving"})

	mc3 := &unstructured.Unstructured{}
	mc3.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	mc3.SetName("spoke3")

	c := fakeClient(cs, mc1, mc2, mc3)
	mgr := New(c, config.Config{}, discardLogger)

	sets, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("got %d sets, want 1", len(sets))
	}
	if sets[0].Name != "team-serving" {
		t.Errorf("name = %q, want team-serving", sets[0].Name)
	}
	if sets[0].Count != 2 {
		t.Errorf("count = %d, want 2", sets[0].Count)
	}
	if len(sets[0].Members) != 2 {
		t.Errorf("members = %v, want [spoke1 spoke2]", sets[0].Members)
	}
}

func TestListEmpty(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	sets, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(sets) != 0 {
		t.Errorf("got %d sets, want 0", len(sets))
	}
}

func TestListSetsError(t *testing.T) {
	c := fakeClient()
	fake := c.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("list", "managedclustersets", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("list blocked")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.List(context.Background())
	if err == nil {
		t.Fatal("expected error when list fails")
	}
}

func TestListClustersError(t *testing.T) {
	cs := &unstructured.Unstructured{}
	cs.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1beta2", Kind: "ManagedClusterSet",
	})
	cs.SetName("team-x")

	c := fakeClient(cs)
	fake := c.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("list", "managedclusters", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("list blocked")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.List(context.Background())
	if err == nil {
		t.Fatal("expected error when cluster list fails")
	}
}

func TestListClusterNoLabels(t *testing.T) {
	cs := &unstructured.Unstructured{}
	cs.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1beta2", Kind: "ManagedClusterSet",
	})
	cs.SetName("empty-set")

	mc := &unstructured.Unstructured{}
	mc.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	mc.SetName("spoke-nolabel")

	c := fakeClient(cs, mc)
	mgr := New(c, config.Config{}, discardLogger)

	sets, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if sets[0].Count != 0 {
		t.Errorf("count = %d, want 0 for cluster without label", sets[0].Count)
	}
}

func TestAssignCluster(t *testing.T) {
	mc := &unstructured.Unstructured{}
	mc.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	mc.SetName("spoke1")

	c := fakeClient(mc)
	mgr := New(c, config.Config{}, discardLogger)

	if err := mgr.Assign(context.Background(), "spoke1", "team-serving"); err != nil {
		t.Fatalf("Assign failed: %v", err)
	}

	updated, _ := c.Get(context.Background(), client.GVRManagedCluster, "", "spoke1")
	labels := updated.GetLabels()
	if labels[clusterSetLabel] != "team-serving" {
		t.Errorf("label = %q, want team-serving", labels[clusterSetLabel])
	}
}

func TestAssignError(t *testing.T) {
	c := fakeClient()
	fake := c.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("patch", "managedclusters", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("patch blocked")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.Assign(context.Background(), "spoke1", "team-x")
	if err == nil {
		t.Fatal("expected error when patch fails")
	}
}
