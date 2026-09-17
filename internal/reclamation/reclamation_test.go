package reclamation

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

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRManagedCluster:         "ManagedClusterList",
			client.GVRClusterDeployment:      "ClusterDeploymentList",
			client.GVRNamespace:              "NamespaceList",
			client.GVRCAPIMachineDeployment:  "MachineDeploymentList",
			client.GVRHostedCluster:          "HostedClusterList",
			client.GVRNodePool:              "NodePoolList",
			client.GVRCAPICluster:           "ClusterList",
		}, objs...)
	return &client.Client{Dynamic: fc}
}

func newTestManager(now time.Time, objs ...runtime.Object) *Manager {
	m := New(fakeClient(objs...), config.Config{}, discardLogger)
	m.now = func() time.Time { return now }
	return m
}

func managedClusterWithTTL(name string, ttlHours int, expiry time.Time) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	obj.SetName(name)
	obj.SetLabels(map[string]string{
		"caas/ttl-hours":   "48",
		"caas/expiry-date": expiry.UTC().Format(time.RFC3339),
		"caas/owner":       "team-alpha",
	})
	return obj
}

func managedClusterNoTTL(name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	obj.SetName(name)
	obj.SetLabels(map[string]string{"vendor": "OpenShift"})
	return obj
}

func hiveClusterDeployment(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata":   map[string]interface{}{"name": name, "namespace": name},
			"spec":       map[string]interface{}{"powerState": "Running"},
		},
	}
}

func TestSetTTLStampsLabels(t *testing.T) {
	mc := managedClusterNoTTL("spoke1")
	m := newTestManager(time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC), mc)

	if err := m.SetTTL(context.Background(), "spoke1", 48); err != nil {
		t.Fatalf("SetTTL: %v", err)
	}

	obj, err := m.client.Get(context.Background(), client.GVRManagedCluster, "", "spoke1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	labels := obj.GetLabels()
	if labels["caas/ttl-hours"] != "48" {
		t.Errorf("ttl-hours = %s, want 48", labels["caas/ttl-hours"])
	}
	if labels["caas/expiry-date"] == "" {
		t.Error("expiry-date label not set")
	}
}

func TestSetTTLCalculatesExpiryCorrectly(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	mc := managedClusterNoTTL("spoke1")
	m := newTestManager(now, mc)

	if err := m.SetTTL(context.Background(), "spoke1", 24); err != nil {
		t.Fatalf("SetTTL: %v", err)
	}

	obj, err := m.client.Get(context.Background(), client.GVRManagedCluster, "", "spoke1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	labels := obj.GetLabels()
	expiry, parseErr := time.Parse(time.RFC3339, labels["caas/expiry-date"])
	if parseErr != nil {
		t.Fatalf("parse expiry: %v", parseErr)
	}

	expected := now.Add(24 * time.Hour)
	if !expiry.Equal(expected) {
		t.Errorf("expiry = %v, want %v", expiry, expected)
	}
}

func TestSetTTLRejectsNonPositive(t *testing.T) {
	m := newTestManager(time.Now(), managedClusterNoTTL("spoke1"))

	if err := m.SetTTL(context.Background(), "spoke1", 0); err == nil {
		t.Error("expected error for TTL=0")
	}
	if err := m.SetTTL(context.Background(), "spoke1", -5); err == nil {
		t.Error("expected error for negative TTL")
	}
}

func TestCheckExpiredFindsOnlyExpired(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	expired := managedClusterWithTTL("expired-1", 48, now.Add(-1*time.Hour))
	active := managedClusterWithTTL("active-1", 48, now.Add(24*time.Hour))
	m := newTestManager(now, expired, active)

	result, err := m.CheckExpired(context.Background())
	if err != nil {
		t.Fatalf("CheckExpired: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 expired, got %d", len(result))
	}
	if result[0].Name != "expired-1" {
		t.Errorf("expired cluster = %s, want expired-1", result[0].Name)
	}
}

func TestCheckExpiredReturnsEmptyWhenNoneExpired(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	active := managedClusterWithTTL("active-1", 48, now.Add(24*time.Hour))
	m := newTestManager(now, active)

	result, err := m.CheckExpired(context.Background())
	if err != nil {
		t.Fatalf("CheckExpired: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 expired, got %d", len(result))
	}
}

func TestListTTLsReturnsAllWithLabels(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	c1 := managedClusterWithTTL("spoke1", 48, now.Add(24*time.Hour))
	c2 := managedClusterWithTTL("spoke2", 36, now.Add(-2*time.Hour))
	m := newTestManager(now, c1, c2)

	result, err := m.ListTTLs(context.Background())
	if err != nil {
		t.Fatalf("ListTTLs: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 TTLs, got %d", len(result))
	}
}

func TestListTTLsSkipsClustersWithoutLabels(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	withTTL := managedClusterWithTTL("spoke1", 48, now.Add(24*time.Hour))
	noTTL := managedClusterNoTTL("spoke2")
	m := newTestManager(now, withTTL, noTTL)

	result, err := m.ListTTLs(context.Background())
	if err != nil {
		t.Fatalf("ListTTLs: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 TTL, got %d", len(result))
	}
	if result[0].Name != "spoke1" {
		t.Errorf("name = %s, want spoke1", result[0].Name)
	}
}

func TestExtendTTLAddsHours(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	originalExpiry := now.Add(12 * time.Hour)
	mc := managedClusterWithTTL("spoke1", 48, originalExpiry)
	m := newTestManager(now, mc)

	if err := m.ExtendTTL(context.Background(), "spoke1", 24, "training job running"); err != nil {
		t.Fatalf("ExtendTTL: %v", err)
	}

	obj, err := m.client.Get(context.Background(), client.GVRManagedCluster, "", "spoke1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	labels := obj.GetLabels()
	newExpiry, parseErr := time.Parse(time.RFC3339, labels["caas/expiry-date"])
	if parseErr != nil {
		t.Fatalf("parse expiry: %v", parseErr)
	}

	expected := originalExpiry.Add(24 * time.Hour)
	if !newExpiry.Equal(expected) {
		t.Errorf("new expiry = %v, want %v", newExpiry, expected)
	}
}

func TestExtendTTLSetsJustification(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	mc := managedClusterWithTTL("spoke1", 48, now.Add(12*time.Hour))
	m := newTestManager(now, mc)

	if err := m.ExtendTTL(context.Background(), "spoke1", 24, "critical workload"); err != nil {
		t.Fatalf("ExtendTTL: %v", err)
	}

	obj, err := m.client.Get(context.Background(), client.GVRManagedCluster, "", "spoke1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	annotations := obj.GetAnnotations()
	if annotations["caas/extend-justification"] != "critical workload" {
		t.Errorf("justification = %s, want 'critical workload'", annotations["caas/extend-justification"])
	}
}

func TestExtendTTLRejectsEmptyJustification(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	mc := managedClusterWithTTL("spoke1", 48, now.Add(12*time.Hour))
	m := newTestManager(now, mc)

	if err := m.ExtendTTL(context.Background(), "spoke1", 24, ""); err == nil {
		t.Error("expected error for empty justification")
	}
}

func TestExtendTTLRejectsClusterWithoutTTL(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	mc := managedClusterNoTTL("spoke1")
	m := newTestManager(now, mc)

	if err := m.ExtendTTL(context.Background(), "spoke1", 24, "reason"); err == nil {
		t.Error("expected error for cluster without TTL")
	}
}

func TestReclaimHiveClusterHibernates(t *testing.T) {
	mc := managedClusterNoTTL("spoke1")
	cd := hiveClusterDeployment("spoke1")
	m := newTestManager(time.Now(), mc, cd)

	result, err := m.ReclaimCluster(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("ReclaimCluster: %v", err)
	}
	if result.Action != "hibernated" {
		t.Errorf("action = %s, want hibernated", result.Action)
	}
}

func TestReclaimImportedClusterDetaches(t *testing.T) {
	mc := managedClusterNoTTL("imported-1")
	m := newTestManager(time.Now(), mc)

	result, err := m.ReclaimCluster(context.Background(), "imported-1")
	if err != nil {
		t.Fatalf("ReclaimCluster: %v", err)
	}
	if result.Action != "detached" {
		t.Errorf("action = %s, want detached", result.Action)
	}
}

func TestParseClusterTTL(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		labels     map[string]interface{}
		wantNil    bool
		wantExpiry bool
		wantOwner  string
	}{
		{
			name:    "no TTL label returns nil",
			labels:  map[string]interface{}{"vendor": "OpenShift"},
			wantNil: true,
		},
		{
			name:       "valid TTL with expiry in future",
			labels:     map[string]interface{}{"caas/ttl-hours": "48", "caas/expiry-date": now.Add(24 * time.Hour).Format(time.RFC3339)},
			wantExpiry: false,
		},
		{
			name:       "valid TTL with expiry in past",
			labels:     map[string]interface{}{"caas/ttl-hours": "48", "caas/expiry-date": now.Add(-1 * time.Hour).Format(time.RFC3339)},
			wantExpiry: true,
		},
		{
			name:      "TTL with owner",
			labels:    map[string]interface{}{"caas/ttl-hours": "36", "caas/expiry-date": now.Add(10 * time.Hour).Format(time.RFC3339), "caas/owner": "team-beta"},
			wantOwner: "team-beta",
		},
		{
			name:    "invalid TTL hours returns nil",
			labels:  map[string]interface{}{"caas/ttl-hours": "not-a-number"},
			wantNil: true,
		},
		{
			name:   "TTL without expiry date",
			labels: map[string]interface{}{"caas/ttl-hours": "48"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := map[string]interface{}{
				"metadata": map[string]interface{}{
					"name":   "test-cluster",
					"labels": tt.labels,
				},
			}
			result := parseClusterTTL(obj, now)

			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("expected non-nil result")
			}

			if result.Expired != tt.wantExpiry {
				t.Errorf("Expired = %v, want %v", result.Expired, tt.wantExpiry)
			}

			if tt.wantOwner != "" && result.Owner != tt.wantOwner {
				t.Errorf("Owner = %s, want %s", result.Owner, tt.wantOwner)
			}
		})
	}
}

func TestFilterExpiredReturnsOnlyExpired(t *testing.T) {
	ttls := []ClusterTTL{
		{Name: "a", Expired: true},
		{Name: "b", Expired: false},
		{Name: "c", Expired: true},
	}
	result := filterExpired(ttls)
	if len(result) != 2 {
		t.Fatalf("expected 2 expired, got %d", len(result))
	}
	if result[0].Name != "a" || result[1].Name != "c" {
		t.Errorf("unexpected names: %s, %s", result[0].Name, result[1].Name)
	}
}

func TestBuildTTLLabelPatch(t *testing.T) {
	expiry := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	patch := buildTTLLabelPatch(48, expiry)

	meta, ok := patch["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("missing metadata")
	}
	labels, ok := meta["labels"].(map[string]interface{})
	if !ok {
		t.Fatal("missing labels")
	}
	if labels["caas/ttl-hours"] != "48" {
		t.Errorf("ttl-hours = %v, want 48", labels["caas/ttl-hours"])
	}
	if labels["caas/expiry-date"] != "2026-09-19T12:00:00Z" {
		t.Errorf("expiry-date = %v, want 2026-09-19T12:00:00Z", labels["caas/expiry-date"])
	}
}

func TestBuildExtendPatch(t *testing.T) {
	expiry := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	patch := buildExtendPatch(expiry, "training job")

	meta := patch["metadata"].(map[string]interface{})
	annotations := meta["annotations"].(map[string]interface{})
	if annotations["caas/extend-justification"] != "training job" {
		t.Errorf("justification = %v, want 'training job'", annotations["caas/extend-justification"])
	}
}
