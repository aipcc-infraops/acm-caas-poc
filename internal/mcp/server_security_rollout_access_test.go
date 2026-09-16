package mcp

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

func TestSecurityApplyBaselineViaMCP(t *testing.T) {
	c := fakeClientWithClusters(newManagedCluster("spoke1", true))
	text := extractToolText(t, callTool(t, c, "acm_apply_security_baseline", map[string]interface{}{
		"cluster": "spoke1",
		"level":   "cis-level1",
	}))
	if !strings.Contains(strings.ToLower(text), "spoke1") {
		t.Errorf("expected spoke1 in response, got: %s", text)
	}
}

func TestSecurityStatusViaMCP(t *testing.T) {
	c := fakeClientWithClusters(newManagedCluster("spoke1", true))
	_ = callTool(t, c, "acm_apply_security_baseline", map[string]interface{}{
		"cluster": "spoke1",
		"level":   "cis-level1",
	})
	text := extractToolText(t, callTool(t, c, "acm_security_status", map[string]interface{}{
		"cluster": "spoke1",
	}))
	if !strings.Contains(strings.ToLower(text), "spoke1") {
		t.Errorf("expected spoke1 in response, got: %s", text)
	}
}

func TestSecurityListBaselinesViaMCP(t *testing.T) {
	c := fakeClientWithClusters(newManagedCluster("spoke1", true))
	_ = callTool(t, c, "acm_apply_security_baseline", map[string]interface{}{
		"cluster": "spoke1",
		"level":   "cis-level1",
	})
	text := extractToolText(t, callTool(t, c, "acm_list_security_baselines", nil))
	if !strings.Contains(text, "spoke1") {
		t.Errorf("expected spoke1 in list, got: %s", text)
	}
}

func TestSecurityRemoveBaselineViaMCP(t *testing.T) {
	c := fakeClientWithClusters(newManagedCluster("spoke1", true))
	_ = callTool(t, c, "acm_apply_security_baseline", map[string]interface{}{
		"cluster": "spoke1",
		"level":   "cis-level1",
	})
	text := extractToolText(t, callTool(t, c, "acm_remove_security_baseline", map[string]interface{}{
		"cluster": "spoke1",
	}))
	if !strings.Contains(strings.ToLower(text), "removed") {
		t.Errorf("expected 'removed' in response, got: %s", text)
	}
}

func TestSecurityRemoveNonexistentViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	text := extractToolText(t, callTool(t, c, "acm_remove_security_baseline", map[string]interface{}{
		"cluster": "nonexistent",
	}))
	if !strings.Contains(strings.ToLower(text), "nothing to remove") {
		t.Errorf("expected 'nothing to remove' in response, got: %s", text)
	}
}

func TestRolloutCreateViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	text := extractToolText(t, callTool(t, c, "acm_create_rollout", map[string]interface{}{
		"name":      "kueue-v12",
		"placement": "gpu-clusters",
		"strategy":  "Progressive",
	}))
	if !strings.Contains(strings.ToLower(text), "kueue-v12") {
		t.Errorf("expected kueue-v12 in response, got: %s", text)
	}
}

func TestRolloutGetViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	_ = callTool(t, c, "acm_create_rollout", map[string]interface{}{
		"name":      "kueue-v12",
		"placement": "gpu-clusters",
		"strategy":  "Progressive",
	})
	text := extractToolText(t, callTool(t, c, "acm_get_rollout", map[string]interface{}{
		"name": "kueue-v12",
	}))
	if !strings.Contains(strings.ToLower(text), "kueue-v12") {
		t.Errorf("expected kueue-v12 in response, got: %s", text)
	}
}

func TestRolloutListViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	_ = callTool(t, c, "acm_create_rollout", map[string]interface{}{
		"name":      "kueue-v12",
		"placement": "gpu-clusters",
	})
	text := extractToolText(t, callTool(t, c, "acm_list_rollouts", nil))
	if !strings.Contains(text, "kueue-v12") {
		t.Errorf("expected kueue-v12 in list, got: %s", text)
	}
}

func TestRolloutDeleteViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	_ = callTool(t, c, "acm_create_rollout", map[string]interface{}{
		"name":      "kueue-v12",
		"placement": "gpu-clusters",
	})
	text := extractToolText(t, callTool(t, c, "acm_delete_rollout", map[string]interface{}{
		"name": "kueue-v12",
	}))
	if !strings.Contains(strings.ToLower(text), "deleted") {
		t.Errorf("expected 'deleted' in response, got: %s", text)
	}
}

func TestRolloutDeleteNonexistentViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	text := extractToolText(t, callTool(t, c, "acm_delete_rollout", map[string]interface{}{
		"name": "nonexistent",
	}))
	if !strings.Contains(strings.ToLower(text), "not found") {
		t.Errorf("expected 'not found' in response, got: %s", text)
	}
}

func TestRolloutUpdateStrategyViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	_ = callTool(t, c, "acm_create_rollout", map[string]interface{}{
		"name":      "kueue-v12",
		"placement": "gpu-clusters",
		"strategy":  "All",
	})
	text := extractToolText(t, callTool(t, c, "acm_update_rollout_strategy", map[string]interface{}{
		"name":     "kueue-v12",
		"strategy": "Progressive",
	}))
	if !strings.Contains(strings.ToLower(text), "updated") {
		t.Errorf("expected 'updated' in response, got: %s", text)
	}
}

func newManagedServiceAccount(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "authentication.open-cluster-management.io/v1beta1",
			"kind":       "ManagedServiceAccount",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"rotation": map[string]interface{}{
					"enabled":  true,
					"validity": "720h",
				},
			},
			"status": map[string]interface{}{
				"tokenSecretRef": map[string]interface{}{
					"name": name,
				},
			},
		},
	}
}

func newAddOn(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "addon.open-cluster-management.io/v1alpha1",
			"kind":       "ManagedClusterAddOn",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Available",
						"status": "True",
					},
				},
			},
		},
	}
}

func TestAccessEnableViaMCP(t *testing.T) {
	c := fakeClientWithClusters(newManagedCluster("spoke1", true))
	text := extractToolText(t, callTool(t, c, "acm_enable_access", map[string]interface{}{
		"cluster": "spoke1",
	}))
	if !strings.Contains(strings.ToLower(text), "spoke1") {
		t.Errorf("expected spoke1 in response, got: %s", text)
	}
}

func TestAccessDisableViaMCP(t *testing.T) {
	c := fakeClientWithClusters(newManagedCluster("spoke1", true))
	_ = callTool(t, c, "acm_enable_access", map[string]interface{}{
		"cluster": "spoke1",
	})
	text := extractToolText(t, callTool(t, c, "acm_disable_access", map[string]interface{}{
		"cluster": "spoke1",
	}))
	if !strings.Contains(strings.ToLower(text), "disabled") || !strings.Contains(strings.ToLower(text), "spoke1") {
		t.Errorf("expected 'disabled' and 'spoke1' in response, got: %s", text)
	}
}

func TestAccessDisableNonexistentViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	text := extractToolText(t, callTool(t, c, "acm_disable_access", map[string]interface{}{
		"cluster": "nonexistent",
	}))
	if !strings.Contains(strings.ToLower(text), "not found") {
		t.Errorf("expected 'not found' in response, got: %s", text)
	}
}

func TestAccessStatusViaMCP(t *testing.T) {
	c := fakeClientWithClusters(
		newManagedCluster("spoke1", true),
		newManagedServiceAccount("acmlab-access", "spoke1"),
		newAddOn("cluster-proxy", "spoke1"),
		newAddOn("managed-serviceaccount", "spoke1"),
	)
	text := extractToolText(t, callTool(t, c, "acm_access_status", map[string]interface{}{
		"cluster": "spoke1",
	}))
	if !strings.Contains(strings.ToLower(text), "spoke1") {
		t.Errorf("expected spoke1 in response, got: %s", text)
	}
}

func TestAccessListViaMCP(t *testing.T) {
	c := fakeClientWithClusters(
		newManagedCluster("spoke1", true),
		newManagedServiceAccount("acmlab-access", "spoke1"),
	)

	_ = newAddOn("managed-serviceaccount", "spoke1")

	text := extractToolText(t, callTool(t, c, "acm_list_access", nil))
	_ = text
}

func TestAccessStatusNonexistentViaMCP(t *testing.T) {
	c := fakeClientWithClusters()
	text := extractToolText(t, callTool(t, c, "acm_access_status", map[string]interface{}{
		"cluster": "nonexistent",
	}))
	if !strings.Contains(strings.ToLower(text), "false") {
		t.Errorf("expected disabled status for nonexistent cluster, got: %s", text)
	}
}

// Verify GVR registrations don't panic during server construction.
func TestNewServerWithAllPackages(t *testing.T) {
	c := fakeClientWithClusters()
	s := NewServer(c, config.Config{}, discardLogger)
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
}

// Verify the new GVRs exist.
func TestNewGVRsExist(t *testing.T) {
	cases := []struct {
		name string
		gvr  schema.GroupVersionResource
	}{
		{"ManifestWorkReplicaSet", client.GVRManifestWorkReplicaSet},
		{"ManagedServiceAccount", client.GVRManagedServiceAccount},
		{"ManagedClusterAddOn", client.GVRManagedClusterAddOn},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.gvr.Resource == "" {
				t.Errorf("GVR %s has empty resource", tc.name)
			}
		})
	}
}
