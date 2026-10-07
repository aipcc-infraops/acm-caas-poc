package submariner

import (
	"context"
	"testing"
)

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
	cfg := buildSubmarinerConfig("spoke1")
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
	cfg1 := buildSubmarinerConfig("spoke1")
	cfg2 := buildSubmarinerConfig("spoke2")
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
	cfg1 := buildSubmarinerConfig("spoke1")
	cfg2 := buildSubmarinerConfig("spoke2")
	mgr := newTestManager(c1, c2, addon1, addon2, cfg1, cfg2)

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
