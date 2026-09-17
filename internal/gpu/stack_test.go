package gpu

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

func policyWithCompliance(cluster, compliant string, details []interface{}, conditions []interface{}) *unstructured.Unstructured {
	obj := map[string]interface{}{
		"apiVersion": "policy.open-cluster-management.io/v1",
		"kind":       "Policy",
		"metadata": map[string]interface{}{
			"name":      stackPolicyName(cluster),
			"namespace": DefaultNamespace,
		},
		"status": map[string]interface{}{
			"compliant": compliant,
		},
	}
	if details != nil {
		obj["status"].(map[string]interface{})["details"] = details
	}
	if conditions != nil {
		obj["status"].(map[string]interface{})["conditions"] = conditions
	}
	return &unstructured.Unstructured{Object: obj}
}

func TestDeployStackCreatesKueueManifestWork(t *testing.T) {
	mgr := newTestManager(managedCluster("gpu1", nil))
	if err := mgr.DeployStack(context.Background(), "gpu1", ""); err != nil {
		t.Fatalf("DeployStack failed: %v", err)
	}
	_, err := mgr.client.Get(context.Background(), client.GVRManifestWork, "gpu1", "gpu1-kueue-stack")
	if err != nil {
		t.Fatalf("Kueue ManifestWork not found: %v", err)
	}
}

func TestDeployStackCreatesKyvernoManifestWork(t *testing.T) {
	mgr := newTestManager(managedCluster("gpu1", nil))
	if err := mgr.DeployStack(context.Background(), "gpu1", ""); err != nil {
		t.Fatalf("DeployStack failed: %v", err)
	}
	_, err := mgr.client.Get(context.Background(), client.GVRManifestWork, "gpu1", "gpu1-kyverno-gpu")
	if err != nil {
		t.Fatalf("Kyverno ManifestWork not found: %v", err)
	}
}

func TestDeployStackCreatesHealthPolicy(t *testing.T) {
	mgr := newTestManager(managedCluster("gpu1", nil))
	if err := mgr.DeployStack(context.Background(), "gpu1", ""); err != nil {
		t.Fatalf("DeployStack failed: %v", err)
	}
	policyName := stackPolicyName("gpu1")
	_, err := mgr.client.Get(context.Background(), client.GVRPolicy, DefaultNamespace, policyName)
	if err != nil {
		t.Fatalf("Health policy not found: %v", err)
	}
}

func TestDeployStackCreatesPlacementAndBinding(t *testing.T) {
	mgr := newTestManager(managedCluster("gpu1", nil))
	if err := mgr.DeployStack(context.Background(), "gpu1", ""); err != nil {
		t.Fatalf("DeployStack failed: %v", err)
	}
	policyName := stackPolicyName("gpu1")
	_, err := mgr.client.Get(context.Background(), client.GVRPlacement, DefaultNamespace, policyName+"-placement")
	if err != nil {
		t.Fatalf("Placement not found: %v", err)
	}
	_, err = mgr.client.Get(context.Background(), client.GVRPlacementBinding, DefaultNamespace, policyName+"-placement-binding")
	if err != nil {
		t.Fatalf("PlacementBinding not found: %v", err)
	}
}

func TestDeployStackLabelsCluster(t *testing.T) {
	mgr := newTestManager(managedCluster("gpu1", nil))
	if err := mgr.DeployStack(context.Background(), "gpu1", ""); err != nil {
		t.Fatalf("DeployStack failed: %v", err)
	}
	mc, err := mgr.client.Get(context.Background(), client.GVRManagedCluster, "", "gpu1")
	if err != nil {
		t.Fatalf("ManagedCluster not found: %v", err)
	}
	labels, _, _ := unstructured.NestedStringMap(mc.Object, "metadata", "labels")
	if labels["gpu-sharing"] != "enabled" {
		t.Errorf("gpu-sharing label = %q, want enabled", labels["gpu-sharing"])
	}
}

func TestDeployStackWithClusterSet(t *testing.T) {
	mgr := newTestManager(managedCluster("gpu1", nil))
	if err := mgr.DeployStack(context.Background(), "gpu1", "team-gpu"); err != nil {
		t.Fatalf("DeployStack failed: %v", err)
	}
	policyName := stackPolicyName("gpu1")
	placement, err := mgr.client.Get(context.Background(), client.GVRPlacement, DefaultNamespace, policyName+"-placement")
	if err != nil {
		t.Fatalf("Placement not found: %v", err)
	}
	cs, _, _ := unstructured.NestedStringSlice(placement.Object, "spec", "clusterSets")
	if len(cs) != 1 || cs[0] != "team-gpu" {
		t.Errorf("clusterSets = %v, want [team-gpu]", cs)
	}
}

func TestDeployStackWithoutClusterSetOmitsField(t *testing.T) {
	mgr := newTestManager(managedCluster("gpu1", nil))
	if err := mgr.DeployStack(context.Background(), "gpu1", ""); err != nil {
		t.Fatalf("DeployStack failed: %v", err)
	}
	policyName := stackPolicyName("gpu1")
	placement, err := mgr.client.Get(context.Background(), client.GVRPlacement, DefaultNamespace, policyName+"-placement")
	if err != nil {
		t.Fatalf("Placement not found: %v", err)
	}
	_, found, _ := unstructured.NestedStringSlice(placement.Object, "spec", "clusterSets")
	if found {
		t.Error("clusterSets should not be set when clusterSet is empty")
	}
}

func TestRemoveStackDeletesResources(t *testing.T) {
	mgr := newTestManager(managedCluster("gpu1", nil))
	ctx := context.Background()
	if err := mgr.DeployStack(ctx, "gpu1", ""); err != nil {
		t.Fatalf("DeployStack failed: %v", err)
	}
	if err := mgr.RemoveStack(ctx, "gpu1"); err != nil {
		t.Fatalf("RemoveStack failed: %v", err)
	}

	_, err := mgr.client.Get(ctx, client.GVRManifestWork, "gpu1", "gpu1-kueue-stack")
	if err == nil {
		t.Error("Kueue ManifestWork should have been deleted")
	}
	_, err = mgr.client.Get(ctx, client.GVRManifestWork, "gpu1", "gpu1-kyverno-gpu")
	if err == nil {
		t.Error("Kyverno ManifestWork should have been deleted")
	}
	policyName := stackPolicyName("gpu1")
	_, err = mgr.client.Get(ctx, client.GVRPolicy, DefaultNamespace, policyName)
	if err == nil {
		t.Error("Health policy should have been deleted")
	}
}

func TestRemoveStackDeletesQueueManifestWork(t *testing.T) {
	mgr := newTestManager(managedCluster("gpu1", nil))
	ctx := context.Background()
	_ = mgr.DeployStack(ctx, "gpu1", "")
	_ = mgr.CreateClusterQueues(ctx, "gpu1", []string{"H100"})
	if err := mgr.RemoveStack(ctx, "gpu1"); err != nil {
		t.Fatalf("RemoveStack failed: %v", err)
	}
	_, err := mgr.client.Get(ctx, client.GVRManifestWork, "gpu1", "gpu1-gpu-queues")
	if err == nil {
		t.Error("Queue ManifestWork should have been deleted")
	}
}

func TestDetectDriftReturnsCompliant(t *testing.T) {
	policy := policyWithCompliance("gpu1", "Compliant", nil, nil)
	mgr := newTestManager(policy)
	status, err := mgr.DetectDrift(context.Background(), "gpu1")
	if err != nil {
		t.Fatalf("DetectDrift failed: %v", err)
	}
	if !status.Compliant {
		t.Error("expected Compliant=true")
	}
	if len(status.Degraded) != 0 {
		t.Errorf("expected no degraded components, got %v", status.Degraded)
	}
}

func TestDetectDriftDetectsDegraded(t *testing.T) {
	details := []interface{}{
		map[string]interface{}{
			"compliant":    "NonCompliant",
			"templateMeta": map[string]interface{}{"name": "kueue-health"},
		},
		map[string]interface{}{
			"compliant":    "Compliant",
			"templateMeta": map[string]interface{}{"name": "kyverno-health"},
		},
	}
	policy := policyWithCompliance("gpu1", "NonCompliant", details, nil)
	mgr := newTestManager(policy)
	status, err := mgr.DetectDrift(context.Background(), "gpu1")
	if err != nil {
		t.Fatalf("DetectDrift failed: %v", err)
	}
	if status.Compliant {
		t.Error("expected Compliant=false")
	}
	if len(status.Degraded) != 1 || status.Degraded[0] != "kueue-health" {
		t.Errorf("Degraded = %v, want [kueue-health]", status.Degraded)
	}
}

func TestDetectDriftReturnsConditions(t *testing.T) {
	conditions := []interface{}{
		map[string]interface{}{
			"type":    "PolicyViolation",
			"message": "kueue-controller-manager not ready",
		},
	}
	policy := policyWithCompliance("gpu1", "NonCompliant", nil, conditions)
	mgr := newTestManager(policy)
	status, err := mgr.DetectDrift(context.Background(), "gpu1")
	if err != nil {
		t.Fatalf("DetectDrift failed: %v", err)
	}
	if len(status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(status.Conditions))
	}
	if status.Conditions[0] != "kueue-controller-manager not ready" {
		t.Errorf("condition = %q, want 'kueue-controller-manager not ready'", status.Conditions[0])
	}
}

func TestDetectDriftPolicyNotFound(t *testing.T) {
	mgr := newTestManager()
	_, err := mgr.DetectDrift(context.Background(), "gpu1")
	if err == nil {
		t.Fatal("expected error for missing policy")
	}
}

func TestCreateClusterQueuesCreatesPerType(t *testing.T) {
	mgr := newTestManager()
	gpuTypes := []string{"H100", "L4"}
	if err := mgr.CreateClusterQueues(context.Background(), "gpu1", gpuTypes); err != nil {
		t.Fatalf("CreateClusterQueues failed: %v", err)
	}
	mw, err := mgr.client.Get(context.Background(), client.GVRManifestWork, "gpu1", "gpu1-gpu-queues")
	if err != nil {
		t.Fatalf("Queue ManifestWork not found: %v", err)
	}
	manifests, _, _ := unstructured.NestedSlice(mw.Object, "spec", "workload", "manifests")
	if len(manifests) != 4 {
		t.Errorf("expected 4 manifests (2 ResourceFlavors + 2 ClusterQueues), got %d", len(manifests))
	}
}

func TestCreateClusterQueuesRejectsEmpty(t *testing.T) {
	mgr := newTestManager()
	err := mgr.CreateClusterQueues(context.Background(), "gpu1", nil)
	if err == nil {
		t.Fatal("expected error for empty gpuTypes")
	}
}

func TestCreateClusterQueuesSingleType(t *testing.T) {
	mgr := newTestManager()
	if err := mgr.CreateClusterQueues(context.Background(), "gpu1", []string{"A100"}); err != nil {
		t.Fatalf("CreateClusterQueues failed: %v", err)
	}
	mw, err := mgr.client.Get(context.Background(), client.GVRManifestWork, "gpu1", "gpu1-gpu-queues")
	if err != nil {
		t.Fatalf("Queue ManifestWork not found: %v", err)
	}
	manifests, _, _ := unstructured.NestedSlice(mw.Object, "spec", "workload", "manifests")
	if len(manifests) != 2 {
		t.Errorf("expected 2 manifests (1 ResourceFlavor + 1 ClusterQueue), got %d", len(manifests))
	}
}

func TestBuildKueueManifestWorkNameAndNamespace(t *testing.T) {
	mw := buildKueueManifestWork("gpu-cluster1")
	if mw.GetName() != "gpu-cluster1-kueue-stack" {
		t.Errorf("name = %q, want gpu-cluster1-kueue-stack", mw.GetName())
	}
	if mw.GetNamespace() != "gpu-cluster1" {
		t.Errorf("namespace = %q, want gpu-cluster1", mw.GetNamespace())
	}
}

func TestBuildKyvernoManifestWorkNameAndNamespace(t *testing.T) {
	mw := buildKyvernoManifestWork("gpu-cluster1")
	if mw.GetName() != "gpu-cluster1-kyverno-gpu" {
		t.Errorf("name = %q, want gpu-cluster1-kyverno-gpu", mw.GetName())
	}
	if mw.GetNamespace() != "gpu-cluster1" {
		t.Errorf("namespace = %q, want gpu-cluster1", mw.GetNamespace())
	}
}

func TestBuildStackHealthPolicyIncludesBothTemplates(t *testing.T) {
	policy := buildStackHealthPolicy("gpu1")
	templates, _, _ := unstructured.NestedSlice(policy.Object, "spec", "policy-templates")
	if len(templates) != 2 {
		t.Errorf("expected 2 policy templates (kueue + kyverno), got %d", len(templates))
	}
}

func TestBuildClusterQueueManifestWorkGPULabels(t *testing.T) {
	mw := buildClusterQueueManifestWork("gpu1", []string{"H100"})
	manifests, _, _ := unstructured.NestedSlice(mw.Object, "spec", "workload", "manifests")
	if len(manifests) < 1 {
		t.Fatal("expected at least 1 manifest")
	}
	flavor := manifests[0].(map[string]interface{})
	nodeLabels, _, _ := unstructured.NestedStringMap(flavor, "spec", "nodeLabels")
	if nodeLabels["nvidia.com/gpu.product"] != "H100" {
		t.Errorf("nodeLabel gpu.product = %q, want H100", nodeLabels["nvidia.com/gpu.product"])
	}
}
