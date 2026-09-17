package migration

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
	client.GVRManifestWork: "ManifestWorkList",
	client.GVRConfigMap:    "ConfigMapList",
}

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrKinds, objs...)
	return &client.Client{Dynamic: fake}
}

func newManager(objs ...runtime.Object) *Manager {
	return New(fakeClient(objs...), config.Config{}, discardLogger)
}

func existingManifestWork(cluster, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": cluster,
			},
		},
	}
}

func appliedManifestWork(cluster, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": cluster,
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Applied",
						"status": "True",
					},
				},
			},
		},
	}
}

func migrationPlanCM(planID, source, target, phase string, workloads int) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "migration-plan-" + planID,
				"namespace": DefaultNamespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":        "true",
					"acmlab.redhat.com/migration-plan": "true",
				},
			},
			"data": map[string]interface{}{
				"planID":        planID,
				"sourceCluster": source,
				"targetCluster": target,
				"phase":         phase,
				"workloads":     fmt.Sprintf("%d", workloads),
				"namespaces":    "*",
				"createdAt":     "2026-09-17T10:00:00Z",
			},
		},
	}
}

func TestNewReturnsManager(t *testing.T) {
	mgr := newManager()
	if mgr == nil {
		t.Fatal("New returned nil")
	}
}

func TestPlanMigration(t *testing.T) {
	source := existingManifestWork("prod-east", "app-deploy")
	mgr := newManager(source)

	plan, err := mgr.PlanMigration(context.Background(), MigrationOpts{
		SourceCluster: "prod-east",
		TargetCluster: "prod-west",
	})
	if err != nil {
		t.Fatalf("PlanMigration failed: %v", err)
	}
	if plan.PlanID != "prod-east-to-prod-west" {
		t.Errorf("PlanID = %q", plan.PlanID)
	}
	if plan.Workloads != 1 {
		t.Errorf("Workloads = %d, want 1", plan.Workloads)
	}
	if plan.Phase != "Planned" {
		t.Errorf("Phase = %q, want Planned", plan.Phase)
	}
}

func TestPlanMigrationWithNamespaces(t *testing.T) {
	mgr := newManager()
	plan, err := mgr.PlanMigration(context.Background(), MigrationOpts{
		SourceCluster: "prod-east",
		TargetCluster: "prod-west",
		Namespaces:    []string{"app-ns", "data-ns"},
	})
	if err != nil {
		t.Fatalf("PlanMigration failed: %v", err)
	}
	if len(plan.Namespaces) != 2 {
		t.Errorf("Namespaces = %v, want 2 entries", plan.Namespaces)
	}
}

func TestPlanMigrationCountError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "manifestworks", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("timeout")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.PlanMigration(context.Background(), MigrationOpts{
		SourceCluster: "prod-east",
		TargetCluster: "prod-west",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "counting source workloads") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExecuteMigration(t *testing.T) {
	planID := "prod-east-to-prod-west"
	planCM := migrationPlanCM(planID, "prod-east", "prod-west", "Planned", 3)
	mgr := newManager(planCM)

	err := mgr.ExecuteMigration(context.Background(), planID)
	if err != nil {
		t.Fatalf("ExecuteMigration failed: %v", err)
	}

	_, err = mgr.client.Get(context.Background(), client.GVRManifestWork, "prod-east", "migration-cordon-"+planID)
	if err != nil {
		t.Errorf("cordon ManifestWork not found: %v", err)
	}
	_, err = mgr.client.Get(context.Background(), client.GVRManifestWork, "prod-west", "migration-workloads-"+planID)
	if err != nil {
		t.Errorf("target ManifestWork not found: %v", err)
	}
}

func TestExecuteMigrationPlanNotFound(t *testing.T) {
	mgr := newManager()
	err := mgr.ExecuteMigration(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "getting migration plan") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMigrationStatusPlanned(t *testing.T) {
	planID := "prod-east-to-prod-west"
	planCM := migrationPlanCM(planID, "prod-east", "prod-west", "Planned", 3)
	mgr := newManager(planCM)

	status, err := mgr.MigrationStatus(context.Background(), planID)
	if err != nil {
		t.Fatalf("MigrationStatus failed: %v", err)
	}
	if status.Phase != "Planned" {
		t.Errorf("Phase = %q, want Planned", status.Phase)
	}
	if status.Workloads != 3 {
		t.Errorf("Workloads = %d, want 3", status.Workloads)
	}
}

func TestMigrationStatusCompleted(t *testing.T) {
	planID := "prod-east-to-prod-west"
	planCM := migrationPlanCM(planID, "prod-east", "prod-west", "Verifying", 2)
	targetWork := appliedManifestWork("prod-west", "migration-workloads-"+planID)
	mgr := newManager(planCM, targetWork)

	status, err := mgr.MigrationStatus(context.Background(), planID)
	if err != nil {
		t.Fatalf("MigrationStatus failed: %v", err)
	}
	if status.Phase != "Completed" {
		t.Errorf("Phase = %q, want Completed", status.Phase)
	}
	if status.Migrated != 2 {
		t.Errorf("Migrated = %d, want 2", status.Migrated)
	}
}

func TestMigrationStatusNotFound(t *testing.T) {
	mgr := newManager()
	status, err := mgr.MigrationStatus(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("MigrationStatus failed: %v", err)
	}
	if status.Phase != "NotFound" {
		t.Errorf("Phase = %q, want NotFound", status.Phase)
	}
}

func TestRollbackMigration(t *testing.T) {
	planID := "prod-east-to-prod-west"
	planCM := migrationPlanCM(planID, "prod-east", "prod-west", "Deploying", 3)
	cordon := existingManifestWork("prod-east", "migration-cordon-"+planID)
	target := existingManifestWork("prod-west", "migration-workloads-"+planID)
	mgr := newManager(planCM, cordon, target)

	err := mgr.RollbackMigration(context.Background(), planID)
	if err != nil {
		t.Fatalf("RollbackMigration failed: %v", err)
	}

	obj, err := mgr.client.Get(context.Background(), client.GVRConfigMap, DefaultNamespace, "migration-plan-"+planID)
	if err != nil {
		t.Fatalf("plan ConfigMap not found: %v", err)
	}
	data, _ := obj.Object["data"].(map[string]interface{})
	if data["phase"] != "RolledBack" {
		t.Errorf("Phase = %q, want RolledBack", data["phase"])
	}
}

func TestRollbackMigrationPlanNotFound(t *testing.T) {
	mgr := newManager()
	err := mgr.RollbackMigration(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListMigrations(t *testing.T) {
	plan1 := migrationPlanCM("east-to-west", "prod-east", "prod-west", "Completed", 3)
	plan2 := migrationPlanCM("north-to-south", "prod-north", "prod-south", "Planned", 5)
	mgr := newManager(plan1, plan2)

	summaries, err := mgr.ListMigrations(context.Background())
	if err != nil {
		t.Fatalf("ListMigrations failed: %v", err)
	}
	if len(summaries) != 2 {
		t.Errorf("got %d migrations, want 2", len(summaries))
	}
}

func TestListMigrationsEmpty(t *testing.T) {
	mgr := newManager()
	summaries, err := mgr.ListMigrations(context.Background())
	if err != nil {
		t.Fatalf("ListMigrations failed: %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("got %d migrations, want 0", len(summaries))
	}
}

func TestListMigrationsError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "configmaps", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("timeout")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.ListMigrations(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMigrationPlanID(t *testing.T) {
	id := migrationPlanID("source", "target")
	if id != "source-to-target" {
		t.Errorf("migrationPlanID = %q, want source-to-target", id)
	}
}

func TestBuildMigrationPlanStructure(t *testing.T) {
	plan := &MigrationPlan{
		PlanID:        "test-plan",
		SourceCluster: "prod-east",
		TargetCluster: "prod-west",
		Workloads:     5,
		Namespaces:    []string{"app-ns", "data-ns"},
		Phase:         "Planned",
		CreatedAt:     "2026-09-17T10:00:00Z",
	}
	cm := buildMigrationPlan(plan)
	if cm.GetName() != "migration-plan-test-plan" {
		t.Errorf("Name = %q", cm.GetName())
	}
	labels := cm.GetLabels()
	if labels["acmlab.redhat.com/migration-plan"] != "true" {
		t.Error("missing migration-plan label")
	}
}

func TestBuildCordonLabelsStructure(t *testing.T) {
	work := buildCordonLabels("prod-east", "test-plan")
	if work.GetName() != "migration-cordon-test-plan" {
		t.Errorf("Name = %q", work.GetName())
	}
	labels := work.GetLabels()
	if labels["acmlab.redhat.com/migration"] != "draining" {
		t.Error("missing migration label")
	}
}

func TestBuildTargetManifestWorkStructure(t *testing.T) {
	plan := &MigrationPlan{
		PlanID:        "test-plan",
		SourceCluster: "prod-east",
		TargetCluster: "prod-west",
		Namespaces:    []string{"app-ns"},
	}
	work := buildTargetManifestWork(plan)
	if work.GetName() != "migration-workloads-test-plan" {
		t.Errorf("Name = %q", work.GetName())
	}
	if work.GetNamespace() != "prod-west" {
		t.Errorf("Namespace = %q", work.GetNamespace())
	}
}

func TestParseMigrationPlanFromConfigMap(t *testing.T) {
	cm := migrationPlanCM("test", "src", "tgt", "Planned", 7)
	plan := parseMigrationPlanFromConfigMap(cm.Object)
	if plan.PlanID != "test" {
		t.Errorf("PlanID = %q", plan.PlanID)
	}
	if plan.SourceCluster != "src" {
		t.Errorf("SourceCluster = %q", plan.SourceCluster)
	}
	if plan.Workloads != 7 {
		t.Errorf("Workloads = %d, want 7", plan.Workloads)
	}
}

func TestParseMigrationSummary(t *testing.T) {
	cm := migrationPlanCM("test", "src", "tgt", "Completed", 3)
	summary := parseMigrationSummary(cm.Object)
	if summary.PlanID != "test" {
		t.Errorf("PlanID = %q", summary.PlanID)
	}
	if summary.Phase != "Completed" {
		t.Errorf("Phase = %q", summary.Phase)
	}
}

func TestIsManifestWorkApplied(t *testing.T) {
	tests := []struct {
		name string
		obj  map[string]interface{}
		want bool
	}{
		{"no status", map[string]interface{}{}, false},
		{"applied", map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{"type": "Applied", "status": "True"},
				},
			},
		}, true},
		{"not applied", map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{"type": "Applied", "status": "False"},
				},
			},
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isManifestWorkApplied(tt.obj)
			if got != tt.want {
				t.Errorf("isManifestWorkApplied = %v, want %v", got, tt.want)
			}
		})
	}
}
