package submariner

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func testSecret(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

func testBrokerCR(clusterSet string) *unstructured.Unstructured {
	return buildBrokerCR(clusterSet + "-broker")
}

func TestDiagnoseEmptyClusterSet(t *testing.T) {
	mgr := newTestManager()

	result, err := mgr.Diagnose(context.Background(), "empty-set")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Healthy {
		t.Error("expected unhealthy for empty set")
	}
	if len(result.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(result.Checks))
	}
	if result.Checks[0].Status != "fail" {
		t.Errorf("expected fail, got %s", result.Checks[0].Status)
	}
}

func TestDiagnoseSingleClusterWarning(t *testing.T) {
	c1 := managedCluster("spoke1", "solo-set")
	addon := addOnWithStatus("spoke1", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
		map[string]interface{}{"type": "SubmarinerGatewayNodesLabeled", "status": "True"},
		map[string]interface{}{"type": "SubmarinerAgentDegraded", "status": "False"},
		map[string]interface{}{"type": "SubmarinerConnectionDegraded", "status": "False"},
	})
	cfg := buildSubmarinerConfig("spoke1", SubmarinerConfigOpts{})
	mgr := newTestManager(c1, addon, cfg)

	result, err := mgr.Diagnose(context.Background(), "solo-set")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Healthy {
		t.Error("expected unhealthy for single cluster")
	}
	found := false
	for _, ch := range result.Checks {
		if ch.Name == "cluster-set" && ch.Status == "fail" {
			found = true
		}
	}
	if !found {
		t.Error("expected fail check for cluster-set with single cluster")
	}
}

func TestDiagnoseLocalClusterWarning(t *testing.T) {
	c1 := managedCluster("spoke1", "mixed-set")
	c2 := managedCluster("local-cluster", "mixed-set")
	addon1 := addOnWithStatus("spoke1", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
		map[string]interface{}{"type": "SubmarinerGatewayNodesLabeled", "status": "True"},
		map[string]interface{}{"type": "SubmarinerAgentDegraded", "status": "False"},
		map[string]interface{}{"type": "SubmarinerConnectionDegraded", "status": "False"},
	})
	addon2 := addOnWithStatus("local-cluster", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
	})
	mgr := newTestManager(c1, c2, addon1, addon2)

	result, err := mgr.Diagnose(context.Background(), "mixed-set")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, ch := range result.Checks {
		if ch.Name == "cluster-set" && ch.Status == "warn" {
			found = true
		}
	}
	if !found {
		t.Error("expected warn about local-cluster in set")
	}
}

func TestDiagnoseNoAddOn(t *testing.T) {
	c1 := managedCluster("spoke1", "test-set")
	c2 := managedCluster("spoke2", "test-set")
	mgr := newTestManager(c1, c2)

	result, err := mgr.Diagnose(context.Background(), "test-set")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Healthy {
		t.Error("expected unhealthy when no addon")
	}
	found := false
	for _, ch := range result.Checks {
		if ch.Name == "addon/spoke1" && ch.Status == "fail" {
			found = true
		}
	}
	if !found {
		t.Error("expected fail for missing addon")
	}
}

func TestDiagnoseConnectionDegradedWithReason(t *testing.T) {
	c1 := managedCluster("spoke1", "test-set")
	c2 := managedCluster("spoke2", "test-set")
	addon1 := addOnWithStatus("spoke1", []interface{}{
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
	addon2 := addOnWithStatus("spoke2", []interface{}{
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
	cfg1 := buildSubmarinerConfig("spoke1", SubmarinerConfigOpts{})
	cfg2 := buildSubmarinerConfig("spoke2", SubmarinerConfigOpts{})
	mgr := newTestManager(c1, c2, addon1, addon2, cfg1, cfg2)

	result, err := mgr.Diagnose(context.Background(), "test-set")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Healthy {
		t.Error("expected unhealthy when connections degraded")
	}
	found := false
	for _, ch := range result.Checks {
		if ch.Name == "addon/spoke1/connections" && ch.Status == "fail" {
			found = true
			if ch.Message == "" {
				t.Error("expected non-empty message for connection failure")
			}
		}
	}
	if !found {
		t.Error("expected fail check for connections")
	}
}

func TestDiagnoseHealthyClusterSet(t *testing.T) {
	c1 := managedCluster("spoke1", "healthy-set")
	c2 := managedCluster("spoke2", "healthy-set")
	addon1 := addOnWithStatus("spoke1", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
		map[string]interface{}{"type": "SubmarinerGatewayNodesLabeled", "status": "True"},
		map[string]interface{}{"type": "SubmarinerAgentDegraded", "status": "False"},
		map[string]interface{}{"type": "SubmarinerConnectionDegraded", "status": "False"},
		map[string]interface{}{"type": "SubmarinerBrokerConfigApplied", "status": "True"},
		map[string]interface{}{"type": "ManifestApplied", "status": "True"},
		map[string]interface{}{"type": "RouteAgentConnectionDegraded", "status": "False"},
	})
	addon2 := addOnWithStatus("spoke2", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
		map[string]interface{}{"type": "SubmarinerGatewayNodesLabeled", "status": "True"},
		map[string]interface{}{"type": "SubmarinerAgentDegraded", "status": "False"},
		map[string]interface{}{"type": "SubmarinerConnectionDegraded", "status": "False"},
		map[string]interface{}{"type": "SubmarinerBrokerConfigApplied", "status": "True"},
		map[string]interface{}{"type": "ManifestApplied", "status": "True"},
		map[string]interface{}{"type": "RouteAgentConnectionDegraded", "status": "False"},
	})
	cfg1 := buildSubmarinerConfig("spoke1", SubmarinerConfigOpts{})
	cfg2 := buildSubmarinerConfig("spoke2", SubmarinerConfigOpts{})
	sec1 := testSecret("spoke1-cloud-creds", "spoke1")
	sec2 := testSecret("spoke2-cloud-creds", "spoke2")
	broker := testBrokerCR("healthy-set")
	mgr := newTestManager(c1, c2, addon1, addon2, cfg1, cfg2, sec1, sec2, broker)

	result, err := mgr.Diagnose(context.Background(), "healthy-set")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Healthy {
		for _, ch := range result.Checks {
			if ch.Status == "fail" {
				t.Errorf("unexpected fail: %s: %s", ch.Name, ch.Message)
			}
		}
	}
}

func TestDiagnoseNoConfig(t *testing.T) {
	c1 := managedCluster("spoke1", "test-set")
	c2 := managedCluster("spoke2", "test-set")
	addon1 := addOnWithStatus("spoke1", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
	})
	addon2 := addOnWithStatus("spoke2", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
	})
	mgr := newTestManager(c1, c2, addon1, addon2)

	result, err := mgr.Diagnose(context.Background(), "test-set")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, ch := range result.Checks {
		if ch.Name == "config/spoke1" && ch.Status == "warn" {
			found = true
		}
	}
	if !found {
		t.Error("expected warn for missing SubmarinerConfig")
	}
}

func TestDiagnoseGatewayNotLabeled(t *testing.T) {
	c1 := managedCluster("spoke1", "test-set")
	c2 := managedCluster("spoke2", "test-set")
	addon := addOnWithStatus("spoke1", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
		map[string]interface{}{"type": "SubmarinerGatewayNodesLabeled", "status": "False", "reason": "NodesNotLabeled", "message": "no nodes labelled"},
	})
	addon2 := addOnWithStatus("spoke2", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
	})
	mgr := newTestManager(c1, c2, addon, addon2)

	result, err := mgr.Diagnose(context.Background(), "test-set")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Healthy {
		t.Error("expected unhealthy when gateway not labeled")
	}
}

func TestDiagnoseAddonUnavailable(t *testing.T) {
	c1 := managedCluster("spoke1", "test-set")
	c2 := managedCluster("spoke2", "test-set")
	addon := addOnWithStatus("spoke1", []interface{}{
		map[string]interface{}{"type": "Available", "status": "False", "reason": "NotReady", "message": "addon not ready"},
	})
	addon2 := addOnWithStatus("spoke2", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
	})
	mgr := newTestManager(c1, c2, addon, addon2)

	result, err := mgr.Diagnose(context.Background(), "test-set")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Healthy {
		t.Error("expected unhealthy when addon unavailable")
	}
	found := false
	for _, ch := range result.Checks {
		if ch.Name == "addon/spoke1/available" && ch.Status == "fail" {
			found = true
		}
	}
	if !found {
		t.Error("expected fail for addon unavailable")
	}
}

func TestExtractConditions(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type":    "Available",
					"status":  "True",
					"reason":  "Ready",
					"message": "addon is available",
				},
				"not-a-map",
			},
		},
	}
	conds := extractConditions(obj)
	if len(conds) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(conds))
	}
	if conds[0].condType != "Available" {
		t.Errorf("expected Available, got %s", conds[0].condType)
	}
	if conds[0].reason != "Ready" {
		t.Errorf("expected Ready, got %s", conds[0].reason)
	}
}

func TestExtractConditionsNoStatus(t *testing.T) {
	conds := extractConditions(map[string]interface{}{})
	if len(conds) != 0 {
		t.Errorf("expected 0 conditions, got %d", len(conds))
	}
}

func TestExtractConditionsNoConditions(t *testing.T) {
	conds := extractConditions(map[string]interface{}{
		"status": map[string]interface{}{},
	})
	if len(conds) != 0 {
		t.Errorf("expected 0 conditions, got %d", len(conds))
	}
}

func TestDiagnoseDefaultClusterSetWarning(t *testing.T) {
	c1 := managedCluster("spoke1", "default")
	c2 := managedCluster("spoke2", "default")
	addon1 := addOnWithStatus("spoke1", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
	})
	addon2 := addOnWithStatus("spoke2", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
	})
	mgr := newTestManager(c1, c2, addon1, addon2)

	result, err := mgr.Diagnose(context.Background(), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, ch := range result.Checks {
		if ch.Name == "cluster-set/naming" && ch.Status == "warn" {
			found = true
		}
	}
	if !found {
		t.Error("expected warn about using default ClusterSet")
	}
}

func TestDiagnoseGlobalnetRecommendation(t *testing.T) {
	c1 := managedCluster("spoke1", "test-set")
	c2 := managedCluster("spoke2", "test-set")
	addon1 := addOnWithStatus("spoke1", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
		map[string]interface{}{"type": "SubmarinerGatewayNodesLabeled", "status": "True"},
		map[string]interface{}{"type": "SubmarinerAgentDegraded", "status": "False"},
		map[string]interface{}{
			"type":   "SubmarinerConnectionDegraded",
			"status": "True",
			"reason": "ConnectionsNotEstablished",
		},
	})
	addon2 := addOnWithStatus("spoke2", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
		map[string]interface{}{"type": "SubmarinerGatewayNodesLabeled", "status": "True"},
		map[string]interface{}{"type": "SubmarinerAgentDegraded", "status": "False"},
		map[string]interface{}{
			"type":   "SubmarinerConnectionDegraded",
			"status": "True",
			"reason": "ConnectionsNotEstablished",
		},
	})
	cfg1 := buildSubmarinerConfig("spoke1", SubmarinerConfigOpts{})
	cfg2 := buildSubmarinerConfig("spoke2", SubmarinerConfigOpts{})
	mgr := newTestManager(c1, c2, addon1, addon2, cfg1, cfg2)

	result, err := mgr.Diagnose(context.Background(), "test-set")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, ch := range result.Checks {
		if ch.Name == "globalnet" && ch.Status == "warn" {
			found = true
		}
	}
	if !found {
		t.Error("expected globalnet recommendation when connections fail without globalnet")
	}
}

func TestDiagnoseNoGlobalnetWarningWhenGlobalnetEnabled(t *testing.T) {
	c1 := managedCluster("spoke1", "gn-set")
	c2 := managedCluster("spoke2", "gn-set")
	addon1 := addOnWithStatus("spoke1", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
		map[string]interface{}{
			"type":   "SubmarinerConnectionDegraded",
			"status": "True",
			"reason": "ConnectionsNotEstablished",
		},
	})
	addon2 := addOnWithStatus("spoke2", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
		map[string]interface{}{
			"type":   "SubmarinerConnectionDegraded",
			"status": "True",
			"reason": "ConnectionsNotEstablished",
		},
	})
	cfg1 := buildSubmarinerConfig("spoke1", SubmarinerConfigOpts{GlobalCIDR: "242.0.0.0/16"})
	cfg2 := buildSubmarinerConfig("spoke2", SubmarinerConfigOpts{GlobalCIDR: "242.1.0.0/16"})
	mgr := newTestManager(c1, c2, addon1, addon2, cfg1, cfg2)

	result, err := mgr.Diagnose(context.Background(), "gn-set")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, ch := range result.Checks {
		if ch.Name == "globalnet" {
			t.Error("should not recommend globalnet when already enabled")
		}
	}
}

func TestDiagnoseNonDefaultClusterSetNoNamingWarning(t *testing.T) {
	c1 := managedCluster("spoke1", "prod-set")
	c2 := managedCluster("spoke2", "prod-set")
	addon1 := addOnWithStatus("spoke1", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
	})
	addon2 := addOnWithStatus("spoke2", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
	})
	mgr := newTestManager(c1, c2, addon1, addon2)

	result, err := mgr.Diagnose(context.Background(), "prod-set")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, ch := range result.Checks {
		if ch.Name == "cluster-set/naming" {
			t.Error("should not warn about ClusterSet naming for non-default set")
		}
	}
}

func TestDiagnoseMissingBrokerCR(t *testing.T) {
	c1 := managedCluster("spoke1", "no-broker-set")
	c2 := managedCluster("spoke2", "no-broker-set")
	addon1 := addOnWithStatus("spoke1", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
	})
	addon2 := addOnWithStatus("spoke2", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
	})
	mgr := newTestManager(c1, c2, addon1, addon2)

	result, err := mgr.Diagnose(context.Background(), "no-broker-set")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, ch := range result.Checks {
		if ch.Name == "broker-cr" && ch.Status == "fail" {
			found = true
		}
	}
	if !found {
		t.Error("expected fail for missing Broker CR")
	}
}

func TestDiagnoseBrokerCRPresent(t *testing.T) {
	c1 := managedCluster("spoke1", "ok-set")
	c2 := managedCluster("spoke2", "ok-set")
	addon1 := addOnWithStatus("spoke1", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
	})
	addon2 := addOnWithStatus("spoke2", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
	})
	broker := testBrokerCR("ok-set")
	mgr := newTestManager(c1, c2, addon1, addon2, broker)

	result, err := mgr.Diagnose(context.Background(), "ok-set")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, ch := range result.Checks {
		if ch.Name == "broker-cr" && ch.Status == "pass" {
			found = true
		}
	}
	if !found {
		t.Error("expected pass for existing Broker CR")
	}
}

func testClusterDeploymentIBM(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": name,
			},
			"spec": map[string]interface{}{
				"platform": map[string]interface{}{
					"ibmcloud": map[string]interface{}{
						"region": "us-east",
					},
				},
			},
		},
	}
}

func TestDiagnoseIBMCloudUDPWarning(t *testing.T) {
	c1 := managedCluster("ibm1", "ibm-set")
	c2 := managedCluster("ibm2", "ibm-set")
	addon1 := addOnWithStatus("ibm1", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
	})
	addon2 := addOnWithStatus("ibm2", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
	})
	cfg1 := buildSubmarinerConfig("ibm1", SubmarinerConfigOpts{LoadBalancer: true})
	cd1 := testClusterDeploymentIBM("ibm1")
	broker := testBrokerCR("ibm-set")
	mgr := newTestManager(c1, c2, addon1, addon2, cfg1, cd1, broker)

	result, err := mgr.Diagnose(context.Background(), "ibm-set")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, ch := range result.Checks {
		if ch.Name == "config/ibm1/ibm-udp" && ch.Status == "warn" {
			found = true
		}
	}
	if !found {
		t.Error("expected IBM Cloud UDP warning for LoadBalancer-enabled config on IBM platform")
	}
}

func TestDiagnoseNoIBMWarningWithoutLB(t *testing.T) {
	c1 := managedCluster("ibm1", "ibm-set2")
	c2 := managedCluster("ibm2", "ibm-set2")
	addon1 := addOnWithStatus("ibm1", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
	})
	addon2 := addOnWithStatus("ibm2", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
	})
	cfg1 := buildSubmarinerConfig("ibm1", SubmarinerConfigOpts{})
	cd1 := testClusterDeploymentIBM("ibm1")
	broker := testBrokerCR("ibm-set2")
	mgr := newTestManager(c1, c2, addon1, addon2, cfg1, cd1, broker)

	result, err := mgr.Diagnose(context.Background(), "ibm-set2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, ch := range result.Checks {
		if ch.Name == "config/ibm1/ibm-udp" {
			t.Error("should not warn about IBM UDP when LB is not enabled")
		}
	}
}

func testSubmarinerEndpoint(clusterID, publicIP, privateIP string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "submariner.io/v1",
			"kind":       "Endpoint",
			"metadata": map[string]interface{}{
				"name":      clusterID + "-submariner-cable-" + clusterID,
				"namespace": "", // set by caller
			},
			"spec": map[string]interface{}{
				"cluster_id":  clusterID,
				"public_ip":   publicIP,
				"private_ip":  privateIP,
				"nat_enabled": true,
				"backend":     "libreswan",
				"backend_config": map[string]interface{}{
					"udp-port":            "4500",
					"natt-discovery-port": "4490",
				},
			},
		},
	}
}

func endpointInBrokerNS(clusterSet, clusterID, publicIP, privateIP string) *unstructured.Unstructured {
	ep := testSubmarinerEndpoint(clusterID, publicIP, privateIP)
	ep.Object["metadata"].(map[string]interface{})["namespace"] = clusterSet + "-broker"
	return ep
}

func TestDiagnoseEndpointsShownOnConnectionFail(t *testing.T) {
	c1 := managedCluster("spoke1", "ep-set")
	c2 := managedCluster("spoke2", "ep-set")
	addon1 := addOnWithStatus("spoke1", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
		map[string]interface{}{"type": "SubmarinerConnectionDegraded", "status": "True", "reason": "ConnectionsNotEstablished"},
	})
	addon2 := addOnWithStatus("spoke2", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
		map[string]interface{}{"type": "SubmarinerConnectionDegraded", "status": "True", "reason": "ConnectionsNotEstablished"},
	})
	cfg1 := buildSubmarinerConfig("spoke1", SubmarinerConfigOpts{})
	cfg2 := buildSubmarinerConfig("spoke2", SubmarinerConfigOpts{})
	ep1 := endpointInBrokerNS("ep-set", "spoke1", "1.1.1.1", "10.0.0.1")
	ep2 := endpointInBrokerNS("ep-set", "spoke2", "2.2.2.2", "10.0.0.2")
	broker := testBrokerCR("ep-set")
	mgr := newTestManager(c1, c2, addon1, addon2, cfg1, cfg2, ep1, ep2, broker)

	result, err := mgr.Diagnose(context.Background(), "ep-set")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	foundEP1 := false
	foundEP2 := false
	for _, ch := range result.Checks {
		if ch.Name == "endpoint/spoke1" {
			foundEP1 = true
		}
		if ch.Name == "endpoint/spoke2" {
			foundEP2 = true
		}
	}
	if !foundEP1 || !foundEP2 {
		t.Error("expected endpoint details for both clusters when connections fail")
	}
}

func TestDiagnoseEndpointsNotShownWhenHealthy(t *testing.T) {
	c1 := managedCluster("spoke1", "healthy-ep-set")
	c2 := managedCluster("spoke2", "healthy-ep-set")
	addon1 := addOnWithStatus("spoke1", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
		map[string]interface{}{"type": "SubmarinerConnectionDegraded", "status": "False"},
		map[string]interface{}{"type": "SubmarinerGatewayNodesLabeled", "status": "True"},
		map[string]interface{}{"type": "SubmarinerAgentDegraded", "status": "False"},
		map[string]interface{}{"type": "SubmarinerBrokerConfigApplied", "status": "True"},
		map[string]interface{}{"type": "ManifestApplied", "status": "True"},
		map[string]interface{}{"type": "RouteAgentConnectionDegraded", "status": "False"},
	})
	addon2 := addOnWithStatus("spoke2", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
		map[string]interface{}{"type": "SubmarinerConnectionDegraded", "status": "False"},
		map[string]interface{}{"type": "SubmarinerGatewayNodesLabeled", "status": "True"},
		map[string]interface{}{"type": "SubmarinerAgentDegraded", "status": "False"},
		map[string]interface{}{"type": "SubmarinerBrokerConfigApplied", "status": "True"},
		map[string]interface{}{"type": "ManifestApplied", "status": "True"},
		map[string]interface{}{"type": "RouteAgentConnectionDegraded", "status": "False"},
	})
	cfg1 := buildSubmarinerConfig("spoke1", SubmarinerConfigOpts{})
	cfg2 := buildSubmarinerConfig("spoke2", SubmarinerConfigOpts{})
	sec1 := testSecret("spoke1-cloud-creds", "spoke1")
	sec2 := testSecret("spoke2-cloud-creds", "spoke2")
	broker := testBrokerCR("healthy-ep-set")
	ep1 := endpointInBrokerNS("healthy-ep-set", "spoke1", "1.1.1.1", "10.0.0.1")
	mgr := newTestManager(c1, c2, addon1, addon2, cfg1, cfg2, sec1, sec2, broker, ep1)

	result, err := mgr.Diagnose(context.Background(), "healthy-ep-set")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, ch := range result.Checks {
		if strings.HasPrefix(ch.Name, "endpoint/") {
			t.Errorf("should not show endpoint details when connections are healthy, but found %s", ch.Name)
		}
	}
}

func TestDiagnoseFirewallPortsIBMCloud(t *testing.T) {
	c1 := managedCluster("ibm1", "fw-set")
	c2 := managedCluster("ibm2", "fw-set")
	addon1 := addOnWithStatus("ibm1", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
		map[string]interface{}{"type": "SubmarinerConnectionDegraded", "status": "True", "reason": "ConnectionsNotEstablished"},
	})
	addon2 := addOnWithStatus("ibm2", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
		map[string]interface{}{"type": "SubmarinerConnectionDegraded", "status": "True", "reason": "ConnectionsNotEstablished"},
	})
	cfg1 := buildSubmarinerConfig("ibm1", SubmarinerConfigOpts{})
	cfg2 := buildSubmarinerConfig("ibm2", SubmarinerConfigOpts{})
	cd1 := testClusterDeploymentIBM("ibm1")
	cd2 := testClusterDeploymentIBM("ibm2")
	broker := testBrokerCR("fw-set")
	mgr := newTestManager(c1, c2, addon1, addon2, cfg1, cfg2, cd1, cd2, broker)

	result, err := mgr.Diagnose(context.Background(), "fw-set")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := 0
	for _, ch := range result.Checks {
		if ch.Name == "firewall/ibm1" || ch.Name == "firewall/ibm2" {
			found++
			if ch.Status != "fail" {
				t.Errorf("expected fail for firewall check, got %s", ch.Status)
			}
		}
	}
	if found != 2 {
		t.Errorf("expected 2 firewall checks for IBM Cloud clusters, got %d", found)
	}
}

func TestDiagnoseNoFirewallCheckWhenHealthy(t *testing.T) {
	c1 := managedCluster("ibm1", "fw-ok-set")
	c2 := managedCluster("ibm2", "fw-ok-set")
	addon1 := addOnWithStatus("ibm1", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
		map[string]interface{}{"type": "SubmarinerConnectionDegraded", "status": "False"},
		map[string]interface{}{"type": "SubmarinerGatewayNodesLabeled", "status": "True"},
		map[string]interface{}{"type": "SubmarinerAgentDegraded", "status": "False"},
		map[string]interface{}{"type": "SubmarinerBrokerConfigApplied", "status": "True"},
		map[string]interface{}{"type": "ManifestApplied", "status": "True"},
		map[string]interface{}{"type": "RouteAgentConnectionDegraded", "status": "False"},
	})
	addon2 := addOnWithStatus("ibm2", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
		map[string]interface{}{"type": "SubmarinerConnectionDegraded", "status": "False"},
		map[string]interface{}{"type": "SubmarinerGatewayNodesLabeled", "status": "True"},
		map[string]interface{}{"type": "SubmarinerAgentDegraded", "status": "False"},
		map[string]interface{}{"type": "SubmarinerBrokerConfigApplied", "status": "True"},
		map[string]interface{}{"type": "ManifestApplied", "status": "True"},
		map[string]interface{}{"type": "RouteAgentConnectionDegraded", "status": "False"},
	})
	cfg1 := buildSubmarinerConfig("ibm1", SubmarinerConfigOpts{})
	cfg2 := buildSubmarinerConfig("ibm2", SubmarinerConfigOpts{})
	sec1 := testSecret("ibm1-cloud-creds", "ibm1")
	sec2 := testSecret("ibm2-cloud-creds", "ibm2")
	cd1 := testClusterDeploymentIBM("ibm1")
	cd2 := testClusterDeploymentIBM("ibm2")
	broker := testBrokerCR("fw-ok-set")
	mgr := newTestManager(c1, c2, addon1, addon2, cfg1, cfg2, sec1, sec2, cd1, cd2, broker)

	result, err := mgr.Diagnose(context.Background(), "fw-ok-set")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, ch := range result.Checks {
		if strings.HasPrefix(ch.Name, "firewall/") {
			t.Error("should not show firewall checks when connections are healthy")
		}
	}
}

func TestConditionIsTrue(t *testing.T) {
	conds := []condition{
		{condType: "Available", status: "True"},
		{condType: "Degraded", status: "False"},
	}
	if !conditionIsTrue(conds, "Available") {
		t.Error("expected Available=true")
	}
	if conditionIsTrue(conds, "Degraded") {
		t.Error("expected Degraded=false")
	}
	if conditionIsTrue(conds, "NonExistent") {
		t.Error("expected false for missing condition")
	}
}
