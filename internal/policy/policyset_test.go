package policy

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

var testLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func fakePolicySetClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRPolicySet:        "PolicySetList",
			client.GVRPlacement:        "PlacementList",
			client.GVRPlacementBinding: "PlacementBindingList",
		}, objs...)
	return &client.Client{Dynamic: fake}
}

func policySetObj(name, namespace string, policies []string) *unstructured.Unstructured {
	policyList := make([]interface{}, len(policies))
	for i, p := range policies {
		policyList[i] = p
	}
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "policy.open-cluster-management.io", Version: "v1beta1", Kind: "PolicySet",
	})
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.Object["spec"] = map[string]interface{}{
		"description": "test policy set",
		"policies":    policyList,
	}
	return obj
}

func TestApplyPolicySetCreatesResources(t *testing.T) {
	c := fakePolicySetClient()
	mgr := New(c, config.Config{}, testLogger)

	err := mgr.ApplyPolicySet(context.Background(), PolicySetOpts{
		Name:        "cis-baseline",
		Description: "CIS Level 1 compliance profile",
		Policies:    []string{"require-labels", "no-privileged"},
	})
	if err != nil {
		t.Fatalf("ApplyPolicySet failed: %v", err)
	}
}

func TestApplyPolicySetWithClusterSet(t *testing.T) {
	c := fakePolicySetClient()
	mgr := New(c, config.Config{}, testLogger)

	err := mgr.ApplyPolicySet(context.Background(), PolicySetOpts{
		Name:        "gpu-compliance",
		Description: "GPU cluster compliance",
		Policies:    []string{"gpu-limits"},
		ClusterSet:  "gpu-clusters",
	})
	if err != nil {
		t.Fatalf("ApplyPolicySet failed: %v", err)
	}
}

func TestGetPolicySetReturnsPolicies(t *testing.T) {
	ps := policySetObj("test-set", DefaultNamespace, []string{"policy-a", "policy-b"})
	c := fakePolicySetClient(ps)
	mgr := New(c, config.Config{}, testLogger)

	info, err := mgr.GetPolicySet(context.Background(), "test-set", "")
	if err != nil {
		t.Fatalf("GetPolicySet failed: %v", err)
	}
	if info.Name != "test-set" {
		t.Errorf("Name = %q, want %q", info.Name, "test-set")
	}
	if len(info.Policies) != 2 {
		t.Errorf("got %d policies, want 2", len(info.Policies))
	}
	if info.Description != "test policy set" {
		t.Errorf("Description = %q, want %q", info.Description, "test policy set")
	}
}

func TestGetPolicySetReturnsErrorForMissing(t *testing.T) {
	c := fakePolicySetClient()
	mgr := New(c, config.Config{}, testLogger)

	_, err := mgr.GetPolicySet(context.Background(), "nonexistent", "")
	if err == nil {
		t.Error("expected error for missing PolicySet")
	}
}

func TestListPolicySets(t *testing.T) {
	ps1 := policySetObj("set-1", DefaultNamespace, []string{"a"})
	ps2 := policySetObj("set-2", DefaultNamespace, []string{"b", "c"})
	c := fakePolicySetClient(ps1, ps2)
	mgr := New(c, config.Config{}, testLogger)

	sets, err := mgr.ListPolicySets(context.Background(), "")
	if err != nil {
		t.Fatalf("ListPolicySets failed: %v", err)
	}
	if len(sets) != 2 {
		t.Errorf("got %d sets, want 2", len(sets))
	}
}

func TestRemovePolicySet(t *testing.T) {
	ps := policySetObj("to-remove", DefaultNamespace, []string{"a"})
	c := fakePolicySetClient(ps)
	mgr := New(c, config.Config{}, testLogger)

	removed, err := mgr.RemovePolicySet(context.Background(), "to-remove", "")
	if err != nil {
		t.Fatalf("RemovePolicySet failed: %v", err)
	}
	if !removed {
		t.Error("expected removed = true")
	}
}

func TestRemovePolicySetNotFound(t *testing.T) {
	c := fakePolicySetClient()
	mgr := New(c, config.Config{}, testLogger)

	removed, err := mgr.RemovePolicySet(context.Background(), "nonexistent", "")
	if err != nil {
		t.Fatalf("RemovePolicySet failed: %v", err)
	}
	if removed {
		t.Error("expected removed = false for nonexistent")
	}
}

func TestBuildPolicySet(t *testing.T) {
	obj := buildPolicySet("test", "ns", "desc", []string{"p1", "p2"})
	if obj.GetName() != "test" {
		t.Errorf("Name = %q, want %q", obj.GetName(), "test")
	}
	spec, _ := obj.Object["spec"].(map[string]interface{})
	policies, _ := spec["policies"].([]interface{})
	if len(policies) != 2 {
		t.Errorf("got %d policies, want 2", len(policies))
	}
}

func TestBuildPolicySetPlacement(t *testing.T) {
	obj := buildPolicySetPlacement("test", "ns", "gpu-set")
	if obj.GetName() != "test-policyset-placement" {
		t.Errorf("Name = %q, want %q", obj.GetName(), "test-policyset-placement")
	}
	spec, _ := obj.Object["spec"].(map[string]interface{})
	sets, _ := spec["clusterSets"].([]interface{})
	if len(sets) != 1 {
		t.Errorf("got %d clusterSets, want 1", len(sets))
	}
}

func TestBuildPolicySetBinding(t *testing.T) {
	obj := buildPolicySetBinding("test", "ns")
	if obj.GetName() != "test-policyset-binding" {
		t.Errorf("Name = %q, want %q", obj.GetName(), "test-policyset-binding")
	}
}
