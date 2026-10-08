package submariner

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

func testManagedClusterSet(name string, finalizers []string) *unstructured.Unstructured {
	fin := make([]interface{}, len(finalizers))
	for i, f := range finalizers {
		fin[i] = f
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta2",
			"kind":       "ManagedClusterSet",
			"metadata": map[string]interface{}{
				"name":       name,
				"finalizers": fin,
			},
		},
	}
}

func stuckAddon(cluster string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "addon.open-cluster-management.io/v1alpha1",
			"kind":       "ManagedClusterAddOn",
			"metadata": map[string]interface{}{
				"name":              "submariner",
				"namespace":         cluster,
				"deletionTimestamp": "2026-10-07T12:00:00Z",
				"finalizers": []interface{}{
					"addon.open-cluster-management.io/cleanup",
				},
			},
		},
	}
}

func TestRepairStuckAddons(t *testing.T) {
	c1 := managedCluster("spoke1", "repair-set")
	c2 := managedCluster("spoke2", "repair-set")
	stuck1 := stuckAddon("spoke1")
	mgr := newTestManager(c1, c2, stuck1)

	result, err := mgr.Repair(context.Background(), "repair-set", false)
	if err != nil {
		t.Fatalf("Repair: %v", err)
	}

	found := false
	for _, a := range result.Actions {
		if a.Name == "stuck-addon/spoke1" && a.Status == "fixed" {
			found = true
		}
	}
	if !found {
		t.Error("expected stuck-addon/spoke1 to be fixed")
	}
}

func TestRepairStuckAddonsDryRun(t *testing.T) {
	c1 := managedCluster("spoke1", "repair-set")
	stuck := stuckAddon("spoke1")
	mgr := newTestManager(c1, stuck)

	result, err := mgr.Repair(context.Background(), "repair-set", true)
	if err != nil {
		t.Fatalf("Repair: %v", err)
	}

	found := false
	for _, a := range result.Actions {
		if a.Name == "stuck-addon/spoke1" && a.Status == "would-fix" {
			found = true
		}
	}
	if !found {
		t.Error("expected dry-run would-fix for stuck addon")
	}

	addon, err := mgr.client.Get(context.Background(), client.GVRManagedClusterAddOn, "spoke1", "submariner")
	if err != nil {
		t.Fatalf("addon should still exist after dry-run: %v", err)
	}
	fins := addon.GetFinalizers()
	if len(fins) == 0 {
		t.Error("finalizers should not be removed in dry-run")
	}
}

func TestRepairMissingBrokerCR(t *testing.T) {
	c1 := managedCluster("spoke1", "repair-set")
	c2 := managedCluster("spoke2", "repair-set")
	mgr := newTestManager(c1, c2)

	result, err := mgr.Repair(context.Background(), "repair-set", false)
	if err != nil {
		t.Fatalf("Repair: %v", err)
	}

	found := false
	for _, a := range result.Actions {
		if a.Name == "broker-cr" && a.Status == "fixed" {
			found = true
		}
	}
	if !found {
		t.Error("expected broker-cr to be fixed")
	}

	_, err = mgr.client.Get(context.Background(), client.GVRSubmarinerBroker, "repair-set-broker", "submariner-broker")
	if err != nil {
		t.Fatalf("Broker CR should exist after repair: %v", err)
	}
}

func TestRepairBrokerCRAlreadyExists(t *testing.T) {
	c1 := managedCluster("spoke1", "ok-set")
	c2 := managedCluster("spoke2", "ok-set")
	broker := testBrokerCR("ok-set")
	mgr := newTestManager(c1, c2, broker)

	result, err := mgr.Repair(context.Background(), "ok-set", false)
	if err != nil {
		t.Fatalf("Repair: %v", err)
	}

	found := false
	for _, a := range result.Actions {
		if a.Name == "broker-cr" && a.Status == "ok" {
			found = true
		}
	}
	if !found {
		t.Error("expected broker-cr status=ok when already exists")
	}
}

func TestRepairDuplicateFinalizers(t *testing.T) {
	c1 := managedCluster("spoke1", "dup-set")
	c2 := managedCluster("spoke2", "dup-set")
	broker := testBrokerCR("dup-set")
	cs := testManagedClusterSet("dup-set", []string{
		"cluster.open-cluster-management.io/submariner-cleanup",
		"cluster.open-cluster-management.io/submariner-cleanup",
		"cluster.open-cluster-management.io/managedclusterset",
	})
	mgr := newTestManager(c1, c2, broker, cs)

	result, err := mgr.Repair(context.Background(), "dup-set", false)
	if err != nil {
		t.Fatalf("Repair: %v", err)
	}

	found := false
	for _, a := range result.Actions {
		if a.Name == "duplicate-finalizers/dup-set" && a.Status == "fixed" {
			found = true
		}
	}
	if !found {
		t.Error("expected duplicate finalizers to be fixed")
	}
}

func TestRepairNoDuplicateFinalizers(t *testing.T) {
	c1 := managedCluster("spoke1", "clean-set")
	c2 := managedCluster("spoke2", "clean-set")
	broker := testBrokerCR("clean-set")
	cs := testManagedClusterSet("clean-set", []string{
		"cluster.open-cluster-management.io/managedclusterset",
	})
	mgr := newTestManager(c1, c2, broker, cs)

	result, err := mgr.Repair(context.Background(), "clean-set", false)
	if err != nil {
		t.Fatalf("Repair: %v", err)
	}

	for _, a := range result.Actions {
		if a.Name == "duplicate-finalizers/clean-set" {
			t.Errorf("should not report duplicate finalizers when there are none, got: %+v", a)
		}
	}
}

func TestRepairNoIssues(t *testing.T) {
	c1 := managedCluster("spoke1", "healthy-set")
	c2 := managedCluster("spoke2", "healthy-set")
	broker := testBrokerCR("healthy-set")
	mgr := newTestManager(c1, c2, broker)

	result, err := mgr.Repair(context.Background(), "healthy-set", false)
	if err != nil {
		t.Fatalf("Repair: %v", err)
	}

	for _, a := range result.Actions {
		if a.Status != "ok" {
			t.Errorf("expected all ok, got %s for %s", a.Status, a.Name)
		}
	}
}
