package backup

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

var gvrKinds = map[schema.GroupVersionResource]string{
	client.GVRBackupSchedule: "BackupScheduleList",
	client.GVRRestore:        "RestoreList",
}

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrKinds, objs...)
	return &client.Client{Dynamic: fake}
}

func newManager(objs ...runtime.Object) *Manager {
	return New(fakeClient(objs...), config.Config{}, discardLogger)
}

func backupScheduleObj(ns, schedule, ttl string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta1",
			"kind":       "BackupSchedule",
			"metadata": map[string]interface{}{
				"name":      "acm-backup-schedule",
				"namespace": ns,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
					"acmlab.redhat.com/backup":  "true",
				},
			},
			"spec": map[string]interface{}{
				"veleroSchedule":        schedule,
				"veleroTtl":             ttl,
				"veleroStorageLocation": "default",
			},
			"status": map[string]interface{}{
				"phase":               "Enabled",
				"lastBackupTimestamp": "2026-09-16T12:00:00Z",
				"lastBackupStatus":   "Completed",
			},
		},
	}
}

func restoreObj(ns, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta1",
			"kind":       "Restore",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
					"acmlab.redhat.com/backup":  "true",
				},
			},
			"spec": map[string]interface{}{
				"veleroManagedClustersBackupName": "latest",
				"veleroCredentialsBackupName":     "latest",
				"veleroResourcesBackupName":       "latest",
			},
			"status": map[string]interface{}{
				"phase":          "Completed",
				"startTimestamp": "2026-09-16T12:30:00Z",
			},
		},
	}
}

func TestNewReturnsManager(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)
	if mgr == nil {
		t.Fatal("New returned nil")
	}
}

func TestEnable(t *testing.T) {
	mgr := newManager()
	err := mgr.Enable(context.Background(), BackupOpts{})
	if err != nil {
		t.Fatalf("Enable failed: %v", err)
	}

	obj, err := mgr.client.Get(context.Background(), client.GVRBackupSchedule, DefaultNamespace, "acm-backup-schedule")
	if err != nil {
		t.Fatalf("BackupSchedule not found: %v", err)
	}

	schedule, _, _ := unstructured.NestedString(obj.Object, "spec", "veleroSchedule")
	if schedule != "0 */6 * * *" {
		t.Errorf("schedule = %q, want '0 */6 * * *'", schedule)
	}
}

func TestEnableIdempotent(t *testing.T) {
	mgr := newManager()
	if err := mgr.Enable(context.Background(), BackupOpts{}); err != nil {
		t.Fatalf("first enable: %v", err)
	}
	if err := mgr.Enable(context.Background(), BackupOpts{}); err != nil {
		t.Fatalf("second enable should be idempotent: %v", err)
	}
}

func TestEnableWithCustomOpts(t *testing.T) {
	mgr := newManager()
	err := mgr.Enable(context.Background(), BackupOpts{
		Namespace:       "custom-ns",
		Schedule:        "0 0 * * *",
		VeleroTTL:       "168h",
		StorageLocation: "s3-bucket",
	})
	if err != nil {
		t.Fatalf("Enable failed: %v", err)
	}

	obj, err := mgr.client.Get(context.Background(), client.GVRBackupSchedule, "custom-ns", "acm-backup-schedule")
	if err != nil {
		t.Fatalf("BackupSchedule not found: %v", err)
	}

	schedule, _, _ := unstructured.NestedString(obj.Object, "spec", "veleroSchedule")
	if schedule != "0 0 * * *" {
		t.Errorf("schedule = %q, want '0 0 * * *'", schedule)
	}

	ttl, _, _ := unstructured.NestedString(obj.Object, "spec", "veleroTtl")
	if ttl != "168h" {
		t.Errorf("ttl = %q, want 168h", ttl)
	}

	loc, _, _ := unstructured.NestedString(obj.Object, "spec", "veleroStorageLocation")
	if loc != "s3-bucket" {
		t.Errorf("storageLocation = %q, want s3-bucket", loc)
	}
}

func TestEnableDefaults(t *testing.T) {
	mgr := newManager()
	opts := BackupOpts{}
	mgr.applyDefaults(&opts)

	if opts.Namespace != DefaultNamespace {
		t.Errorf("namespace = %q, want %s", opts.Namespace, DefaultNamespace)
	}
	if opts.Schedule != "0 */6 * * *" {
		t.Errorf("schedule = %q, want '0 */6 * * *'", opts.Schedule)
	}
	if opts.VeleroTTL != "720h" {
		t.Errorf("ttl = %q, want 720h", opts.VeleroTTL)
	}
	if opts.StorageLocation != "default" {
		t.Errorf("storageLocation = %q, want default", opts.StorageLocation)
	}
}

func TestEnableError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "backupschedules", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("forbidden")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.Enable(context.Background(), BackupOpts{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "creating BackupSchedule") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDisable(t *testing.T) {
	existing := backupScheduleObj(DefaultNamespace, "0 */6 * * *", "720h")
	mgr := newManager(existing)

	removed, err := mgr.Disable(context.Background(), "")
	if err != nil {
		t.Fatalf("Disable failed: %v", err)
	}
	if !removed {
		t.Error("Disable should return true for existing schedule")
	}

	_, err = mgr.client.Get(context.Background(), client.GVRBackupSchedule, DefaultNamespace, "acm-backup-schedule")
	if err == nil {
		t.Error("BackupSchedule should not exist after disable")
	}
}

func TestDisableNotFound(t *testing.T) {
	mgr := newManager()
	removed, err := mgr.Disable(context.Background(), "")
	if err != nil {
		t.Fatalf("Disable failed: %v", err)
	}
	if removed {
		t.Error("Disable should return false for nonexistent schedule")
	}
}

func TestDisableDeleteError(t *testing.T) {
	existing := backupScheduleObj(DefaultNamespace, "0 */6 * * *", "720h")
	mgr := newManager(existing)

	fake := mgr.client.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("delete", "backupschedules", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("delete blocked")
	})

	_, err := mgr.Disable(context.Background(), "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "deleting BackupSchedule") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetStatus(t *testing.T) {
	existing := backupScheduleObj(DefaultNamespace, "0 */6 * * *", "720h")
	mgr := newManager(existing)

	status, err := mgr.GetStatus(context.Background(), "")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if !status.Enabled {
		t.Error("Enabled should be true")
	}
	if status.Schedule != "0 */6 * * *" {
		t.Errorf("Schedule = %q", status.Schedule)
	}
	if status.Phase != "Enabled" {
		t.Errorf("Phase = %q", status.Phase)
	}
	if status.LastBackup != "2026-09-16T12:00:00Z" {
		t.Errorf("LastBackup = %q", status.LastBackup)
	}
	if status.LastStatus != "Completed" {
		t.Errorf("LastStatus = %q", status.LastStatus)
	}
}

func TestGetStatusNotEnabled(t *testing.T) {
	mgr := newManager()
	status, err := mgr.GetStatus(context.Background(), "")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if status.Enabled {
		t.Error("Enabled should be false when no schedule exists")
	}
}

func TestGetStatusError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("get", "backupschedules", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("timeout")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.GetStatus(context.Background(), "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "getting BackupSchedule") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestListBackups(t *testing.T) {
	r1 := restoreObj(DefaultNamespace, "acm-restore-1726480200")
	r2 := restoreObj(DefaultNamespace, "acm-restore-1726483800")
	mgr := newManager(r1, r2)

	infos, err := mgr.ListBackups(context.Background(), "")
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}
	if len(infos) != 2 {
		t.Errorf("got %d backups, want 2", len(infos))
	}
}

func TestListBackupsEmpty(t *testing.T) {
	mgr := newManager()
	infos, err := mgr.ListBackups(context.Background(), "")
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("got %d backups, want 0", len(infos))
	}
}

func TestListBackupsError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "restores", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("timeout")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.ListBackups(context.Background(), "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "listing backups") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRestore(t *testing.T) {
	mgr := newManager()
	err := mgr.Restore(context.Background(), RestoreOpts{
		BackupName: "test-backup",
	})
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	list, err := mgr.client.List(context.Background(), client.GVRRestore, DefaultNamespace, "")
	if err != nil {
		t.Fatalf("List restores failed: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 restore, got %d", len(list.Items))
	}

	syncMode, _, _ := unstructured.NestedString(list.Items[0].Object, "spec", "veleroManagedClustersBackupName")
	if syncMode != "latest" {
		t.Errorf("syncMode = %q, want latest", syncMode)
	}
}

func TestRestoreWithSyncMode(t *testing.T) {
	mgr := newManager()
	err := mgr.Restore(context.Background(), RestoreOpts{
		SyncMode: "skip",
	})
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	list, err := mgr.client.List(context.Background(), client.GVRRestore, DefaultNamespace, "")
	if err != nil {
		t.Fatalf("List restores failed: %v", err)
	}
	syncMode, _, _ := unstructured.NestedString(list.Items[0].Object, "spec", "veleroManagedClustersBackupName")
	if syncMode != "skip" {
		t.Errorf("syncMode = %q, want skip", syncMode)
	}
}

func TestRestoreError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "restores", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("forbidden")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.Restore(context.Background(), RestoreOpts{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "creating Restore") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuildBackupScheduleStructure(t *testing.T) {
	bs := buildBackupSchedule(BackupOpts{
		Namespace:       "test-ns",
		Schedule:        "0 0 * * *",
		VeleroTTL:       "168h",
		StorageLocation: "s3",
	})

	if bs.GetName() != "acm-backup-schedule" {
		t.Errorf("Name = %q", bs.GetName())
	}
	if bs.GetNamespace() != "test-ns" {
		t.Errorf("Namespace = %q", bs.GetNamespace())
	}

	labels := bs.GetLabels()
	if labels["acmlab.redhat.com/backup"] != "true" {
		t.Error("missing backup label")
	}

	schedule, _, _ := unstructured.NestedString(bs.Object, "spec", "veleroSchedule")
	if schedule != "0 0 * * *" {
		t.Errorf("veleroSchedule = %q", schedule)
	}
}

func TestBuildRestoreStructure(t *testing.T) {
	r := buildRestore(RestoreOpts{
		Namespace:  "test-ns",
		BackupName: "my-backup",
		SyncMode:   "skip",
	})

	if r.GetName() != "acm-restore-my-backup" {
		t.Errorf("Name = %q, want acm-restore-my-backup", r.GetName())
	}
	if r.GetNamespace() != "test-ns" {
		t.Errorf("Namespace = %q", r.GetNamespace())
	}

	syncMode, _, _ := unstructured.NestedString(r.Object, "spec", "veleroManagedClustersBackupName")
	if syncMode != "skip" {
		t.Errorf("syncMode = %q, want skip", syncMode)
	}
}

func TestBuildRestoreAutoName(t *testing.T) {
	r := buildRestore(RestoreOpts{
		Namespace: "test-ns",
		SyncMode:  "latest",
	})

	if !strings.HasPrefix(r.GetName(), "acm-restore-") {
		t.Errorf("Name = %q, should have acm-restore- prefix", r.GetName())
	}
}

func TestParseBackupStatusNoStatus(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"veleroSchedule": "0 */6 * * *",
		},
	}
	bs := parseBackupStatus(obj)
	if !bs.Enabled {
		t.Error("Enabled should be true")
	}
	if bs.Schedule != "0 */6 * * *" {
		t.Errorf("Schedule = %q", bs.Schedule)
	}
	if bs.Phase != "" {
		t.Errorf("Phase = %q, want empty", bs.Phase)
	}
}

func TestParseBackupInfoComplete(t *testing.T) {
	r := restoreObj(DefaultNamespace, "acm-restore-test")
	info := parseBackupInfo(r.Object)
	if info.Name != "acm-restore-test" {
		t.Errorf("Name = %q", info.Name)
	}
	if info.Phase != "Completed" {
		t.Errorf("Phase = %q", info.Phase)
	}
	if info.StartTime != "2026-09-16T12:30:00Z" {
		t.Errorf("StartTime = %q", info.StartTime)
	}
}

func TestParseBackupInfoEmpty(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "test",
		},
	}
	info := parseBackupInfo(obj)
	if info.Name != "test" {
		t.Errorf("Name = %q", info.Name)
	}
	if info.Phase != "" {
		t.Errorf("Phase = %q, want empty", info.Phase)
	}
}

func TestRestoreName(t *testing.T) {
	name := restoreName()
	if !strings.HasPrefix(name, "acm-restore-") {
		t.Errorf("restoreName = %q, should start with acm-restore-", name)
	}
}
