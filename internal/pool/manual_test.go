package pool

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
	"github.com/pablofelix/acm-caas-poc/internal/lifecycle"
	"github.com/pablofelix/acm-caas-poc/internal/provisioning"
)

var manualGVRKinds = map[schema.GroupVersionResource]string{
	client.GVRClusterDeployment:     "ClusterDeploymentList",
	client.GVRClusterImageSet:       "ClusterImageSetList",
	client.GVRManagedCluster:        "ManagedClusterList",
	client.GVRKlusterletAddonConfig: "KlusterletAddonConfigList",
	client.GVRNamespace:             "NamespaceList",
	client.GVRSecret:                "SecretList",
	client.GVRClusterPool:           "ClusterPoolList",
	client.GVRClusterClaim:          "ClusterClaimList",
}

func manualFakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, manualGVRKinds, objs...)
	return &client.Client{Dynamic: fake}
}

func newManualManager(objs ...runtime.Object) *Manager {
	c := manualFakeClient(objs...)
	cfg := config.Config{
		Platform:        "ibmcloud",
		IBMCloudRegion:  "us-east",
		BaseDomain:      "test.example.com",
		ClusterImageSet: "img4.22.15-multi-appsub",
		IBMCloudAPIKey:  "test-api-key",
	}
	provMgr := provisioning.New(c, cfg, discardLogger)
	lcMgr := lifecycle.New(c, cfg, discardLogger)
	return NewWithManagers(c, cfg, discardLogger, provMgr, lcMgr)
}

func poolClusterDeployment(name, poolName string, installed bool, powerState string, claimed string) *unstructured.Unstructured {
	labels := map[string]interface{}{
		"acmlab.redhat.com/managed":      "true",
		"acmlab.redhat.com/pool":         poolName,
		"acmlab.redhat.com/pool-claimed": claimed,
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": name,
				"labels":    labels,
			},
			"spec": map[string]interface{}{
				"installed":  installed,
				"baseDomain": "test.example.com",
				"clusterName": name,
				"platform": map[string]interface{}{
					"ibmcloud": map[string]interface{}{
						"region": "us-east",
					},
				},
			},
		},
	}

	if powerState != "" {
		_ = unstructured.SetNestedField(obj.Object, powerState, "spec", "powerState")
	}

	return obj
}

func TestNewWithManagersReturnsManager(t *testing.T) {
	mgr := newManualManager()
	if mgr == nil {
		t.Fatal("NewWithManagers returned nil")
	}
	if mgr.provisioning == nil {
		t.Error("provisioning manager should not be nil")
	}
	if mgr.lifecycle == nil {
		t.Error("lifecycle manager should not be nil")
	}
}

func TestCreateManualPoolRequiresProvisioningManager(t *testing.T) {
	mgr := newManager()
	err := mgr.CreateManualPool(context.Background(), ManualPoolOpts{
		Name: "test-pool",
		Size: 2,
	})
	if err == nil {
		t.Fatal("expected error when provisioning manager is nil")
	}
	if !strings.Contains(err.Error(), "provisioning manager required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCreateManualPoolCreatesClusterDeployments(t *testing.T) {
	mgr := newManualManager()
	// AWS doesn't call external APIs during Create (no generateIBMCloudCredentials)
	err := mgr.CreateManualPool(context.Background(), ManualPoolOpts{
		Name: "test-pool",
		Size: 2,
		ProvisionOpts: provisioning.ClusterOpts{
			Platform:           "aws",
			Region:             "us-east-1",
			BaseDomain:         "test.example.com",
			ImageSet:           "img4.22.15-multi-appsub",
			AWSAccessKeyID:     "test-access-key-id",
			AWSSecretAccessKey: "test-secret-access-key",
			PullSecret:         `{"auths":{}}`,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	clusters, err := mgr.listPoolClusters(context.Background(), "test-pool")
	if err != nil {
		t.Fatalf("unexpected error listing pool clusters: %v", err)
	}
	if len(clusters) != 2 {
		t.Errorf("got %d clusters, want 2", len(clusters))
	}
}

func TestCreateManualPoolDefaultSize(t *testing.T) {
	mgr := newManualManager()
	err := mgr.CreateManualPool(context.Background(), ManualPoolOpts{
		Name: "test-pool",
		Size: 0,
		ProvisionOpts: provisioning.ClusterOpts{
			Platform:           "aws",
			Region:             "us-east-1",
			BaseDomain:         "test.example.com",
			ImageSet:           "img4.22.15-multi-appsub",
			AWSAccessKeyID:     "test-key",
			AWSSecretAccessKey: "test-secret",
			PullSecret:         `{"auths":{}}`,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Default size is 2
	clusters, err := mgr.listPoolClusters(context.Background(), "test-pool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(clusters) != 2 {
		t.Errorf("got %d clusters, want 2 (default size)", len(clusters))
	}
}

func TestListManualPoolEmpty(t *testing.T) {
	mgr := newManualManager()
	info, err := mgr.ListManualPool(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Size != 0 {
		t.Errorf("Size = %d, want 0", info.Size)
	}
}

func TestListManualPoolWithClusters(t *testing.T) {
	cd1 := poolClusterDeployment("pool-1", "test-pool", true, "Hibernating", "false")
	cd2 := poolClusterDeployment("pool-2", "test-pool", true, "Hibernating", "false")
	cd3 := poolClusterDeployment("pool-3", "test-pool", true, "Running", "true")

	mgr := newManualManager(cd1, cd2, cd3)

	info, err := mgr.ListManualPool(context.Background(), "test-pool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Size != 3 {
		t.Errorf("Size = %d, want 3", info.Size)
	}
	if info.Standby != 2 {
		t.Errorf("Standby = %d, want 2", info.Standby)
	}
	if info.Claimed != 1 {
		t.Errorf("Claimed = %d, want 1", info.Claimed)
	}
}

func TestListManualPoolProvisioningCluster(t *testing.T) {
	cd := poolClusterDeployment("pool-1", "test-pool", false, "", "false")
	mgr := newManualManager(cd)

	info, err := mgr.ListManualPool(context.Background(), "test-pool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Size != 1 {
		t.Errorf("Size = %d, want 1", info.Size)
	}
	if info.Ready != 0 {
		t.Errorf("Ready = %d, want 0 (still provisioning)", info.Ready)
	}
}

func TestClaimManualPoolFindsHibernated(t *testing.T) {
	cd1 := poolClusterDeployment("pool-1", "test-pool", true, "Hibernating", "false")
	mgr := newManualManager(cd1)

	info, err := mgr.ClaimManualPool(context.Background(), "test-pool", "my-claim")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Cluster != "pool-1" {
		t.Errorf("Cluster = %q, want %q", info.Cluster, "pool-1")
	}
	if info.Name != "my-claim" {
		t.Errorf("Name = %q, want %q", info.Name, "my-claim")
	}
	if info.Pool != "test-pool" {
		t.Errorf("Pool = %q, want %q", info.Pool, "test-pool")
	}
}

func TestClaimManualPoolSkipsClaimed(t *testing.T) {
	cd1 := poolClusterDeployment("pool-1", "test-pool", true, "Hibernating", "true")
	cd2 := poolClusterDeployment("pool-2", "test-pool", true, "Hibernating", "false")
	mgr := newManualManager(cd1, cd2)

	info, err := mgr.ClaimManualPool(context.Background(), "test-pool", "claim")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Cluster != "pool-2" {
		t.Errorf("Cluster = %q, want %q (should skip claimed pool-1)", info.Cluster, "pool-2")
	}
}

func TestClaimManualPoolNoneAvailable(t *testing.T) {
	cd1 := poolClusterDeployment("pool-1", "test-pool", true, "Hibernating", "true")
	mgr := newManualManager(cd1)

	_, err := mgr.ClaimManualPool(context.Background(), "test-pool", "claim")
	if err == nil {
		t.Fatal("expected error when no clusters available")
	}
	if !strings.Contains(err.Error(), "no available") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClaimManualPoolSkipsRunning(t *testing.T) {
	cd1 := poolClusterDeployment("pool-1", "test-pool", true, "Running", "false")
	mgr := newManualManager(cd1)

	_, err := mgr.ClaimManualPool(context.Background(), "test-pool", "claim")
	if err == nil {
		t.Fatal("expected error — running clusters should not be claimable")
	}
}

func TestClaimManualPoolRequiresLifecycle(t *testing.T) {
	mgr := newManager()
	_, err := mgr.ClaimManualPool(context.Background(), "pool", "claim")
	if err == nil {
		t.Fatal("expected error when lifecycle manager is nil")
	}
}

func TestReleaseManualClaimHibernates(t *testing.T) {
	cd := poolClusterDeployment("pool-1", "test-pool", true, "Running", "true")
	mgr := newManualManager(cd)

	err := mgr.ReleaseManualClaim(context.Background(), "pool-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReleaseManualClaimRequiresLifecycle(t *testing.T) {
	mgr := newManager()
	err := mgr.ReleaseManualClaim(context.Background(), "pool-1")
	if err == nil {
		t.Fatal("expected error when lifecycle manager is nil")
	}
}

func TestDeleteManualPoolRequiresProvisioning(t *testing.T) {
	mgr := newManager()
	err := mgr.DeleteManualPool(context.Background(), "pool")
	if err == nil {
		t.Fatal("expected error when provisioning manager is nil")
	}
}

func TestDeleteManualPoolDestroysAll(t *testing.T) {
	cd1 := poolClusterDeployment("pool-1", "test-pool", true, "Hibernating", "false")
	cd2 := poolClusterDeployment("pool-2", "test-pool", true, "Hibernating", "false")
	mgr := newManualManager(cd1, cd2)

	err := mgr.DeleteManualPool(context.Background(), "test-pool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := mgr.ListManualPool(context.Background(), "test-pool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Size != 0 {
		t.Errorf("Size = %d after delete, want 0", info.Size)
	}
}

func TestDeleteManualPoolEmptyPool(t *testing.T) {
	mgr := newManualManager()
	err := mgr.DeleteManualPool(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("deleting empty pool should not error: %v", err)
	}
}

func TestListPoolClusters(t *testing.T) {
	cd1 := poolClusterDeployment("pool-1", "my-pool", true, "Hibernating", "false")
	cd2 := poolClusterDeployment("pool-2", "my-pool", true, "Running", "true")
	cd3 := poolClusterDeployment("other-1", "other-pool", true, "Hibernating", "false")
	mgr := newManualManager(cd1, cd2, cd3)

	names, err := mgr.listPoolClusters(context.Background(), "my-pool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("got %d clusters, want 2 (should not include other-pool)", len(names))
	}
}

func TestWaitManualPoolReadyRequiresManagers(t *testing.T) {
	mgr := newManager()
	err := mgr.WaitManualPoolReady(context.Background(), "pool", 0)
	if err == nil {
		t.Fatal("expected error when managers are nil")
	}
}

func TestWaitManualPoolReadyEmptyPool(t *testing.T) {
	mgr := newManualManager()
	err := mgr.WaitManualPoolReady(context.Background(), "nonexistent", 0)
	if err == nil {
		t.Fatal("expected error for empty pool")
	}
	if !strings.Contains(err.Error(), "no clusters found") {
		t.Errorf("unexpected error: %v", err)
	}
}
