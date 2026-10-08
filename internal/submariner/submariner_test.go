package submariner

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

var subGVRKinds = map[schema.GroupVersionResource]string{
	client.GVRManagedCluster:       "ManagedClusterList",
	client.GVRManagedClusterAddOn:  "ManagedClusterAddOnList",
	client.GVRSubmarinerConfig:     "SubmarinerConfigList",
	client.GVRNamespace:            "NamespaceList",
	client.GVRSubmarinerBroker:     "BrokerList",
	client.GVRManagedClusterSet:    "ManagedClusterSetList",
	client.GVRClusterDeployment:    "ClusterDeploymentList",
	client.GVRSecret:               "SecretList",
	client.GVRSubmarinerEndpoint:   "EndpointList",
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

func healthyAddon(cluster string) *unstructured.Unstructured {
	return addOnWithStatus(cluster, []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
		map[string]interface{}{"type": "SubmarinerBrokerConfigApplied", "status": "True"},
		map[string]interface{}{"type": "SubmarinerGatewayNodesLabeled", "status": "True"},
		map[string]interface{}{"type": "SubmarinerAgentDegraded", "status": "False"},
		map[string]interface{}{"type": "SubmarinerConnectionDegraded", "status": "False"},
	})
}

func TestEnable(t *testing.T) {
	c1 := managedCluster("spoke1", "prod-set")
	c2 := managedCluster("spoke2", "prod-set")
	mgr := newTestManager(c1, c2)

	err := mgr.Enable(context.Background(), "prod-set", EnableOpts{})
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

	err := mgr.Enable(context.Background(), "empty-set", EnableOpts{})
	if err == nil {
		t.Fatal("expected error for empty cluster set")
	}
}

func TestDisable(t *testing.T) {
	c1 := managedCluster("spoke1", "prod-set")
	addon := buildSubmarinerAddOn("spoke1")
	cfg := buildSubmarinerConfig("spoke1", SubmarinerConfigOpts{})
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
			"type":   "SubmarinerBrokerConfigApplied",
			"status": "True",
		},
		map[string]interface{}{
			"type":   "SubmarinerGatewayNodesLabeled",
			"status": "True",
		},
		map[string]interface{}{
			"type":   "SubmarinerAgentDegraded",
			"status": "False",
		},
		map[string]interface{}{
			"type":   "SubmarinerConnectionDegraded",
			"status": "False",
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
	cfg := buildSubmarinerConfig("spoke1", SubmarinerConfigOpts{})
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
				map[string]interface{}{"type": "SubmarinerConnectionDegraded", "status": "False"},
			},
		},
	}
	cs := parseClusterStatus("spoke1", obj)
	if !cs.GatewayReady {
		t.Error("expected GatewayReady")
	}
	if cs.ConnectionDegraded {
		t.Error("expected ConnectionDegraded=false")
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

func TestParseClusterStatusAvailableAndConnectionDegraded(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Available", "status": "True"},
				map[string]interface{}{"type": "SubmarinerGatewayNodesLabeled", "status": "True"},
				map[string]interface{}{"type": "SubmarinerAgentDegraded", "status": "False"},
				map[string]interface{}{
					"type":    "SubmarinerConnectionDegraded",
					"status":  "True",
					"reason":  "ConnectionsNotEstablished",
					"message": "There are no connections on gateways",
				},
			},
		},
	}
	cs := parseClusterStatus("spoke1", obj)
	if !cs.AddonAvailable {
		t.Error("expected AddonAvailable=true")
	}
	if !cs.GatewayReady {
		t.Error("expected GatewayReady=true")
	}
	if !cs.AgentReady {
		t.Error("expected AgentReady=true")
	}
	if !cs.ConnectionDegraded {
		t.Error("expected ConnectionDegraded=true")
	}
	if cs.Reason != "ConnectionsNotEstablished" {
		t.Errorf("expected reason=ConnectionsNotEstablished, got %s", cs.Reason)
	}
	if cs.Message != "There are no connections on gateways" {
		t.Errorf("expected message about no connections, got %s", cs.Message)
	}
	if cs.Connections != 0 {
		t.Errorf("expected connections=0, got %d", cs.Connections)
	}
}

func TestParseClusterStatusConnectionNotDegraded(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "SubmarinerConnectionDegraded", "status": "False"},
			},
		},
	}
	cs := parseClusterStatus("spoke1", obj)
	if cs.ConnectionDegraded {
		t.Error("expected ConnectionDegraded=false")
	}
	if cs.Reason != "" {
		t.Errorf("expected empty reason, got %s", cs.Reason)
	}
}

func TestParseClusterStatusAvailableFalse(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Available", "status": "False"},
			},
		},
	}
	cs := parseClusterStatus("spoke1", obj)
	if cs.AddonAvailable {
		t.Error("expected AddonAvailable=false")
	}
}

func TestParseClusterStatusInferConnectionsWhenConditionAbsent(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Available", "status": "True"},
				map[string]interface{}{"type": "SubmarinerBrokerConfigApplied", "status": "True"},
				map[string]interface{}{"type": "SubmarinerGatewayNodesLabeled", "status": "True"},
				map[string]interface{}{"type": "SubmarinerAgentDegraded", "status": "False"},
			},
		},
	}
	cs := parseClusterStatus("spoke1", obj)
	if cs.Connections != 1 {
		t.Errorf("expected Connections=1 when addon healthy and no ConnectionDegraded condition, got %d", cs.Connections)
	}
	if cs.ConnectionDegraded {
		t.Error("expected ConnectionDegraded=false")
	}
}

func TestStatusIncludesConnectionDegraded(t *testing.T) {
	c1 := managedCluster("spoke1", "prod-set")
	addon := addOnWithStatus("spoke1", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
		map[string]interface{}{"type": "SubmarinerGatewayNodesLabeled", "status": "True"},
		map[string]interface{}{"type": "SubmarinerAgentDegraded", "status": "False"},
		map[string]interface{}{
			"type":    "SubmarinerConnectionDegraded",
			"status":  "True",
			"reason":  "ConnectionsNotEstablished",
			"message": "There are no connections on gateways",
		},
	})
	mgr := newTestManager(c1, addon)

	status, err := mgr.Status(context.Background(), "prod-set")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Connected {
		t.Error("expected Connected=false when connection degraded")
	}
	if !status.Clusters[0].ConnectionDegraded {
		t.Error("expected ConnectionDegraded=true in cluster status")
	}
	if status.Clusters[0].Reason != "ConnectionsNotEstablished" {
		t.Errorf("expected reason in JSON, got %s", status.Clusters[0].Reason)
	}
}

func TestStatusDisconnectedWhenZeroConnections(t *testing.T) {
	c1 := managedCluster("spoke1", "prod-set")
	addon := addOnWithStatus("spoke1", []interface{}{
		map[string]interface{}{"type": "SubmarinerGatewayNodesLabeled", "status": "True"},
		map[string]interface{}{"type": "SubmarinerAgentDegraded", "status": "False"},
		map[string]interface{}{"type": "SubmarinerConnectionDegraded", "status": "True", "reason": "ConnectionsNotEstablished", "message": "connections degraded"},
	})
	mgr := newTestManager(c1, addon)

	status, err := mgr.Status(context.Background(), "prod-set")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Connected {
		t.Error("expected Connected=false when connections degraded")
	}
}

func TestEnableMultipleClusters(t *testing.T) {
	c1 := managedCluster("spoke1", "multi-set")
	c2 := managedCluster("spoke2", "multi-set")
	c3 := managedCluster("spoke3", "multi-set")
	mgr := newTestManager(c1, c2, c3)

	err := mgr.Enable(context.Background(), "multi-set", EnableOpts{})
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

func TestEnableWithGlobalnet(t *testing.T) {
	c1 := managedCluster("spoke1", "gn-set")
	c2 := managedCluster("spoke2", "gn-set")
	mgr := newTestManager(c1, c2)

	err := mgr.Enable(context.Background(), "gn-set", EnableOpts{Globalnet: true})
	if err != nil {
		t.Fatalf("Enable with Globalnet: %v", err)
	}

	for i, name := range []string{"spoke1", "spoke2"} {
		cfg, err := mgr.client.Get(context.Background(), client.GVRSubmarinerConfig, name, "submariner")
		if err != nil {
			t.Fatalf("config not created for %s: %v", name, err)
		}
		globalCIDR, _, _ := unstructured.NestedString(cfg.Object, "spec", "globalCIDR")
		expected := defaultGlobalCIDR(i)
		if globalCIDR != expected {
			t.Errorf("%s: globalCIDR = %q, want %q", name, globalCIDR, expected)
		}
	}
}

func TestEnableWithoutGlobalnetHasNoGlobalCIDR(t *testing.T) {
	c1 := managedCluster("spoke1", "std-set")
	c2 := managedCluster("spoke2", "std-set")
	mgr := newTestManager(c1, c2)

	err := mgr.Enable(context.Background(), "std-set", EnableOpts{})
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}

	cfg, err := mgr.client.Get(context.Background(), client.GVRSubmarinerConfig, "spoke1", "submariner")
	if err != nil {
		t.Fatalf("config not created: %v", err)
	}
	globalCIDR, exists, _ := unstructured.NestedString(cfg.Object, "spec", "globalCIDR")
	if exists && globalCIDR != "" {
		t.Errorf("expected no globalCIDR without --globalnet, got %q", globalCIDR)
	}
}

func TestWaitForReadyAlreadyConnected(t *testing.T) {
	c1 := managedCluster("spoke1", "ready-set")
	addon := addOnWithStatus("spoke1", []interface{}{
		map[string]interface{}{"type": "SubmarinerBrokerConfigApplied", "status": "True"},
		map[string]interface{}{"type": "SubmarinerGatewayNodesLabeled", "status": "True"},
		map[string]interface{}{"type": "SubmarinerAgentDegraded", "status": "False"},
		map[string]interface{}{"type": "SubmarinerConnectionDegraded", "status": "False"},
	})
	mgr := newTestManager(c1, addon)

	status, err := mgr.WaitForReady(context.Background(), "ready-set", 5*time.Second)
	if err != nil {
		t.Fatalf("WaitForReady: %v", err)
	}
	if !status.Connected {
		t.Error("expected Connected=true")
	}
}

func TestWaitForReadyTimeout(t *testing.T) {
	c1 := managedCluster("spoke1", "slow-set")
	mgr := newTestManager(c1)

	_, err := mgr.WaitForReady(context.Background(), "slow-set", 1*time.Second)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestDefaultGlobalCIDR(t *testing.T) {
	tests := []struct {
		index int
		want  string
	}{
		{0, "242.0.0.0/16"},
		{1, "242.1.0.0/16"},
		{255, "242.255.0.0/16"},
	}
	for _, tt := range tests {
		got := defaultGlobalCIDR(tt.index)
		if got != tt.want {
			t.Errorf("defaultGlobalCIDR(%d) = %q, want %q", tt.index, got, tt.want)
		}
	}
}

func TestBuildSubmarinerConfigGlobalnet(t *testing.T) {
	cfg := buildSubmarinerConfig("spoke1", SubmarinerConfigOpts{GlobalCIDR: "242.0.0.0/16"})
	globalCIDR, _, _ := unstructured.NestedString(cfg.Object, "spec", "globalCIDR")
	if globalCIDR != "242.0.0.0/16" {
		t.Errorf("expected globalCIDR=242.0.0.0/16, got %s", globalCIDR)
	}
}

func TestBuildSubmarinerConfigNoGlobalnet(t *testing.T) {
	cfg := buildSubmarinerConfig("spoke1", SubmarinerConfigOpts{})
	_, exists, _ := unstructured.NestedString(cfg.Object, "spec", "globalCIDR")
	if exists {
		t.Error("expected no globalCIDR in spec without opts")
	}
}

func TestBuildSubmarinerConfigForceUDPEncaps(t *testing.T) {
	cfg := buildSubmarinerConfig("spoke1", SubmarinerConfigOpts{ForceUDPEncaps: true})
	val, found, _ := unstructured.NestedBool(cfg.Object, "spec", "forceUDPEncaps")
	if !found || !val {
		t.Error("expected forceUDPEncaps=true")
	}
}

func TestBuildSubmarinerConfigLoadBalancer(t *testing.T) {
	cfg := buildSubmarinerConfig("spoke1", SubmarinerConfigOpts{LoadBalancer: true})
	val, found, _ := unstructured.NestedBool(cfg.Object, "spec", "loadBalancerEnable")
	if !found || !val {
		t.Error("expected loadBalancerEnable=true")
	}
}

func TestBuildSubmarinerConfigNoExtraFields(t *testing.T) {
	cfg := buildSubmarinerConfig("spoke1", SubmarinerConfigOpts{})
	_, found, _ := unstructured.NestedBool(cfg.Object, "spec", "forceUDPEncaps")
	if found {
		t.Error("forceUDPEncaps should not be present when disabled")
	}
	_, found, _ = unstructured.NestedBool(cfg.Object, "spec", "loadBalancerEnable")
	if found {
		t.Error("loadBalancerEnable should not be present when disabled")
	}
}

func TestBuildBrokerCR(t *testing.T) {
	broker := buildBrokerCR("test-broker")
	name, _, _ := unstructured.NestedString(broker.Object, "metadata", "name")
	if name != "submariner-broker" {
		t.Errorf("expected name=submariner-broker, got %s", name)
	}
	ns, _, _ := unstructured.NestedString(broker.Object, "metadata", "namespace")
	if ns != "test-broker" {
		t.Errorf("expected namespace=test-broker, got %s", ns)
	}
	components, _, _ := unstructured.NestedStringSlice(broker.Object, "spec", "components")
	if len(components) != 2 || components[0] != "service-discovery" || components[1] != "connectivity" {
		t.Errorf("expected [service-discovery, connectivity], got %v", components)
	}
}

func TestEnableCreatesBrokerCR(t *testing.T) {
	c1 := managedCluster("spoke1", "broker-test")
	c2 := managedCluster("spoke2", "broker-test")
	mgr := newTestManager(c1, c2)

	err := mgr.Enable(context.Background(), "broker-test", EnableOpts{})
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}

	_, err = mgr.client.Get(context.Background(), client.GVRSubmarinerBroker, "broker-test-broker", "submariner-broker")
	if err != nil {
		t.Fatalf("Broker CR not created: %v", err)
	}
}

func TestBuildSubmarinerConfigNATTDiscoveryPort(t *testing.T) {
	cfg := buildSubmarinerConfig("spoke1", SubmarinerConfigOpts{})
	port, _, _ := unstructured.NestedFieldNoCopy(cfg.Object, "spec", "NATTDiscoveryPort")
	if port != int64(4490) {
		t.Errorf("expected NATTDiscoveryPort=4490, got %v", port)
	}
}

func TestDetectPlatformIBMCloud(t *testing.T) {
	c1 := managedCluster("ibm1", "ibm-set")
	cd := testClusterDeploymentIBM("ibm1")
	mgr := newTestManager(c1, cd)

	platform := mgr.detectPlatform(context.Background(), "ibm1")
	if platform != "ibmcloud" {
		t.Errorf("expected ibmcloud, got %q", platform)
	}
}

func TestDetectPlatformUnknown(t *testing.T) {
	c1 := managedCluster("spoke1", "test-set")
	mgr := newTestManager(c1)

	platform := mgr.detectPlatform(context.Background(), "spoke1")
	if platform != "" {
		t.Errorf("expected empty platform, got %q", platform)
	}
}

func TestEnableIBMCloudAutoForceUDPEncaps(t *testing.T) {
	c1 := managedCluster("ibm1", "ibm-set")
	cd := testClusterDeploymentIBM("ibm1")
	mgr := newTestManager(c1, cd)

	err := mgr.Enable(context.Background(), "ibm-set", EnableOpts{})
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}

	cfg, err := mgr.client.Get(context.Background(), client.GVRSubmarinerConfig, "ibm1", "submariner")
	if err != nil {
		t.Fatalf("config not found: %v", err)
	}
	udp, _, _ := unstructured.NestedBool(cfg.Object, "spec", "forceUDPEncaps")
	if !udp {
		t.Error("expected forceUDPEncaps=true auto-set for IBM Cloud")
	}
}

func TestEnableIBMCloudNoAutoWhenLoadBalancer(t *testing.T) {
	c1 := managedCluster("ibm1", "ibm-set2")
	cd := testClusterDeploymentIBM("ibm1")
	mgr := newTestManager(c1, cd)

	err := mgr.Enable(context.Background(), "ibm-set2", EnableOpts{LoadBalancer: true})
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}

	cfg, err := mgr.client.Get(context.Background(), client.GVRSubmarinerConfig, "ibm1", "submariner")
	if err != nil {
		t.Fatalf("config not found: %v", err)
	}
	udp, found, _ := unstructured.NestedBool(cfg.Object, "spec", "forceUDPEncaps")
	if found && udp {
		t.Error("expected forceUDPEncaps NOT auto-set when LoadBalancer explicitly requested")
	}
}

func TestParseEndpoint(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"cluster_id":  "cluster-a",
			"public_ip":   "1.2.3.4",
			"private_ip":  "10.0.0.1",
			"nat_enabled": true,
			"backend":     "libreswan",
			"backend_config": map[string]interface{}{
				"udp-port":            "4500",
				"natt-discovery-port": "4490",
			},
		},
	}
	info := parseEndpoint(obj)
	if info.ClusterID != "cluster-a" {
		t.Errorf("expected cluster-a, got %s", info.ClusterID)
	}
	if info.PublicIP != "1.2.3.4" {
		t.Errorf("expected 1.2.3.4, got %s", info.PublicIP)
	}
	if info.PrivateIP != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %s", info.PrivateIP)
	}
	if !info.NATEnabled {
		t.Error("expected NATEnabled=true")
	}
	if info.Backend != "libreswan" {
		t.Errorf("expected libreswan, got %s", info.Backend)
	}
	if info.UDPPort != "4500" {
		t.Errorf("expected 4500, got %s", info.UDPPort)
	}
	if info.NATTDiscoveryPort != "4490" {
		t.Errorf("expected 4490, got %s", info.NATTDiscoveryPort)
	}
}

func TestEnableForceUDPEncapsAndLoadBalancer(t *testing.T) {
	c1 := managedCluster("spoke1", "flags-set")
	mgr := newTestManager(c1)

	err := mgr.Enable(context.Background(), "flags-set", EnableOpts{
		ForceUDPEncaps: true,
		LoadBalancer:   true,
	})
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}

	cfg, err := mgr.client.Get(context.Background(), client.GVRSubmarinerConfig, "spoke1", "submariner")
	if err != nil {
		t.Fatalf("config not found: %v", err)
	}
	udp, _, _ := unstructured.NestedBool(cfg.Object, "spec", "forceUDPEncaps")
	if !udp {
		t.Error("expected forceUDPEncaps=true in SubmarinerConfig")
	}
	lb, _, _ := unstructured.NestedBool(cfg.Object, "spec", "loadBalancerEnable")
	if !lb {
		t.Error("expected loadBalancerEnable=true in SubmarinerConfig")
	}
}
