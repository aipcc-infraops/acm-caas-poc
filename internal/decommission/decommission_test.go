package decommission

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func managedCluster(name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	obj.SetName(name)
	obj.SetLabels(map[string]string{
		"caas-poc/owner": "team-alpha@example.com",
		"cloud":          "ibmcloud",
	})
	obj.Object["metadata"].(map[string]interface{})["creationTimestamp"] = "2025-11-01T00:00:00Z"
	return obj
}

func managedClusterInfo(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "internal.open-cluster-management.io/v1beta1",
			"kind":       "ManagedClusterInfo",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": name,
			},
			"status": map[string]interface{}{
				"nodeList": []interface{}{
					map[string]interface{}{
						"name":     "node-1",
						"capacity": map[string]interface{}{"cpu": "8", "memory": "32Gi"},
					},
					map[string]interface{}{
						"name":     "node-2",
						"capacity": map[string]interface{}{"cpu": "8", "memory": "32Gi"},
					},
					map[string]interface{}{
						"name":     "node-3",
						"capacity": map[string]interface{}{"cpu": "8", "memory": "32Gi"},
					},
				},
			},
		},
	}
}

func clusterDeployment(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata":   map[string]interface{}{"name": name, "namespace": name},
			"spec":       map[string]interface{}{"platform": map[string]interface{}{"aws": map[string]interface{}{}}},
		},
	}
}

func setupCluster(name string) []runtime.Object {
	return []runtime.Object{
		makeNamespace(name),
		managedCluster(name),
		managedClusterInfo(name),
	}
}

func newTestManager(objs ...runtime.Object) *Manager {
	return New(fakeClient(objs...), config.Config{}, discardLogger)
}

// --- Audit tests ---

func TestAuditCollectsClusterData(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)

	report, err := m.Audit(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if report.NodeCount != 3 {
		t.Errorf("NodeCount = %d, want 3", report.NodeCount)
	}
	if report.CPUCapacity != "24" {
		t.Errorf("CPUCapacity = %s, want 24", report.CPUCapacity)
	}
	if report.MemoryCapacity != "96Gi" {
		t.Errorf("MemoryCapacity = %s, want 96Gi", report.MemoryCapacity)
	}
	if report.Owner != "team-alpha@example.com" {
		t.Errorf("Owner = %s, want team-alpha@example.com", report.Owner)
	}
	if report.Platform != "ibmcloud" {
		t.Errorf("Platform = %s, want ibmcloud", report.Platform)
	}
}

func TestAuditDetectsPlatformFromClusterDeployment(t *testing.T) {
	mc := managedCluster("spoke1")
	labels := mc.GetLabels()
	delete(labels, "cloud")
	mc.SetLabels(labels)

	mci := managedClusterInfo("spoke1")
	cd := clusterDeployment("spoke1")
	m := newTestManager(makeNamespace("spoke1"), mc, mci, cd)

	report, err := m.Audit(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if report.Platform != "aws" {
		t.Errorf("Platform = %s, want aws", report.Platform)
	}
}

func TestAuditMissingClusterReturnsError(t *testing.T) {
	m := newTestManager()
	_, err := m.Audit(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for missing cluster")
	}
}

func TestParseMemory(t *testing.T) {
	tests := []struct {
		input   string
		wantVal int
		wantUnt string
	}{
		{"32Gi", 32, "Gi"},
		{"16384Mi", 16384, "Mi"},
		{"1024", 1024, ""},
	}
	for _, tt := range tests {
		val, unit := parseMemory(tt.input)
		if val != tt.wantVal || unit != tt.wantUnt {
			t.Errorf("parseMemory(%s) = (%d, %s), want (%d, %s)", tt.input, val, unit, tt.wantVal, tt.wantUnt)
		}
	}
}

// --- Manager tests ---

func TestStartCreatesStateAndAudits(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)

	state, err := m.Start(context.Background(), "spoke1", StartOpts{
		Owner:    "team-alpha@example.com",
		Deadline: "2026-09-28T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if state.Phase != PhaseAudited {
		t.Errorf("phase = %s, want audited", state.Phase)
	}
	if state.Audit == nil {
		t.Fatal("audit report is nil")
	}
	if state.Audit.NodeCount != 3 {
		t.Errorf("audit NodeCount = %d, want 3", state.Audit.NodeCount)
	}
}

func TestStartIdempotent(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)

	m.Start(context.Background(), "spoke1", StartOpts{})
	state, err := m.Start(context.Background(), "spoke1", StartOpts{})
	if err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if state.Phase != PhaseAudited {
		t.Errorf("phase = %s, want audited", state.Phase)
	}
}

func TestGetState(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)

	m.Start(context.Background(), "spoke1", StartOpts{})
	state, err := m.GetState(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if state.ClusterName != "spoke1" {
		t.Errorf("ClusterName = %s, want spoke1", state.ClusterName)
	}
}

func TestGetStateNotFound(t *testing.T) {
	m := newTestManager()
	_, err := m.GetState(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for missing state")
	}
}

func TestList(t *testing.T) {
	objs := append(setupCluster("spoke1"), setupCluster("spoke2")...)
	m := newTestManager(objs...)

	m.Start(context.Background(), "spoke1", StartOpts{})
	m.Start(context.Background(), "spoke2", StartOpts{})

	states, err := m.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(states) != 2 {
		t.Errorf("List len = %d, want 2", len(states))
	}
}

func TestCancel(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)

	m.Start(context.Background(), "spoke1", StartOpts{})
	if err := m.Cancel(context.Background(), "spoke1"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	_, err := m.GetState(context.Background(), "spoke1")
	if err == nil {
		t.Error("expected error after cancel")
	}
}

func TestAdvanceFromAuditedToNotified(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)

	m.Start(context.Background(), "spoke1", StartOpts{
		Owner:    "team-alpha@example.com",
		Deadline: "2026-09-28T00:00:00Z",
	})

	state, err := m.Advance(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if state.Phase != PhaseNotified {
		t.Errorf("phase = %s, want notified", state.Phase)
	}
}

func TestAdvanceFullCycle(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)

	m.Start(context.Background(), "spoke1", StartOpts{Owner: "team@example.com"})

	expected := []Phase{PhaseNotified, PhaseBackedUp, PhaseDrained, PhaseDeleted, PhaseCleaned}
	for _, exp := range expected {
		state, err := m.Advance(context.Background(), "spoke1")
		if err != nil {
			t.Fatalf("Advance to %s: %v", exp, err)
		}
		if state.Phase != exp {
			t.Errorf("phase = %s, want %s", state.Phase, exp)
		}
	}

	state, err := m.Advance(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("Advance past cleaned: %v", err)
	}
	if state.Phase != PhaseCleaned {
		t.Errorf("phase = %s, want cleaned (should not advance further)", state.Phase)
	}
}

// --- Notify tests ---

func TestNotifyRecordsTimestamp(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{Owner: "team@example.com"})

	err := m.Notify(context.Background(), "spoke1", "team@example.com", "2026-09-28T00:00:00Z")
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}

	state, _ := m.GetState(context.Background(), "spoke1")
	if state.NotifiedAt == "" {
		t.Error("NotifiedAt is empty after Notify")
	}
}

func TestNotifyIdempotent(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{Owner: "team@example.com"})

	m.Notify(context.Background(), "spoke1", "team@example.com", "2026-09-28T00:00:00Z")
	state1, _ := m.GetState(context.Background(), "spoke1")
	firstNotify := state1.NotifiedAt

	m.Notify(context.Background(), "spoke1", "team@example.com", "2026-10-01T00:00:00Z")
	state2, _ := m.GetState(context.Background(), "spoke1")

	if state2.NotifiedAt != firstNotify {
		t.Errorf("second Notify changed NotifiedAt: %s -> %s", firstNotify, state2.NotifiedAt)
	}
}

// --- Backup tests ---

func TestBackupRecordsPath(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{})

	err := m.Backup(context.Background(), "spoke1", "/tmp/backup/spoke1")
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	state, _ := m.GetState(context.Background(), "spoke1")
	if state.BackupPath != "/tmp/backup/spoke1" {
		t.Errorf("BackupPath = %s, want /tmp/backup/spoke1", state.BackupPath)
	}
}

func TestBackupDefaultPath(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{})

	err := m.Backup(context.Background(), "spoke1", "")
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}

	state, _ := m.GetState(context.Background(), "spoke1")
	if state.BackupPath != "./decommission-backups/spoke1" {
		t.Errorf("BackupPath = %s, want ./decommission-backups/spoke1", state.BackupPath)
	}
}

func TestBackupIdempotent(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{})

	m.Backup(context.Background(), "spoke1", "/first/path")
	m.Backup(context.Background(), "spoke1", "/second/path")

	state, _ := m.GetState(context.Background(), "spoke1")
	if state.BackupPath != "/first/path" {
		t.Errorf("BackupPath = %s, want /first/path (idempotent)", state.BackupPath)
	}
}

// --- Drain tests ---

func TestDrainSuccess(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{})

	if err := m.Drain(context.Background(), "spoke1", 5*time.Minute); err != nil {
		t.Fatalf("Drain: %v", err)
	}
}

func TestDrainDefaultTimeout(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{})

	if err := m.Drain(context.Background(), "spoke1", 0); err != nil {
		t.Fatalf("Drain default timeout: %v", err)
	}
}

// --- Delete and Cleanup tests ---

func TestDeleteHiveCluster(t *testing.T) {
	objs := setupCluster("spoke1")
	cd := clusterDeployment("spoke1")
	objs = append(objs, cd)
	m := newTestManager(objs...)

	err := m.Delete(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = m.client.Get(context.Background(), client.GVRClusterDeployment, "spoke1", "spoke1")
	if err == nil {
		t.Error("ClusterDeployment should be deleted")
	}
}

func TestDeleteImportedCluster(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)

	err := m.Delete(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = m.client.Get(context.Background(), client.GVRManagedCluster, "", "spoke1")
	if err == nil {
		t.Error("ManagedCluster should be deleted")
	}
}

func TestDeleteIdempotent(t *testing.T) {
	m := newTestManager()
	err := m.Delete(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("Delete should be idempotent: %v", err)
	}
}

func TestCleanupRemovesNamespace(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)

	err := m.Cleanup(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	_, err = m.client.Get(context.Background(), client.GVRNamespace, "", "spoke1")
	if err == nil {
		t.Error("Namespace should be deleted")
	}
}

func TestCleanupIdempotent(t *testing.T) {
	m := newTestManager()
	err := m.Cleanup(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("Cleanup should be idempotent: %v", err)
	}
}

func TestDeleteWithClusterDeploymentDeleteError(t *testing.T) {
	objs := setupCluster("spoke1")
	cd := clusterDeployment("spoke1")
	objs = append(objs, cd)
	m := newTestManager(objs...)

	m.Delete(context.Background(), "spoke1")
	err := m.Delete(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("Delete should handle already-deleted CD: %v", err)
	}
}

func TestStartOwnerFromAudit(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)

	state, err := m.Start(context.Background(), "spoke1", StartOpts{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if state.Owner != "team-alpha@example.com" {
		t.Errorf("Owner = %s, want team-alpha@example.com (from audit)", state.Owner)
	}
}

func TestStateFromConfigMapNoData(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": "test"},
		},
	}
	_, err := stateFromConfigMap(obj)
	if err == nil {
		t.Error("expected error for ConfigMap with no data")
	}
}

func TestAdvanceAtCleanedNoOp(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{Owner: "team@example.com"})

	for i := 0; i < 5; i++ {
		m.Advance(context.Background(), "spoke1")
	}

	state, err := m.Advance(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("Advance at cleaned: %v", err)
	}
	if state.Phase != PhaseCleaned {
		t.Errorf("phase = %s, want cleaned", state.Phase)
	}
}

func TestAdvanceNotFoundReturnsError(t *testing.T) {
	m := newTestManager()
	_, err := m.Advance(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for advancing nonexistent workflow")
	}
}

func TestDetectPlatformNoCDReturnsUnknown(t *testing.T) {
	c := fakeClient()
	platform := detectPlatformFromCD(context.Background(), c, "nonexistent")
	if platform != "unknown" {
		t.Errorf("platform = %s, want unknown", platform)
	}
}

func TestDetectPlatformCDNoPlatformKey(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata":   map[string]interface{}{"name": "spoke1", "namespace": "spoke1"},
			"spec":       map[string]interface{}{"platform": map[string]interface{}{}},
		},
	}
	c := fakeClient(makeNamespace("spoke1"), cd)
	platform := detectPlatformFromCD(context.Background(), c, "spoke1")
	if platform != "unknown" {
		t.Errorf("platform = %s, want unknown", platform)
	}
}

func TestBackupReturnsErrorForMissingState(t *testing.T) {
	m := newTestManager()
	err := m.Backup(context.Background(), "nonexistent", "/tmp")
	if err == nil {
		t.Error("expected error for backup on nonexistent state")
	}
}

func TestNotifyReturnsErrorForMissingState(t *testing.T) {
	m := newTestManager()
	err := m.Notify(context.Background(), "nonexistent", "owner", "deadline")
	if err == nil {
		t.Error("expected error for notify on nonexistent state")
	}
}

func TestStateToConfigMapAllFields(t *testing.T) {
	state := &DecommissionState{
		ClusterName:    "spoke1",
		Phase:          PhaseNotified,
		Owner:          "owner@example.com",
		Deadline:       "2026-09-28T00:00:00Z",
		NotifiedAt:     "2026-09-14T12:00:00Z",
		BackupPath:     "/backup/spoke1",
		KubeconfigPath: "/kubeconfig/spoke1",
	}
	cm := stateToConfigMap(state)
	data, _, _ := unstructured.NestedStringMap(cm.Object, "data")
	if data["notifiedAt"] != "2026-09-14T12:00:00Z" {
		t.Errorf("notifiedAt = %s, want 2026-09-14T12:00:00Z", data["notifiedAt"])
	}
	if data["backupPath"] != "/backup/spoke1" {
		t.Errorf("backupPath = %s, want /backup/spoke1", data["backupPath"])
	}
	if data["kubeconfigPath"] != "/kubeconfig/spoke1" {
		t.Errorf("kubeconfigPath = %s, want /kubeconfig/spoke1", data["kubeconfigPath"])
	}
}

func TestCleanupRemovesManifestWorks(t *testing.T) {
	objs := setupCluster("spoke1")
	mw := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata":   map[string]interface{}{"name": "spoke1-tenant", "namespace": "spoke1"},
		},
	}
	objs = append(objs, mw)
	m := newTestManager(objs...)

	err := m.Cleanup(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	_, err = m.client.Get(context.Background(), client.GVRManifestWork, "spoke1", "spoke1-tenant")
	if err == nil {
		t.Error("ManifestWork should be deleted")
	}
}
