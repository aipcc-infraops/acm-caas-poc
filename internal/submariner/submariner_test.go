package submariner

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

var subGVRKinds = map[schema.GroupVersionResource]string{
	client.GVRManagedCluster:      "ManagedClusterList",
	client.GVRManagedClusterAddOn: "ManagedClusterAddOnList",
	client.GVRSubmarinerConfig:    "SubmarinerConfigList",
	client.GVRNamespace:           "NamespaceList",
}

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, subGVRKinds, objs...)
	return &client.Client{Dynamic: fc}
}

func newTestManager(objs ...runtime.Object) *Manager {
	return New(fakeClient(objs...), config.Config{}, discardLogger)
}

func managedCluster(name, clusterSet string) *unstructured.Unstructured {
	labels := map[string]interface{}{}
	if clusterSet != "" {
		labels["cluster.open-cluster-management.io/clusterset"] = clusterSet
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name":   name,
				"labels": labels,
			},
		},
	}
}

func submarinerCluster(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": name,
				"labels": map[string]interface{}{
					"submariner":                                          "enabled",
					"cluster.open-cluster-management.io/clusterset":       "prod-set",
				},
			},
		},
	}
}

func addOnWithStatus(cluster string, conditions []interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "addon.open-cluster-management.io/v1alpha1",
			"kind":       "ManagedClusterAddOn",
			"metadata": map[string]interface{}{
				"name":      "submariner",
				"namespace": cluster,
			},
			"status": map[string]interface{}{
				"conditions": conditions,
			},
		},
	}
}

func TestEnable(t *testing.T) {
	c1 := managedCluster("spoke1", "prod-set")
	c2 := managedCluster("spoke2", "prod-set")
	mgr := newTestManager(c1, c2)

	err := mgr.Enable(context.Background(), "prod-set")
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}

	addon, err := mgr.client.Get(context.Background(), client.GVRManagedClusterAddOn, "spoke1", "submariner")
	if err != nil {
		t.Fatalf("addon not created for spoke1: %v", err)
	}
	ns, _, _ := unstructured.NestedString(addon.Object, "spec", "installNamespace")
	if ns != "submariner-operator" {
		t.Errorf("expected installNamespace=submariner-operator, got %s", ns)
	}

	cfg, err := mgr.client.Get(context.Background(), client.GVRSubmarinerConfig, "spoke1", "submariner")
	if err != nil {
		t.Fatalf("config not created for spoke1: %v", err)
	}
	driver, _, _ := unstructured.NestedString(cfg.Object, "spec", "cableDriver")
	if driver != "libreswan" {
		t.Errorf("expected cableDriver=libreswan, got %s", driver)
	}
}

func TestEnableEmptyClusterSet(t *testing.T) {
	mgr := newTestManager()

	err := mgr.Enable(context.Background(), "empty-set")
	if err == nil {
		t.Fatal("expected error for empty cluster set")
	}
}

func TestDisable(t *testing.T) {
	c1 := managedCluster("spoke1", "prod-set")
	addon := buildSubmarinerAddOn("spoke1")
	cfg := buildSubmarinerConfig("spoke1")
	mgr := newTestManager(c1, addon, cfg)

	err := mgr.Disable(context.Background(), "prod-set")
	if err != nil {
		t.Fatalf("Disable: %v", err)
	}

	_, err = mgr.client.Get(context.Background(), client.GVRManagedClusterAddOn, "spoke1", "submariner")
	if err == nil {
		t.Error("expected addon to be deleted")
	}
}

func TestStatusConnected(t *testing.T) {
	c1 := managedCluster("spoke1", "prod-set")
	addon := addOnWithStatus("spoke1", []interface{}{
		map[string]interface{}{
			"type":   "SubmarinerGatewayNodesLabeled",
			"status": "True",
		},
		map[string]interface{}{
			"type":   "SubmarinerAgentDegraded",
			"status": "False",
		},
		map[string]interface{}{
			"type":   "SubmarinerConnectionsEstablished",
			"status": "True",
		},
	})
	mgr := newTestManager(c1, addon)

	status, err := mgr.Status(context.Background(), "prod-set")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !status.Connected {
		t.Error("expected Connected=true")
	}
	if len(status.Clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(status.Clusters))
	}
	if !status.Clusters[0].GatewayReady {
		t.Error("expected GatewayReady=true")
	}
	if status.Clusters[0].Connections != 1 {
		t.Error("expected Connections=1")
	}
}

func TestStatusDisconnected(t *testing.T) {
	c1 := managedCluster("spoke1", "prod-set")
	mgr := newTestManager(c1)

	status, err := mgr.Status(context.Background(), "prod-set")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Connected {
		t.Error("expected Connected=false when addon missing")
	}
}

func TestList(t *testing.T) {
	c1 := submarinerCluster("spoke1")
	c2 := submarinerCluster("spoke2")
	mgr := newTestManager(c1, c2)

	infos, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 cluster set, got %d", len(infos))
	}
	if infos[0].ClusterSet != "prod-set" {
		t.Errorf("expected prod-set, got %s", infos[0].ClusterSet)
	}
	if infos[0].Clusters != 2 {
		t.Errorf("expected 2 clusters, got %d", infos[0].Clusters)
	}
}

func TestListEmpty(t *testing.T) {
	mgr := newTestManager()

	infos, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("expected 0 infos, got %d", len(infos))
	}
}

func TestBuildSubmarinerAddOn(t *testing.T) {
	addon := buildSubmarinerAddOn("spoke1")
	name, _, _ := unstructured.NestedString(addon.Object, "metadata", "name")
	if name != "submariner" {
		t.Errorf("expected name=submariner, got %s", name)
	}
	ns, _, _ := unstructured.NestedString(addon.Object, "metadata", "namespace")
	if ns != "spoke1" {
		t.Errorf("expected namespace=spoke1, got %s", ns)
	}
}

func TestBuildSubmarinerConfig(t *testing.T) {
	cfg := buildSubmarinerConfig("spoke1")
	driver, _, _ := unstructured.NestedString(cfg.Object, "spec", "cableDriver")
	if driver != "libreswan" {
		t.Errorf("expected cableDriver=libreswan, got %s", driver)
	}
	port, _, _ := unstructured.NestedFieldNoCopy(cfg.Object, "spec", "IPSecNATTPort")
	if port != int64(4500) {
		t.Errorf("expected port=4500, got %v", port)
	}
}

func TestParseClusterStatusHealthy(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "SubmarinerGatewayNodesLabeled", "status": "True"},
				map[string]interface{}{"type": "SubmarinerAgentDegraded", "status": "False"},
				map[string]interface{}{"type": "SubmarinerConnectionsEstablished", "status": "True"},
			},
		},
	}
	cs := parseClusterStatus("spoke1", obj)
	if !cs.GatewayReady {
		t.Error("expected GatewayReady")
	}
	if !cs.AgentReady {
		t.Error("expected AgentReady")
	}
	if cs.Connections != 1 {
		t.Error("expected 1 connection")
	}
}

func TestParseClusterStatusNoConditions(t *testing.T) {
	cs := parseClusterStatus("spoke1", map[string]interface{}{})
	if cs.GatewayReady || cs.AgentReady || cs.Connections != 0 {
		t.Error("expected all false/zero for empty object")
	}
}

func TestGroupByClusterSet(t *testing.T) {
	clusters := []map[string]interface{}{
		{
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"cluster.open-cluster-management.io/clusterset": "set-a",
				},
			},
		},
		{
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"cluster.open-cluster-management.io/clusterset": "set-a",
				},
			},
		},
		{
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"cluster.open-cluster-management.io/clusterset": "set-b",
				},
			},
		},
	}
	infos := groupByClusterSet(clusters)
	if len(infos) != 2 {
		t.Fatalf("expected 2 sets, got %d", len(infos))
	}
	if infos[0].Clusters != 2 {
		t.Errorf("set-a: expected 2 clusters, got %d", infos[0].Clusters)
	}
	if infos[1].Clusters != 1 {
		t.Errorf("set-b: expected 1 cluster, got %d", infos[1].Clusters)
	}
}

func TestGroupByClusterSetUnassigned(t *testing.T) {
	clusters := []map[string]interface{}{
		{"metadata": map[string]interface{}{"labels": map[string]interface{}{}}},
	}
	infos := groupByClusterSet(clusters)
	if len(infos) != 1 || infos[0].ClusterSet != "unassigned" {
		t.Errorf("expected unassigned, got %v", infos)
	}
}

func TestEnableMultipleClusters(t *testing.T) {
	c1 := managedCluster("spoke1", "multi-set")
	c2 := managedCluster("spoke2", "multi-set")
	c3 := managedCluster("spoke3", "multi-set")
	mgr := newTestManager(c1, c2, c3)

	err := mgr.Enable(context.Background(), "multi-set")
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}

	for _, name := range []string{"spoke1", "spoke2", "spoke3"} {
		_, err := mgr.client.Get(context.Background(), client.GVRManagedClusterAddOn, name, "submariner")
		if err != nil {
			t.Errorf("addon not created for %s: %v", name, err)
		}
		_, err = mgr.client.Get(context.Background(), client.GVRSubmarinerConfig, name, "submariner")
		if err != nil {
			t.Errorf("config not created for %s: %v", name, err)
		}
	}
}
