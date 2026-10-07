package submariner

import (
	"context"
	"testing"
	"time"
)

func TestTestConnectivityNoAddOn(t *testing.T) {
	c1 := managedCluster("spoke1", "test-set")
	c2 := managedCluster("spoke2", "test-set")
	mgr := newTestManager(c1, c2)

	result, err := mgr.TestConnectivity(context.Background(), ConnectivityTestOpts{
		ClusterA:  "spoke1",
		ClusterB:  "spoke2",
		Namespace: "test-ns",
		Timeout:   10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Phase != "PreflightFailed" {
		t.Errorf("expected PreflightFailed, got %s", result.Phase)
	}
}

func TestTestConnectivityClusterNotFound(t *testing.T) {
	mgr := newTestManager()

	result, err := mgr.TestConnectivity(context.Background(), ConnectivityTestOpts{
		ClusterA:  "nonexistent-a",
		ClusterB:  "nonexistent-b",
		Namespace: "test-ns",
		Timeout:   10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Phase != "PreflightFailed" {
		t.Errorf("expected PreflightFailed, got %s", result.Phase)
	}
}

func TestTestConnectivityNoAddOnButClustersExist(t *testing.T) {
	c1 := managedCluster("spoke1", "test-set")
	c2 := managedCluster("spoke2", "test-set")
	mgr := newTestManager(c1, c2)

	result, err := mgr.TestConnectivity(context.Background(), ConnectivityTestOpts{
		ClusterA:  "spoke1",
		ClusterB:  "spoke2",
		Namespace: "test-ns",
		Timeout:   10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Phase != "PreflightFailed" {
		t.Errorf("expected PreflightFailed, got %s", result.Phase)
	}
}

func TestTestConnectivitySpokeAccessRequired(t *testing.T) {
	c1 := managedCluster("spoke1", "test-set")
	c2 := managedCluster("spoke2", "test-set")
	addon1 := addOnWithStatus("spoke1", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
	})
	addon2 := addOnWithStatus("spoke2", []interface{}{
		map[string]interface{}{"type": "Available", "status": "True"},
	})
	mgr := newTestManager(c1, c2, addon1, addon2)

	result, err := mgr.TestConnectivity(context.Background(), ConnectivityTestOpts{
		ClusterA:  "spoke1",
		ClusterB:  "spoke2",
		Namespace: "test-ns",
		Timeout:   10 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Phase != "SpokeAccessRequired" {
		t.Errorf("expected SpokeAccessRequired, got %s", result.Phase)
	}
	if result.Success {
		t.Error("expected success=false without kubeconfigs")
	}
}

func TestBuildTestNamespace(t *testing.T) {
	ns := buildTestNamespace("acmlab-submariner-test")
	name, _, _ := nestedString(ns.Object, "metadata", "name")
	if name != "acmlab-submariner-test" {
		t.Errorf("expected name=acmlab-submariner-test, got %s", name)
	}
	labels, _, _ := nestedMap(ns.Object, "metadata", "labels")
	if labels["acmlab.redhat.com/managed"] != "true" {
		t.Error("expected managed label")
	}
}

func TestBuildTestService(t *testing.T) {
	svc := buildTestService("test-ns")
	name, _, _ := nestedString(svc.Object, "metadata", "name")
	if name != "submariner-test-svc" {
		t.Errorf("expected name=submariner-test-svc, got %s", name)
	}
}

func TestBuildTestServerPod(t *testing.T) {
	pod := buildTestServerPod("test-ns", "ubi9/ubi-minimal")
	name, _, _ := nestedString(pod.Object, "metadata", "name")
	if name != "submariner-test-server" {
		t.Errorf("expected name=submariner-test-server, got %s", name)
	}
}

func TestBuildTestClientPod(t *testing.T) {
	pod := buildTestClientPod("test-ns", "ubi9/ubi-minimal", "spoke2")
	name, _, _ := nestedString(pod.Object, "metadata", "name")
	if name != "submariner-test-client" {
		t.Errorf("expected name=submariner-test-client, got %s", name)
	}
}

func TestPodPhaseSucceeded(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{
			"phase": "Succeeded",
		},
	}
	if got := podPhase(obj); got != "Succeeded" {
		t.Errorf("expected Succeeded, got %s", got)
	}
}

func TestPodPhaseEmpty(t *testing.T) {
	if got := podPhase(map[string]interface{}{}); got != "" {
		t.Errorf("expected empty, got %s", got)
	}
}

func nestedString(obj map[string]interface{}, fields ...string) (string, bool, error) {
	current := obj
	for i, f := range fields {
		if i == len(fields)-1 {
			v, ok := current[f].(string)
			return v, ok, nil
		}
		next, ok := current[f].(map[string]interface{})
		if !ok {
			return "", false, nil
		}
		current = next
	}
	return "", false, nil
}

func nestedMap(obj map[string]interface{}, fields ...string) (map[string]interface{}, bool, error) {
	current := obj
	for _, f := range fields {
		next, ok := current[f].(map[string]interface{})
		if !ok {
			return nil, false, nil
		}
		current = next
	}
	return current, true, nil
}
