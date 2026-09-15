package importing

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

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRNamespace:              "NamespaceList",
			client.GVRManagedCluster:         "ManagedClusterList",
			client.GVRKlusterletAddonConfig:  "KlusterletAddonConfigList",
			client.GVRSecret:                 "SecretList",
			client.GVRClusterDeployment:      "ClusterDeploymentList",
		}, objs...)
	return &client.Client{Dynamic: fake}
}

func newManager(objs ...runtime.Object) *Manager {
	return New(fakeClient(objs...), config.Config{}, discardLogger)
}

func makeManagedCluster(name string, labels, annotations map[string]string, available, joined string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	obj.SetName(name)
	if labels != nil {
		obj.SetLabels(labels)
	}
	if annotations != nil {
		obj.SetAnnotations(annotations)
	}
	if available != "" || joined != "" {
		var conds []interface{}
		if available != "" {
			conds = append(conds, map[string]interface{}{
				"type":   "ManagedClusterConditionAvailable",
				"status": available,
			})
		}
		if joined != "" {
			conds = append(conds, map[string]interface{}{
				"type":   "ManagedClusterJoined",
				"status": joined,
			})
		}
		obj.Object["status"] = map[string]interface{}{
			"conditions": conds,
		}
	}
	return obj
}

func makeNamespace(name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"})
	obj.SetName(name)
	return obj
}

func makeSecret(namespace, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Secret"})
	obj.SetName(name)
	obj.SetNamespace(namespace)
	return obj
}

func makeClusterDeployment(namespace, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "hive.openshift.io", Version: "v1", Kind: "ClusterDeployment",
	})
	obj.SetName(name)
	obj.SetNamespace(namespace)
	return obj
}

func makeKlusterletAddonConfig(namespace, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "agent.open-cluster-management.io", Version: "v1", Kind: "KlusterletAddonConfig",
	})
	obj.SetName(name)
	obj.SetNamespace(namespace)
	return obj
}

// ---------------------------------------------------------------------------
// extractConditions
// ---------------------------------------------------------------------------

func TestExtractConditionsReturnsAvailableAndJoined(t *testing.T) {
	mc := makeManagedCluster("test", nil, nil, "True", "True")
	available, joined := extractConditions(mc)
	if available != "True" {
		t.Errorf("expected available=True, got %s", available)
	}
	if joined != "True" {
		t.Errorf("expected joined=True, got %s", joined)
	}
}

func TestExtractConditionsReturnsEmptyWhenMissing(t *testing.T) {
	mc := &unstructured.Unstructured{Object: map[string]interface{}{}}
	available, joined := extractConditions(mc)
	if available != "" {
		t.Errorf("expected available='', got %s", available)
	}
	if joined != "" {
		t.Errorf("expected joined='', got %s", joined)
	}
}

func TestExtractConditionsUnknownState(t *testing.T) {
	mc := makeManagedCluster("test", nil, nil, "Unknown", "True")
	available, joined := extractConditions(mc)
	if available != "Unknown" {
		t.Errorf("expected available=Unknown, got %s", available)
	}
	if joined != "True" {
		t.Errorf("expected joined=True, got %s", joined)
	}
}

func TestExtractConditionsSkipsBadConditionType(t *testing.T) {
	mc := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					"not-a-map",
					map[string]interface{}{
						"type":   "ManagedClusterConditionAvailable",
						"status": "True",
					},
				},
			},
		},
	}
	available, joined := extractConditions(mc)
	if available != "True" {
		t.Errorf("expected available=True, got %s", available)
	}
	if joined != "" {
		t.Errorf("expected joined='', got %s", joined)
	}
}

// ---------------------------------------------------------------------------
// New
// ---------------------------------------------------------------------------

func TestNewReturnsManager(t *testing.T) {
	c := fakeClient()
	cfg := config.Config{}
	m := New(c, cfg, discardLogger)
	if m == nil {
		t.Fatal("expected non-nil Manager")
	}
	if m.client != c {
		t.Error("client mismatch")
	}
}

// ---------------------------------------------------------------------------
// Import (success paths)
// ---------------------------------------------------------------------------

func TestImportWithKubeconfig(t *testing.T) {
	m := newManager()
	ctx := context.Background()

	result, err := m.Import(ctx, ImportOptions{
		Name:       "spoke-1",
		Kubeconfig: []byte("fake-kubeconfig-data"),
		Labels:     map[string]string{"cloud": "AWS"},
		ClusterSet: "production",
	})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if result.Name != "spoke-1" {
		t.Errorf("name = %q, want spoke-1", result.Name)
	}
	if !result.AutoImport {
		t.Error("expected AutoImport=true when kubeconfig provided")
	}
	if result.Message != "Cluster spoke-1 registered for import (auto-import enabled)" {
		t.Errorf("unexpected message: %s", result.Message)
	}

	// Verify the ManagedCluster was created
	mc, err := m.client.Get(ctx, client.GVRManagedCluster, "", "spoke-1")
	if err != nil {
		t.Fatalf("ManagedCluster not found: %v", err)
	}
	labels, _, _ := unstructured.NestedStringMap(mc.Object, "metadata", "labels")
	if labels["cloud"] != "AWS" {
		t.Errorf("label cloud = %q, want AWS", labels["cloud"])
	}
	if labels["cluster.open-cluster-management.io/clusterset"] != "production" {
		t.Errorf("clusterset = %q, want production", labels["cluster.open-cluster-management.io/clusterset"])
	}

	// Verify auto-import secret was created
	_, err = m.client.Get(ctx, client.GVRSecret, "spoke-1", "auto-import-secret")
	if err != nil {
		t.Fatalf("auto-import secret not found: %v", err)
	}
}

func TestImportWithoutKubeconfig(t *testing.T) {
	m := newManager()
	ctx := context.Background()

	result, err := m.Import(ctx, ImportOptions{
		Name: "spoke-2",
	})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if result.AutoImport {
		t.Error("expected AutoImport=false when no kubeconfig")
	}
	if result.Message != "Cluster spoke-2 registered for import — apply import manifests manually on the spoke" {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

func TestImportDefaultClusterSet(t *testing.T) {
	m := newManager()
	ctx := context.Background()

	_, err := m.Import(ctx, ImportOptions{Name: "spoke-default"})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	mc, err := m.client.Get(ctx, client.GVRManagedCluster, "", "spoke-default")
	if err != nil {
		t.Fatalf("ManagedCluster not found: %v", err)
	}
	labels, _, _ := unstructured.NestedStringMap(mc.Object, "metadata", "labels")
	if labels["cluster.open-cluster-management.io/clusterset"] != "default" {
		t.Errorf("expected default clusterset, got %q", labels["cluster.open-cluster-management.io/clusterset"])
	}
}

func TestImportWithExistingNamespace(t *testing.T) {
	m := newManager(makeNamespace("existing-ns"))
	ctx := context.Background()

	result, err := m.Import(ctx, ImportOptions{Name: "existing-ns"})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if result.Name != "existing-ns" {
		t.Errorf("name = %q, want existing-ns", result.Name)
	}
}

func TestImportWithExistingManagedCluster(t *testing.T) {
	mc := makeManagedCluster("existing-mc", nil, nil, "", "")
	m := newManager(mc)
	ctx := context.Background()

	result, err := m.Import(ctx, ImportOptions{Name: "existing-mc"})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if result.Name != "existing-mc" {
		t.Errorf("name = %q, want existing-mc", result.Name)
	}
}

func TestImportWithExistingKlusterletAddonConfig(t *testing.T) {
	kac := makeKlusterletAddonConfig("existing-kac", "existing-kac")
	ns := makeNamespace("existing-kac")
	m := newManager(kac, ns)
	ctx := context.Background()

	result, err := m.Import(ctx, ImportOptions{Name: "existing-kac"})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if result.Name != "existing-kac" {
		t.Errorf("name = %q, want existing-kac", result.Name)
	}
}

func TestImportWithExistingAutoImportSecretUpdates(t *testing.T) {
	existingSecret := makeSecret("spoke-update", "auto-import-secret")
	existingSecret.Object["data"] = map[string]interface{}{
		"kubeconfig": "old-data",
	}
	existingSecret.Object["type"] = "Opaque"
	ns := makeNamespace("spoke-update")
	m := newManager(existingSecret, ns)
	ctx := context.Background()

	result, err := m.Import(ctx, ImportOptions{
		Name:       "spoke-update",
		Kubeconfig: []byte("new-kubeconfig"),
	})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if !result.AutoImport {
		t.Error("expected AutoImport=true")
	}
}

func TestImportWithMultipleLabels(t *testing.T) {
	m := newManager()
	ctx := context.Background()

	_, err := m.Import(ctx, ImportOptions{
		Name: "multi-label",
		Labels: map[string]string{
			"cloud":       "IBM",
			"vendor":      "OpenShift",
			"environment": "staging",
		},
		ClusterSet: "dev",
	})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	mc, err := m.client.Get(ctx, client.GVRManagedCluster, "", "multi-label")
	if err != nil {
		t.Fatalf("ManagedCluster not found: %v", err)
	}
	labels, _, _ := unstructured.NestedStringMap(mc.Object, "metadata", "labels")
	if labels["cloud"] != "IBM" {
		t.Errorf("cloud = %q, want IBM", labels["cloud"])
	}
	if labels["vendor"] != "OpenShift" {
		t.Errorf("vendor = %q, want OpenShift", labels["vendor"])
	}
	if labels["environment"] != "staging" {
		t.Errorf("environment = %q, want staging", labels["environment"])
	}
	if labels["name"] != "multi-label" {
		t.Errorf("name label = %q, want multi-label", labels["name"])
	}
	if labels["cluster.open-cluster-management.io/clusterset"] != "dev" {
		t.Errorf("clusterset = %q, want dev", labels["cluster.open-cluster-management.io/clusterset"])
	}
}

// ---------------------------------------------------------------------------
// Detach
// ---------------------------------------------------------------------------

func TestDetachSuccess(t *testing.T) {
	mc := makeManagedCluster("spoke-detach", nil, nil, "True", "True")
	m := newManager(mc)
	ctx := context.Background()

	err := m.Detach(ctx, "spoke-detach")
	if err != nil {
		t.Fatalf("Detach failed: %v", err)
	}

	// Verify it's gone
	_, err = m.client.Get(ctx, client.GVRManagedCluster, "", "spoke-detach")
	if err == nil {
		t.Error("expected ManagedCluster to be deleted")
	}
}

func TestDetachNotFoundIsNoop(t *testing.T) {
	m := newManager()
	ctx := context.Background()

	err := m.Detach(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Detach should not error on not-found, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetImportStatus
// ---------------------------------------------------------------------------

func TestGetImportStatusSuccess(t *testing.T) {
	mc := makeManagedCluster("status-cluster",
		map[string]string{"cloud": "AWS", "name": "status-cluster"},
		map[string]string{"open-cluster-management/created-via": "discovery"},
		"True", "True",
	)
	secret := makeSecret("status-cluster", "auto-import-secret")
	m := newManager(mc, secret)
	ctx := context.Background()

	status, err := m.GetImportStatus(ctx, "status-cluster")
	if err != nil {
		t.Fatalf("GetImportStatus failed: %v", err)
	}
	if status.Name != "status-cluster" {
		t.Errorf("name = %q, want status-cluster", status.Name)
	}
	if status.Available != "True" {
		t.Errorf("available = %q, want True", status.Available)
	}
	if status.Joined != "True" {
		t.Errorf("joined = %q, want True", status.Joined)
	}
	if status.CreatedVia != "discovery" {
		t.Errorf("createdVia = %q, want discovery", status.CreatedVia)
	}
	if !status.AutoImport {
		t.Error("expected AutoImport=true when secret exists")
	}
	if status.Labels["cloud"] != "AWS" {
		t.Errorf("label cloud = %q, want AWS", status.Labels["cloud"])
	}
}

func TestGetImportStatusWithoutAutoImportSecret(t *testing.T) {
	mc := makeManagedCluster("no-secret", nil, nil, "True", "True")
	m := newManager(mc)
	ctx := context.Background()

	status, err := m.GetImportStatus(ctx, "no-secret")
	if err != nil {
		t.Fatalf("GetImportStatus failed: %v", err)
	}
	if status.AutoImport {
		t.Error("expected AutoImport=false when no secret")
	}
}

func TestGetImportStatusWithoutAnnotations(t *testing.T) {
	mc := makeManagedCluster("no-annot", nil, nil, "False", "")
	m := newManager(mc)
	ctx := context.Background()

	status, err := m.GetImportStatus(ctx, "no-annot")
	if err != nil {
		t.Fatalf("GetImportStatus failed: %v", err)
	}
	if status.CreatedVia != "" {
		t.Errorf("createdVia = %q, want empty", status.CreatedVia)
	}
	if status.Available != "False" {
		t.Errorf("available = %q, want False", status.Available)
	}
}

func TestGetImportStatusNotFound(t *testing.T) {
	m := newManager()
	ctx := context.Background()

	_, err := m.GetImportStatus(ctx, "missing-cluster")
	if err == nil {
		t.Error("expected error for missing cluster")
	}
}

// ---------------------------------------------------------------------------
// IsImported
// ---------------------------------------------------------------------------

func TestIsImportedTrueWhenNoClusterDeployment(t *testing.T) {
	mc := makeManagedCluster("imported", nil, nil, "True", "True")
	m := newManager(mc)
	ctx := context.Background()

	imported, err := m.IsImported(ctx, "imported")
	if err != nil {
		t.Fatalf("IsImported failed: %v", err)
	}
	if !imported {
		t.Error("expected imported=true when no ClusterDeployment")
	}
}

func TestIsImportedFalseWhenClusterDeploymentExists(t *testing.T) {
	cd := makeClusterDeployment("provisioned", "provisioned")
	m := newManager(cd)
	ctx := context.Background()

	imported, err := m.IsImported(ctx, "provisioned")
	if err != nil {
		t.Fatalf("IsImported failed: %v", err)
	}
	if imported {
		t.Error("expected imported=false when ClusterDeployment exists")
	}
}

// ---------------------------------------------------------------------------
// ListImported
// ---------------------------------------------------------------------------

func TestListImportedFiltersProvisionedClusters(t *testing.T) {
	mc1 := makeManagedCluster("imported-1",
		map[string]string{"cloud": "AWS"},
		map[string]string{"open-cluster-management/created-via": "discovery"},
		"True", "True",
	)
	mc2 := makeManagedCluster("provisioned-1", nil, nil, "True", "True")
	cd := makeClusterDeployment("provisioned-1", "provisioned-1")
	m := newManager(mc1, mc2, cd)
	ctx := context.Background()

	imported, err := m.ListImported(ctx)
	if err != nil {
		t.Fatalf("ListImported failed: %v", err)
	}
	if len(imported) != 1 {
		t.Fatalf("got %d imported clusters, want 1", len(imported))
	}
	if imported[0].Name != "imported-1" {
		t.Errorf("name = %q, want imported-1", imported[0].Name)
	}
	if imported[0].Available != "True" {
		t.Errorf("available = %q, want True", imported[0].Available)
	}
	if imported[0].CreatedVia != "discovery" {
		t.Errorf("createdVia = %q, want discovery", imported[0].CreatedVia)
	}
	if imported[0].Labels["cloud"] != "AWS" {
		t.Errorf("label cloud = %q, want AWS", imported[0].Labels["cloud"])
	}
}

func TestListImportedSkipsLocalCluster(t *testing.T) {
	local := makeManagedCluster("local-cluster", nil, nil, "True", "True")
	other := makeManagedCluster("external", nil, nil, "True", "True")
	m := newManager(local, other)
	ctx := context.Background()

	imported, err := m.ListImported(ctx)
	if err != nil {
		t.Fatalf("ListImported failed: %v", err)
	}
	if len(imported) != 1 {
		t.Fatalf("got %d imported, want 1 (local-cluster should be skipped)", len(imported))
	}
	if imported[0].Name != "external" {
		t.Errorf("name = %q, want external", imported[0].Name)
	}
}

func TestListImportedReturnsEmptyWhenNoClusters(t *testing.T) {
	m := newManager()
	ctx := context.Background()

	imported, err := m.ListImported(ctx)
	if err != nil {
		t.Fatalf("ListImported failed: %v", err)
	}
	if len(imported) != 0 {
		t.Errorf("got %d imported, want 0", len(imported))
	}
}

func TestListImportedMultipleClusters(t *testing.T) {
	mc1 := makeManagedCluster("cluster-a", nil, nil, "True", "True")
	mc2 := makeManagedCluster("cluster-b", nil, nil, "False", "True")
	mc3 := makeManagedCluster("cluster-c", nil, nil, "True", "False")
	m := newManager(mc1, mc2, mc3)
	ctx := context.Background()

	imported, err := m.ListImported(ctx)
	if err != nil {
		t.Fatalf("ListImported failed: %v", err)
	}
	if len(imported) != 3 {
		t.Fatalf("got %d imported, want 3", len(imported))
	}

	nameSet := map[string]bool{}
	for _, s := range imported {
		nameSet[s.Name] = true
	}
	for _, name := range []string{"cluster-a", "cluster-b", "cluster-c"} {
		if !nameSet[name] {
			t.Errorf("expected %s in list", name)
		}
	}
}

func TestListImportedWithAnnotations(t *testing.T) {
	mc := makeManagedCluster("annot-cluster", nil,
		map[string]string{"open-cluster-management/created-via": "other"},
		"True", "True",
	)
	m := newManager(mc)
	ctx := context.Background()

	imported, err := m.ListImported(ctx)
	if err != nil {
		t.Fatalf("ListImported failed: %v", err)
	}
	if len(imported) != 1 {
		t.Fatalf("got %d, want 1", len(imported))
	}
	if imported[0].CreatedVia != "other" {
		t.Errorf("createdVia = %q, want other", imported[0].CreatedVia)
	}
}

// ---------------------------------------------------------------------------
// WaitForImport
// ---------------------------------------------------------------------------

func TestWaitForImportSucceedsOnFirstTick(t *testing.T) {
	mc := makeManagedCluster("ready-cluster", nil, nil, "True", "True")
	m := newManager(mc)
	ctx := context.Background()

	// The ticker inside WaitForImport fires every 10s, so we need
	// a timeout larger than that to allow the first tick to check conditions.
	err := m.WaitForImport(ctx, "ready-cluster", 15*time.Second)
	if err != nil {
		t.Fatalf("WaitForImport failed: %v", err)
	}
}

func TestWaitForImportTimesOut(t *testing.T) {
	mc := makeManagedCluster("not-ready", nil, nil, "False", "False")
	m := newManager(mc)
	ctx := context.Background()

	// Use a very short timeout so the deadline fires before the first tick
	err := m.WaitForImport(ctx, "not-ready", 1*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestWaitForImportContextCancelled(t *testing.T) {
	mc := makeManagedCluster("pending", nil, nil, "False", "False")
	m := newManager(mc)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := m.WaitForImport(ctx, "pending", 30*time.Second)
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

// ---------------------------------------------------------------------------
// ensureNamespace (tested through Import, also directly)
// ---------------------------------------------------------------------------

func TestEnsureNamespaceCreatesNew(t *testing.T) {
	m := newManager()
	ctx := context.Background()

	err := m.ensureNamespace(ctx, "new-ns")
	if err != nil {
		t.Fatalf("ensureNamespace failed: %v", err)
	}

	_, err = m.client.Get(ctx, client.GVRNamespace, "", "new-ns")
	if err != nil {
		t.Fatalf("namespace not created: %v", err)
	}
}

func TestEnsureNamespaceAlreadyExists(t *testing.T) {
	m := newManager(makeNamespace("existing"))
	ctx := context.Background()

	err := m.ensureNamespace(ctx, "existing")
	if err != nil {
		t.Fatalf("ensureNamespace should not fail on existing ns: %v", err)
	}
}

// ---------------------------------------------------------------------------
// createManagedCluster (tested through Import, also directly)
// ---------------------------------------------------------------------------

func TestCreateManagedClusterWithClusterSet(t *testing.T) {
	m := newManager()
	ctx := context.Background()

	err := m.createManagedCluster(ctx, "with-set", map[string]string{"env": "prod"}, "my-set")
	if err != nil {
		t.Fatalf("createManagedCluster failed: %v", err)
	}

	mc, err := m.client.Get(ctx, client.GVRManagedCluster, "", "with-set")
	if err != nil {
		t.Fatalf("ManagedCluster not found: %v", err)
	}
	labels, _, _ := unstructured.NestedStringMap(mc.Object, "metadata", "labels")
	if labels["cluster.open-cluster-management.io/clusterset"] != "my-set" {
		t.Errorf("clusterset = %q, want my-set", labels["cluster.open-cluster-management.io/clusterset"])
	}
	if labels["env"] != "prod" {
		t.Errorf("env = %q, want prod", labels["env"])
	}
}

func TestCreateManagedClusterDefaultClusterSet(t *testing.T) {
	m := newManager()
	ctx := context.Background()

	err := m.createManagedCluster(ctx, "default-set", nil, "")
	if err != nil {
		t.Fatalf("createManagedCluster failed: %v", err)
	}

	mc, err := m.client.Get(ctx, client.GVRManagedCluster, "", "default-set")
	if err != nil {
		t.Fatalf("ManagedCluster not found: %v", err)
	}
	labels, _, _ := unstructured.NestedStringMap(mc.Object, "metadata", "labels")
	if labels["cluster.open-cluster-management.io/clusterset"] != "default" {
		t.Errorf("clusterset = %q, want default", labels["cluster.open-cluster-management.io/clusterset"])
	}
}

func TestCreateManagedClusterAlreadyExists(t *testing.T) {
	mc := makeManagedCluster("existing-mc", nil, nil, "", "")
	m := newManager(mc)
	ctx := context.Background()

	err := m.createManagedCluster(ctx, "existing-mc", nil, "")
	if err != nil {
		t.Fatalf("expected no error on already-exists, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// createKlusterletAddonConfig
// ---------------------------------------------------------------------------

func TestCreateKlusterletAddonConfigSuccess(t *testing.T) {
	m := newManager()
	ctx := context.Background()

	err := m.createKlusterletAddonConfig(ctx, "kac-test")
	if err != nil {
		t.Fatalf("createKlusterletAddonConfig failed: %v", err)
	}

	_, err = m.client.Get(ctx, client.GVRKlusterletAddonConfig, "kac-test", "kac-test")
	if err != nil {
		t.Fatalf("KlusterletAddonConfig not found: %v", err)
	}
}

func TestCreateKlusterletAddonConfigAlreadyExists(t *testing.T) {
	kac := makeKlusterletAddonConfig("existing-kac", "existing-kac")
	m := newManager(kac)
	ctx := context.Background()

	err := m.createKlusterletAddonConfig(ctx, "existing-kac")
	if err != nil {
		t.Fatalf("expected no error on already-exists, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// createAutoImportSecret / updateAutoImportSecret
// ---------------------------------------------------------------------------

func TestCreateAutoImportSecretSuccess(t *testing.T) {
	m := newManager()
	ctx := context.Background()

	err := m.createAutoImportSecret(ctx, "secret-test", []byte("kubeconfig-data"))
	if err != nil {
		t.Fatalf("createAutoImportSecret failed: %v", err)
	}

	secret, err := m.client.Get(ctx, client.GVRSecret, "secret-test", "auto-import-secret")
	if err != nil {
		t.Fatalf("secret not found: %v", err)
	}
	data, _, _ := unstructured.NestedString(secret.Object, "data", "kubeconfig")
	if data == "" {
		t.Error("expected kubeconfig data in secret")
	}
}

func TestCreateAutoImportSecretAlreadyExistsUpdates(t *testing.T) {
	existing := makeSecret("update-test", "auto-import-secret")
	existing.Object["data"] = map[string]interface{}{
		"kubeconfig": "old-value",
	}
	existing.Object["type"] = "Opaque"
	m := newManager(existing)
	ctx := context.Background()

	err := m.createAutoImportSecret(ctx, "update-test", []byte("new-kubeconfig"))
	if err != nil {
		t.Fatalf("createAutoImportSecret (update) failed: %v", err)
	}
}

func TestUpdateAutoImportSecretSuccess(t *testing.T) {
	existing := makeSecret("upd-test", "auto-import-secret")
	existing.Object["data"] = map[string]interface{}{
		"kubeconfig": "old",
	}
	m := newManager(existing)
	ctx := context.Background()

	err := m.updateAutoImportSecret(ctx, "upd-test", []byte("new-data"))
	if err != nil {
		t.Fatalf("updateAutoImportSecret failed: %v", err)
	}
}

func TestUpdateAutoImportSecretGetError(t *testing.T) {
	m := newManager() // no secret exists
	ctx := context.Background()

	err := m.updateAutoImportSecret(ctx, "missing-ns", []byte("data"))
	if err == nil {
		t.Error("expected error when secret doesn't exist")
	}
}

// ---------------------------------------------------------------------------
// ImportStatus struct (field validation)
// ---------------------------------------------------------------------------

func TestImportStatusFields(t *testing.T) {
	status := ImportStatus{
		Name:       "external-cluster",
		Available:  "True",
		Joined:     "True",
		CreatedVia: "discovery",
		AutoImport: true,
		Labels: map[string]string{
			"cloud":  "Other",
			"vendor": "Other",
		},
	}
	if status.Name != "external-cluster" {
		t.Errorf("expected name=external-cluster, got %s", status.Name)
	}
	if status.CreatedVia != "discovery" {
		t.Errorf("expected createdVia=discovery, got %s", status.CreatedVia)
	}
	if status.Labels["cloud"] != "Other" {
		t.Errorf("expected cloud=Other, got %s", status.Labels["cloud"])
	}
}

// ---------------------------------------------------------------------------
// ImportResult struct
// ---------------------------------------------------------------------------

func TestImportResultAutoImportMessage(t *testing.T) {
	result := &ImportResult{
		Name:       "test",
		AutoImport: true,
		Message:    "Cluster test registered for import (auto-import enabled)",
	}
	if !result.AutoImport {
		t.Error("expected AutoImport=true")
	}

	resultManual := &ImportResult{
		Name:       "test",
		AutoImport: false,
		Message:    "Cluster test registered for import — apply import manifests manually on the spoke",
	}
	if resultManual.AutoImport {
		t.Error("expected AutoImport=false")
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestImportNilLabels(t *testing.T) {
	m := newManager()
	ctx := context.Background()

	result, err := m.Import(ctx, ImportOptions{
		Name:   "nil-labels",
		Labels: nil,
	})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if result.Name != "nil-labels" {
		t.Errorf("name = %q, want nil-labels", result.Name)
	}
}

func TestImportEmptyLabels(t *testing.T) {
	m := newManager()
	ctx := context.Background()

	result, err := m.Import(ctx, ImportOptions{
		Name:   "empty-labels",
		Labels: map[string]string{},
	})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if result.Name != "empty-labels" {
		t.Errorf("name = %q, want empty-labels", result.Name)
	}
}

func TestGetImportStatusNoConditions(t *testing.T) {
	mc := makeManagedCluster("no-cond", nil, nil, "", "")
	m := newManager(mc)
	ctx := context.Background()

	status, err := m.GetImportStatus(ctx, "no-cond")
	if err != nil {
		t.Fatalf("GetImportStatus failed: %v", err)
	}
	if status.Available != "" {
		t.Errorf("available = %q, want empty", status.Available)
	}
	if status.Joined != "" {
		t.Errorf("joined = %q, want empty", status.Joined)
	}
}

func TestListImportedAllProvisioned(t *testing.T) {
	mc1 := makeManagedCluster("prov-1", nil, nil, "True", "True")
	cd1 := makeClusterDeployment("prov-1", "prov-1")
	mc2 := makeManagedCluster("prov-2", nil, nil, "True", "True")
	cd2 := makeClusterDeployment("prov-2", "prov-2")
	m := newManager(mc1, cd1, mc2, cd2)
	ctx := context.Background()

	imported, err := m.ListImported(ctx)
	if err != nil {
		t.Fatalf("ListImported failed: %v", err)
	}
	if len(imported) != 0 {
		t.Errorf("got %d imported, want 0 (all provisioned)", len(imported))
	}
}

func TestListImportedMixedConditions(t *testing.T) {
	mc := makeManagedCluster("mixed", nil, nil, "Unknown", "False")
	m := newManager(mc)
	ctx := context.Background()

	imported, err := m.ListImported(ctx)
	if err != nil {
		t.Fatalf("ListImported failed: %v", err)
	}
	if len(imported) != 1 {
		t.Fatalf("got %d, want 1", len(imported))
	}
	if imported[0].Available != "Unknown" {
		t.Errorf("available = %q, want Unknown", imported[0].Available)
	}
	if imported[0].Joined != "False" {
		t.Errorf("joined = %q, want False", imported[0].Joined)
	}
}
