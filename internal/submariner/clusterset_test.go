package submariner

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

var clusterSetGVRKinds = map[schema.GroupVersionResource]string{
	client.GVRManagedCluster:    "ManagedClusterList",
	client.GVRManagedClusterSet: "ManagedClusterSetList",
}

func newTestManagerWithClusterSets(objs ...runtime.Object) *Manager {
	scheme := runtime.NewScheme()
	merged := map[schema.GroupVersionResource]string{}
	for k, v := range subGVRKinds {
		merged[k] = v
	}
	for k, v := range clusterSetGVRKinds {
		merged[k] = v
	}
	fc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, merged, objs...)
	c := &client.Client{Dynamic: fc}
	return New(c, config.Config{}, discardLogger)
}

func TestCreateTestSetSuccess(t *testing.T) {
	c1 := managedCluster("spoke1", "default")
	c2 := managedCluster("spoke2", "default")
	mgr := newTestManagerWithClusterSets(c1, c2)

	err := mgr.CreateTestSet(context.Background(), CreateTestSetOpts{
		Name:     "uc26-test",
		Clusters: []string{"spoke1", "spoke2"},
		Confirm:  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cs, err := mgr.client.Get(context.Background(), client.GVRManagedClusterSet, "", "uc26-test")
	if err != nil {
		t.Fatalf("ClusterSet not created: %v", err)
	}
	if cs.GetName() != "uc26-test" {
		t.Errorf("expected name=uc26-test, got %s", cs.GetName())
	}
}

func TestCreateTestSetTooFewClusters(t *testing.T) {
	c1 := managedCluster("spoke1", "default")
	mgr := newTestManagerWithClusterSets(c1)

	err := mgr.CreateTestSet(context.Background(), CreateTestSetOpts{
		Name:     "uc26-test",
		Clusters: []string{"spoke1"},
		Confirm:  true,
	})
	if err == nil {
		t.Fatal("expected error for single cluster")
	}
}

func TestCreateTestSetNoConfirm(t *testing.T) {
	c1 := managedCluster("spoke1", "default")
	c2 := managedCluster("spoke2", "default")
	mgr := newTestManagerWithClusterSets(c1, c2)

	err := mgr.CreateTestSet(context.Background(), CreateTestSetOpts{
		Name:     "uc26-test",
		Clusters: []string{"spoke1", "spoke2"},
		Confirm:  false,
	})
	if err == nil {
		t.Fatal("expected error when --confirm not set")
	}
}

func TestCreateTestSetClusterNotFound(t *testing.T) {
	c1 := managedCluster("spoke1", "default")
	mgr := newTestManagerWithClusterSets(c1)

	err := mgr.CreateTestSet(context.Background(), CreateTestSetOpts{
		Name:     "uc26-test",
		Clusters: []string{"spoke1", "nonexistent"},
		Confirm:  true,
	})
	if err == nil {
		t.Fatal("expected error for nonexistent cluster")
	}
}

func TestBuildManagedClusterSet(t *testing.T) {
	cs := buildManagedClusterSet("test-set")
	if cs.GetName() != "test-set" {
		t.Errorf("expected name=test-set, got %s", cs.GetName())
	}
	labels := cs.GetLabels()
	if labels["acmlab.redhat.com/managed"] != "true" {
		t.Error("expected managed label")
	}
}
