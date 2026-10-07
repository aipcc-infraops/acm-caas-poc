package submariner

import (
	"context"
	"testing"
	"time"
)

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

func TestDefaultImages(t *testing.T) {
	if DefaultServerImage != "registry.redhat.io/ubi9/ubi:latest" {
		t.Errorf("expected registry.redhat.io/ubi9/ubi:latest, got %s", DefaultServerImage)
	}
	if DefaultClientImage != "registry.redhat.io/ubi9/ubi:latest" {
		t.Errorf("expected registry.redhat.io/ubi9/ubi:latest, got %s", DefaultClientImage)
	}
}

func TestBuildTestNamespace(t *testing.T) {
	ns := buildTestNamespace("acmlab-submariner-test")
	name := ns.GetName()
	if name != "acmlab-submariner-test" {
		t.Errorf("expected name=acmlab-submariner-test, got %s", name)
	}
	labels := ns.GetLabels()
	if labels["acmlab.redhat.com/managed"] != "true" {
		t.Error("expected managed label")
	}
}

func TestBuildTestService(t *testing.T) {
	svc := buildTestService("test-ns")
	if svc.GetName() != testSvcName {
		t.Errorf("expected name=%s, got %s", testSvcName, svc.GetName())
	}
}

func TestBuildServiceExport(t *testing.T) {
	exp := buildServiceExport("test-ns")
	if exp.GetName() != testSvcName {
		t.Errorf("expected name=%s, got %s", testSvcName, exp.GetName())
	}
	apiVersion := exp.GetAPIVersion()
	if apiVersion != "multicluster.x-k8s.io/v1alpha1" {
		t.Errorf("expected apiVersion=multicluster.x-k8s.io/v1alpha1, got %s", apiVersion)
	}
	if exp.GetKind() != "ServiceExport" {
		t.Errorf("expected kind=ServiceExport, got %s", exp.GetKind())
	}
}

func TestBuildTestServerPodUsesPython(t *testing.T) {
	pod := buildTestServerPod("test-ns", DefaultServerImage)
	if pod.GetName() != testServerName {
		t.Errorf("expected name=%s, got %s", testServerName, pod.GetName())
	}
	containers, _, _ := nestedSlice(pod.Object, "spec", "containers")
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	c := containers[0].(map[string]interface{})
	cmd, _ := c["command"].([]interface{})
	if len(cmd) < 1 || cmd[0] != "python3" {
		t.Error("expected server pod to use python3 command")
	}
	sc, ok := c["securityContext"].(map[string]interface{})
	if !ok {
		t.Fatal("expected securityContext")
	}
	if sc["runAsNonRoot"] != true {
		t.Error("expected runAsNonRoot=true")
	}
	if sc["allowPrivilegeEscalation"] != false {
		t.Error("expected allowPrivilegeEscalation=false")
	}
}

func TestBuildTestClientPodUsesCurl(t *testing.T) {
	pod := buildTestClientPod("test-ns", DefaultClientImage)
	if pod.GetName() != testClientName {
		t.Errorf("expected name=%s, got %s", testClientName, pod.GetName())
	}
	containers, _, _ := nestedSlice(pod.Object, "spec", "containers")
	c := containers[0].(map[string]interface{})
	cmd, _ := c["command"].([]interface{})
	if len(cmd) < 3 {
		t.Fatal("expected command with at least 3 elements")
	}
	script := cmd[2].(string)
	if len(script) == 0 {
		t.Error("expected non-empty script")
	}
	sc, ok := c["securityContext"].(map[string]interface{})
	if !ok {
		t.Fatal("expected securityContext")
	}
	if sc["runAsNonRoot"] != true {
		t.Error("expected runAsNonRoot=true")
	}
}

func TestPodPhaseSucceeded(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{"phase": "Succeeded"},
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

func TestPodStatusReasonFromContainerStatus(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{
			"containerStatuses": []interface{}{
				map[string]interface{}{
					"waiting": map[string]interface{}{
						"reason": "ImagePullBackOff",
					},
				},
			},
		},
	}
	if got := podStatusReason(obj); got != "ImagePullBackOff" {
		t.Errorf("expected ImagePullBackOff, got %s", got)
	}
}

func TestPodStatusReasonFromTerminated(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{
			"containerStatuses": []interface{}{
				map[string]interface{}{
					"terminated": map[string]interface{}{
						"reason": "Error",
					},
				},
			},
		},
	}
	if got := podStatusReason(obj); got != "Error" {
		t.Errorf("expected Error, got %s", got)
	}
}

func TestPodStatusReasonEmpty(t *testing.T) {
	if got := podStatusReason(map[string]interface{}{}); got != "" {
		t.Errorf("expected empty, got %s", got)
	}
}

func TestPodStatusReasonFallbackToStatus(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{
			"reason": "Evicted",
		},
	}
	if got := podStatusReason(obj); got != "Evicted" {
		t.Errorf("expected Evicted, got %s", got)
	}
}

func TestIsImagePullFailure(t *testing.T) {
	tests := []struct {
		reason string
		want   bool
	}{
		{"ImagePullBackOff", true},
		{"ErrImagePull", true},
		{"SignatureValidationFailed", true},
		{"CrashLoopBackOff", false},
		{"Error", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isImagePullFailure(tt.reason); got != tt.want {
			t.Errorf("isImagePullFailure(%q) = %v, want %v", tt.reason, got, tt.want)
		}
	}
}

func TestRestrictedSecurityContext(t *testing.T) {
	if restrictedSecurityContext["runAsNonRoot"] != true {
		t.Error("expected runAsNonRoot=true")
	}
	if restrictedSecurityContext["allowPrivilegeEscalation"] != false {
		t.Error("expected allowPrivilegeEscalation=false")
	}
	caps, ok := restrictedSecurityContext["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatal("expected capabilities")
	}
	drop, ok := caps["drop"].([]interface{})
	if !ok || len(drop) != 1 || drop[0] != "ALL" {
		t.Error("expected drop=[ALL]")
	}
	profile, ok := restrictedSecurityContext["seccompProfile"].(map[string]interface{})
	if !ok || profile["type"] != "RuntimeDefault" {
		t.Error("expected seccompProfile.type=RuntimeDefault")
	}
}

func nestedSlice(obj map[string]interface{}, fields ...string) ([]interface{}, bool, error) {
	current := obj
	for i, f := range fields {
		if i == len(fields)-1 {
			v, ok := current[f].([]interface{})
			return v, ok, nil
		}
		next, ok := current[f].(map[string]interface{})
		if !ok {
			return nil, false, nil
		}
		current = next
	}
	return nil, false, nil
}
