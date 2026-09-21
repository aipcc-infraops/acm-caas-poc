package decommission

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

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

func TestAdvanceBlocksAtNotify(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)

	m.Start(context.Background(), "spoke1", StartOpts{
		Owner:    "team-alpha@example.com",
		Deadline: "2026-09-28T00:00:00Z",
	})

	_, err := m.Advance(context.Background(), "spoke1")
	if err == nil {
		t.Fatal("Advance should block at notify until notification is implemented")
	}

	state, _ := m.GetState(context.Background(), "spoke1")
	if state.Phase != PhaseAudited {
		t.Errorf("phase = %s, want audited (should not advance past unimplemented step)", state.Phase)
	}
}

func TestAdvanceBlocksAtBackup(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{Owner: "team@example.com"})

	// Manually advance past notify
	state, _ := m.GetState(context.Background(), "spoke1")
	state.Phase = PhaseNotified
	state.NotifiedAt = "2026-09-18T12:00:00Z"
	setState(context.Background(), m.client, state)

	_, err := m.Advance(context.Background(), "spoke1")
	if err == nil {
		t.Fatal("Advance should block at backup until real export is implemented")
	}

	state, _ = m.GetState(context.Background(), "spoke1")
	if state.Phase != PhaseNotified {
		t.Errorf("phase = %s, want notified (should not advance past unimplemented step)", state.Phase)
	}
}

func TestAdvanceBlocksAtDrain(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{Owner: "team@example.com"})

	// Manually advance past notify and backup
	state, _ := m.GetState(context.Background(), "spoke1")
	state.Phase = PhaseBackedUp
	state.NotifiedAt = "2026-09-18T12:00:00Z"
	state.BackupPath = "/tmp/backup/spoke1"
	setState(context.Background(), m.client, state)

	_, err := m.Advance(context.Background(), "spoke1")
	if err == nil {
		t.Fatal("Advance should block at drain until real drain is implemented")
	}

	state, _ = m.GetState(context.Background(), "spoke1")
	if state.Phase != PhaseBackedUp {
		t.Errorf("phase = %s, want backed-up (should not advance past unimplemented step)", state.Phase)
	}
}

// --- Notify tests ---

func TestNotifyBlocksUntilImplemented(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{Owner: "team@example.com"})

	err := m.Notify(context.Background(), "spoke1", "team@example.com", "2026-09-28T00:00:00Z")
	if err == nil {
		t.Fatal("Notify should return error until real notification is implemented")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("expected 'not implemented' error, got: %v", err)
	}
}

func TestNotifyIdempotentWhenAlreadyNotified(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{Owner: "team@example.com"})

	// Manually set NotifiedAt to simulate a completed notification
	state, _ := m.GetState(context.Background(), "spoke1")
	state.NotifiedAt = "2026-09-18T12:00:00Z"
	setState(context.Background(), m.client, state)

	err := m.Notify(context.Background(), "spoke1", "team@example.com", "2026-10-01T00:00:00Z")
	if err != nil {
		t.Fatalf("Notify with existing NotifiedAt should be idempotent: %v", err)
	}
}

// --- Backup tests ---

func TestBackupBlocksUntilImplemented(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{})

	err := m.Backup(context.Background(), "spoke1", "/tmp/backup/spoke1")
	if err == nil {
		t.Fatal("Backup should return error until real export is implemented")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("expected 'not implemented' error, got: %v", err)
	}
}

func TestBackupIdempotentWhenAlreadyBackedUp(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{})

	// Manually set BackupPath to simulate a completed backup
	state, _ := m.GetState(context.Background(), "spoke1")
	state.BackupPath = "/completed/backup"
	setState(context.Background(), m.client, state)

	err := m.Backup(context.Background(), "spoke1", "/another/path")
	if err != nil {
		t.Fatalf("Backup with existing BackupPath should be idempotent: %v", err)
	}
}

// --- Drain tests ---

func TestDrainBlocksUntilImplemented(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{})

	err := m.Drain(context.Background(), "spoke1", 5*time.Minute)
	if err == nil {
		t.Fatal("Drain should return error until real drain is implemented")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("expected 'not implemented' error, got: %v", err)
	}
}

func TestDrainBlocksWithDefaultTimeout(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{})

	err := m.Drain(context.Background(), "spoke1", 0)
	if err == nil {
		t.Fatal("Drain should return error until real drain is implemented")
	}
}

// --- Delete and Cleanup tests ---

func TestDeleteHiveCluster(t *testing.T) {
	objs := setupCluster("spoke1")
	cd := clusterDeployment("spoke1")
	objs = append(objs, cd)
	m := newTestManager(objs...)

	hive, err := m.Delete(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !hive {
		t.Error("Delete should report Hive cluster")
	}

	_, err = m.client.Get(context.Background(), client.GVRClusterDeployment, "spoke1", "spoke1")
	if err == nil {
		t.Error("ClusterDeployment should be deleted")
	}
}

func TestDeleteImportedCluster(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)

	hive, err := m.Delete(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if hive {
		t.Error("Delete should report imported cluster")
	}

	_, err = m.client.Get(context.Background(), client.GVRManagedCluster, "", "spoke1")
	if err == nil {
		t.Error("ManagedCluster should be deleted")
	}
}

func TestDeleteIdempotent(t *testing.T) {
	m := newTestManager()
	_, err := m.Delete(context.Background(), "nonexistent")
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
	_, err := m.Delete(context.Background(), "spoke1")
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

	// Manually set phase to cleaned to test no-op behavior
	state, _ := m.GetState(context.Background(), "spoke1")
	state.Phase = PhaseCleaned
	setState(context.Background(), m.client, state)

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

func TestCleanupLogsWarnOnDeleteErrors(t *testing.T) {
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

	fc := m.client.Dynamic.(k8stesting.FakeClient)
	fc.PrependReactor("delete", "manifestworks", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("connection refused")
	})
	fc.PrependReactor("delete", "managedclusters", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("connection refused")
	})
	fc.PrependReactor("delete", "namespaces", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("connection refused")
	})

	err := m.Cleanup(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("Cleanup should not return error on delete failures: %v", err)
	}
}

func TestDeleteRejectsNonNotFoundError(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)

	fc := m.client.Dynamic.(k8stesting.FakeClient)
	fc.PrependReactor("get", "clusterdeployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("forbidden: insufficient permissions")
	})

	_, err := m.Delete(context.Background(), "spoke1")
	if err == nil {
		t.Fatal("Delete should fail when ClusterDeployment lookup returns non-NotFound error")
	}
	if !strings.Contains(err.Error(), "cannot determine cluster type") {
		t.Errorf("expected 'cannot determine cluster type' error, got: %v", err)
	}
}

func TestAdvanceDeletePhaseWithEvidence(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{Owner: "team@example.com"})

	state, _ := m.GetState(context.Background(), "spoke1")
	state.Phase = PhaseDrained
	state.NotifiedAt = "2026-09-18T12:00:00Z"
	state.BackupPath = "/tmp/backup/spoke1"
	setState(context.Background(), m.client, state)

	state, err := m.Advance(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("Advance to deleted: %v", err)
	}
	if state.Phase != PhaseDeleted {
		t.Errorf("phase = %s, want deleted", state.Phase)
	}
}

func TestAdvanceDeleteBlocksOldWorkflowWithoutEvidence(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{Owner: "team@example.com"})

	// Simulate a workflow persisted by the previous version: phase=drained
	// but no NotifiedAt or BackupPath (the old no-op stubs didn't set them)
	state, _ := m.GetState(context.Background(), "spoke1")
	state.Phase = PhaseDrained
	setState(context.Background(), m.client, state)

	_, err := m.Advance(context.Background(), "spoke1")
	if err == nil {
		t.Fatal("Advance should block deletion when prior safeguards have no evidence")
	}
	if !strings.Contains(err.Error(), "cannot delete") {
		t.Errorf("expected 'cannot delete' error, got: %v", err)
	}

	state, _ = m.GetState(context.Background(), "spoke1")
	if state.Phase != PhaseDrained {
		t.Errorf("phase = %s, want drained (should remain unchanged)", state.Phase)
	}
}

func TestAdvanceDeleteBlocksWithoutBackup(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{Owner: "team@example.com"})

	state, _ := m.GetState(context.Background(), "spoke1")
	state.Phase = PhaseDrained
	state.NotifiedAt = "2026-09-18T12:00:00Z"
	// BackupPath deliberately empty
	setState(context.Background(), m.client, state)

	_, err := m.Advance(context.Background(), "spoke1")
	if err == nil {
		t.Fatal("Advance should block deletion when backup was never completed")
	}
	if !strings.Contains(err.Error(), "backup") {
		t.Errorf("expected backup-related error, got: %v", err)
	}
}

func TestAdvanceDeleteToCleanedPhase(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{Owner: "team@example.com"})

	state, _ := m.GetState(context.Background(), "spoke1")
	state.Phase = PhaseDeleted
	state.NotifiedAt = "2026-09-18T12:00:00Z"
	state.BackupPath = "/tmp/backup/spoke1"
	setState(context.Background(), m.client, state)

	state, err := m.Advance(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("Advance to cleaned: %v", err)
	}
	if state.Phase != PhaseCleaned {
		t.Errorf("phase = %s, want cleaned", state.Phase)
	}
}

func TestAdvanceDeleteHiveCluster(t *testing.T) {
	objs := setupCluster("spoke1")
	cd := clusterDeployment("spoke1")
	objs = append(objs, cd)
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{Owner: "team@example.com"})

	state, _ := m.GetState(context.Background(), "spoke1")
	state.Phase = PhaseDrained
	state.NotifiedAt = "2026-09-18T12:00:00Z"
	state.BackupPath = "/tmp/backup/spoke1"
	setState(context.Background(), m.client, state)

	state, err := m.Advance(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("Advance to deleted (Hive): %v", err)
	}
	if state.Phase != PhaseDeleted {
		t.Errorf("phase = %s, want deleted", state.Phase)
	}
}

func TestAdvanceNotifyError(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{Owner: "team@example.com"})

	state, err := m.Advance(context.Background(), "spoke1")
	if err == nil {
		t.Fatal("Advance should fail at unimplemented notify")
	}
	if state.Phase != PhaseAudited {
		t.Errorf("phase = %s, want audited (should remain unchanged)", state.Phase)
	}
}

func TestAdvanceBackupError(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{Owner: "team@example.com"})

	state, _ := m.GetState(context.Background(), "spoke1")
	state.Phase = PhaseNotified
	state.NotifiedAt = "2026-09-18T12:00:00Z"
	setState(context.Background(), m.client, state)

	state, err := m.Advance(context.Background(), "spoke1")
	if err == nil {
		t.Fatal("Advance should fail at unimplemented backup")
	}
	if state.Phase != PhaseNotified {
		t.Errorf("phase = %s, want notified (should remain unchanged)", state.Phase)
	}
}

func TestAdvanceDrainError(t *testing.T) {
	objs := setupCluster("spoke1")
	m := newTestManager(objs...)
	m.Start(context.Background(), "spoke1", StartOpts{Owner: "team@example.com"})

	state, _ := m.GetState(context.Background(), "spoke1")
	state.Phase = PhaseBackedUp
	state.BackupPath = "/tmp/backup"
	setState(context.Background(), m.client, state)

	state, err := m.Advance(context.Background(), "spoke1")
	if err == nil {
		t.Fatal("Advance should fail at unimplemented drain")
	}
	if state.Phase != PhaseBackedUp {
		t.Errorf("phase = %s, want backed-up (should remain unchanged)", state.Phase)
	}
}
