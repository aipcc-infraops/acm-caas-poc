package gpu

import (
	"context"
	"fmt"
	"testing"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

func TestRouteByVersionFindsCluster(t *testing.T) {
	cluster := managedCluster("gpu-h100-01", map[string]string{
		"ai-platform-version": "2.18",
		"gpu-available":       "true",
	})
	mgr := newTestManager(cluster)

	name, err := mgr.RouteByVersion(context.Background(), "2.18")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "gpu-h100-01" {
		t.Fatalf("expected gpu-h100-01, got %s", name)
	}
}

func TestRouteByVersionErrorsWhenNone(t *testing.T) {
	cluster := managedCluster("gpu-h100-01", map[string]string{
		"ai-platform-version": "2.17",
		"gpu-available":       "true",
	})
	mgr := newTestManager(cluster)

	_, err := mgr.RouteByVersion(context.Background(), "2.18")
	if err == nil {
		t.Fatal("expected error for missing version")
	}
}

func TestRouteByVersionSkipsUnavailable(t *testing.T) {
	unavailable := managedCluster("gpu-h100-01", map[string]string{
		"ai-platform-version": "2.18",
		"gpu-available":       "false",
	})
	available := managedCluster("gpu-h100-02", map[string]string{
		"ai-platform-version": "2.18",
		"gpu-available":       "true",
	})
	mgr := newTestManager(unavailable, available)

	name, err := mgr.RouteByVersion(context.Background(), "2.18")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "gpu-h100-02" {
		t.Fatalf("expected gpu-h100-02, got %s", name)
	}
}

func TestEnforceVersionPolicyCreatesResources(t *testing.T) {
	mc := managedCluster("gpu-h100-01", map[string]string{})
	mgr := newTestManager(mc)
	ctx := context.Background()

	if err := mgr.EnforceVersionPolicy(ctx, "gpu-h100-01", "2.18"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	prefix := versionPolicyName("gpu-h100-01")

	_, err := mgr.client.Get(ctx, client.GVRConfigurationPolicy, DefaultNamespace, prefix+"-config")
	if err != nil {
		t.Fatalf("config policy not created: %v", err)
	}

	_, err = mgr.client.Get(ctx, client.GVRPolicy, DefaultNamespace, prefix)
	if err != nil {
		t.Fatalf("policy not created: %v", err)
	}

	_, err = mgr.client.Get(ctx, client.GVRPlacement, DefaultNamespace, prefix+"-placement")
	if err != nil {
		t.Fatalf("placement not created: %v", err)
	}

	_, err = mgr.client.Get(ctx, client.GVRPlacementBinding, DefaultNamespace, prefix+"-binding")
	if err != nil {
		t.Fatalf("placement binding not created: %v", err)
	}
}

func TestRemoveVersionPolicyDeletesResources(t *testing.T) {
	mc := managedCluster("gpu-h100-01", map[string]string{})
	mgr := newTestManager(mc)
	ctx := context.Background()

	if err := mgr.EnforceVersionPolicy(ctx, "gpu-h100-01", "2.18"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := mgr.RemoveVersionPolicy(ctx, "gpu-h100-01"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	prefix := versionPolicyName("gpu-h100-01")
	_, err := mgr.client.Get(ctx, client.GVRPolicy, DefaultNamespace, prefix)
	if err == nil {
		t.Fatal("policy should have been deleted")
	}
}

func TestProvisionForVersionLabelsCluster(t *testing.T) {
	cluster := managedCluster("gpu-h100-01", map[string]string{})
	mgr := newTestManager(cluster)
	ctx := context.Background()

	if err := mgr.ProvisionForVersion(ctx, "gpu-h100-01", "2.18"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated, err := mgr.client.Get(ctx, client.GVRManagedCluster, "", "gpu-h100-01")
	if err != nil {
		t.Fatalf("cluster not found: %v", err)
	}

	labels := updated.GetLabels()
	if labels["ai-platform-version"] != "2.18" {
		t.Fatalf("expected version label 2.18, got %s", labels["ai-platform-version"])
	}
	if labels["ai-platform-channel"] != "stable" {
		t.Fatalf("expected channel label stable, got %s", labels["ai-platform-channel"])
	}
	if labels["gpu-available"] != "true" {
		t.Fatalf("expected gpu-available=true, got %s", labels["gpu-available"])
	}
}

func TestProvisionForVersionCreatesManifestWork(t *testing.T) {
	cluster := managedCluster("gpu-h100-01", map[string]string{})
	mgr := newTestManager(cluster)
	ctx := context.Background()

	if err := mgr.ProvisionForVersion(ctx, "gpu-h100-01", "2.18"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mwName := fmt.Sprintf("ai-platform-operator-%s", "2.18")
	mw, err := mgr.client.Get(ctx, client.GVRManifestWork, "gpu-h100-01", mwName)
	if err != nil {
		t.Fatalf("manifest work not created: %v", err)
	}

	labels := mw.GetLabels()
	if labels["acmlab.redhat.com/operator-version"] != "2.18" {
		t.Fatalf("expected operator-version label 2.18, got %s", labels["acmlab.redhat.com/operator-version"])
	}
}

func TestListVersionClustersReturnsAll(t *testing.T) {
	c1 := managedCluster("gpu-h100-01", map[string]string{
		"ai-platform-version": "2.18",
		"ai-platform-channel": "stable",
		"gpu-available":       "true",
	})
	c2 := managedCluster("gpu-l4-01", map[string]string{
		"ai-platform-version": "2.17",
		"ai-platform-channel": "stable",
		"ai-platform-build":   "nightly",
		"gpu-available":       "false",
	})
	mgr := newTestManager(c1, c2)

	clusters, err := mgr.ListVersionClusters(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(clusters))
	}
}

func TestListVersionClustersSkipsUnlabelled(t *testing.T) {
	c1 := managedCluster("gpu-h100-01", map[string]string{
		"ai-platform-version": "2.18",
		"gpu-available":       "true",
	})
	c2 := managedCluster("regular-cluster", map[string]string{
		"env": "prod",
	})
	mgr := newTestManager(c1, c2)

	clusters, err := mgr.ListVersionClusters(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	if clusters[0].Name != "gpu-h100-01" {
		t.Fatalf("expected gpu-h100-01, got %s", clusters[0].Name)
	}
}

func TestParseVersionCluster(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected VersionCluster
	}{
		{
			name: "full labels",
			labels: map[string]string{
				"ai-platform-version": "2.18",
				"ai-platform-channel": "stable",
				"ai-platform-build":   "nightly",
				"gpu-available":       "true",
			},
			expected: VersionCluster{
				Name:      "test-cluster",
				Version:   "2.18",
				Channel:   "stable",
				Build:     "nightly",
				Available: true,
			},
		},
		{
			name: "minimal labels",
			labels: map[string]string{
				"ai-platform-version": "2.17",
			},
			expected: VersionCluster{
				Name:      "test-cluster",
				Version:   "2.17",
				Channel:   "",
				Build:     "",
				Available: false,
			},
		},
		{
			name: "unavailable",
			labels: map[string]string{
				"ai-platform-version": "2.18",
				"ai-platform-channel": "stable",
				"gpu-available":       "false",
			},
			expected: VersionCluster{
				Name:      "test-cluster",
				Version:   "2.18",
				Channel:   "stable",
				Available: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := map[string]interface{}{}
			for k, v := range tt.labels {
				l[k] = v
			}
			obj := map[string]interface{}{
				"metadata": map[string]interface{}{
					"name":   "test-cluster",
					"labels": l,
				},
			}
			got := parseVersionCluster(obj)
			if got != tt.expected {
				t.Fatalf("expected %+v, got %+v", tt.expected, got)
			}
		})
	}
}

func TestBuildVersionEnforcementPolicy(t *testing.T) {
	p := buildVersionEnforcementPolicy("gpu-h100-01", "2.18")

	if p.GetName() != "version-enforce-gpu-h100-01-config" {
		t.Fatalf("unexpected name: %s", p.GetName())
	}

	labels := p.GetLabels()
	if labels["acmlab.redhat.com/managed"] != "true" {
		t.Fatal("missing managed label")
	}
	if labels["acmlab.redhat.com/version-segregation"] != "true" {
		t.Fatal("missing version-segregation label")
	}

	spec, _ := p.Object["spec"].(map[string]interface{})
	if spec["remediationAction"] != "inform" {
		t.Fatalf("expected inform remediation, got %s", spec["remediationAction"])
	}
}

func TestBuildOperatorManifestWork(t *testing.T) {
	mw := buildOperatorManifestWork("gpu-h100-01", "2.18")

	if mw.GetName() != "ai-platform-operator-2.18" {
		t.Fatalf("unexpected name: %s", mw.GetName())
	}
	if mw.GetNamespace() != "gpu-h100-01" {
		t.Fatalf("unexpected namespace: %s", mw.GetNamespace())
	}

	labels := mw.GetLabels()
	if labels["acmlab.redhat.com/operator-version"] != "2.18" {
		t.Fatalf("unexpected version label: %s", labels["acmlab.redhat.com/operator-version"])
	}

	spec, _ := mw.Object["spec"].(map[string]interface{})
	workload, _ := spec["workload"].(map[string]interface{})
	manifests, _ := workload["manifests"].([]interface{})
	if len(manifests) != 3 {
		t.Fatalf("expected 3 manifests (namespace, operatorgroup, subscription), got %d", len(manifests))
	}
}

func TestBuildVersionPlacement(t *testing.T) {
	p := buildVersionPlacement("gpu-h100-01")
	if p.GetName() != "version-enforce-gpu-h100-01-placement" {
		t.Fatalf("unexpected name: %s", p.GetName())
	}
	if p.GetNamespace() != DefaultNamespace {
		t.Fatalf("unexpected namespace: %s", p.GetNamespace())
	}
}

func TestBuildVersionPlacementBinding(t *testing.T) {
	b := buildVersionPlacementBinding("gpu-h100-01")
	if b.GetName() != "version-enforce-gpu-h100-01-binding" {
		t.Fatalf("unexpected name: %s", b.GetName())
	}

	ref, _ := b.Object["placementRef"].(map[string]interface{})
	if ref["name"] != "version-enforce-gpu-h100-01-placement" {
		t.Fatalf("unexpected placement ref: %s", ref["name"])
	}

	subjects, _ := b.Object["subjects"].([]interface{})
	if len(subjects) != 1 {
		t.Fatalf("expected 1 subject, got %d", len(subjects))
	}
	sub, _ := subjects[0].(map[string]interface{})
	if sub["name"] != "version-enforce-gpu-h100-01" {
		t.Fatalf("unexpected subject name: %s", sub["name"])
	}
}
