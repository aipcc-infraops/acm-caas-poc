package pool

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
	client.GVRClusterPool:  "ClusterPoolList",
	client.GVRClusterClaim: "ClusterClaimList",
	client.GVRNamespace:    "NamespaceList",
}

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrKinds, objs...)
	return &client.Client{Dynamic: fake}
}

func newManager(objs ...runtime.Object) *Manager {
	return New(fakeClient(objs...), config.Config{}, discardLogger)
}

func clusterPool(name, namespace string, size int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterPool",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"size":       size,
				"baseDomain": "example.com",
				"imageSetRef": map[string]interface{}{
					"name": "ocp-4.19",
				},
				"platform": map[string]interface{}{
					"ibmcloud": map[string]interface{}{
						"region": "us-south",
					},
				},
			},
			"status": map[string]interface{}{
				"ready":               int64(2),
				"standby":             int64(1),
				"claimedClusterCount": int64(0),
			},
		},
	}
}

func clusterClaim(name, namespace, poolName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterClaim",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"clusterPoolName": poolName,
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "ClusterRunning",
						"status": "True",
					},
				},
				"clusterDeploymentRef": map[string]interface{}{
					"name": "pool-abc123",
				},
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

func TestCreatePoolCreatesClusterPool(t *testing.T) {
	mgr := newManager()
	err := mgr.CreatePool(context.Background(), PoolOpts{
		Name:     "amd64-419",
		Size:     3,
		Platform: "ibmcloud",
		Region:   "us-south",
		ImageSet: "img4.19-multi",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pool, err := mgr.GetPool(context.Background(), "amd64-419", "amd64-419")
	if err != nil {
		t.Fatalf("pool not found after create: %v", err)
	}
	if pool.Name != "amd64-419" {
		t.Errorf("Name = %q, want %q", pool.Name, "amd64-419")
	}
}

func TestCreatePoolIdempotent(t *testing.T) {
	mgr := newManager()
	opts := PoolOpts{
		Name:     "test-pool",
		Size:     2,
		ImageSet: "ocp-4.19",
	}
	if err := mgr.CreatePool(context.Background(), opts); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := mgr.CreatePool(context.Background(), opts); err != nil {
		t.Fatalf("second create should be idempotent: %v", err)
	}
}

func TestCreatePoolDefaultSize(t *testing.T) {
	mgr := newManager()
	err := mgr.CreatePool(context.Background(), PoolOpts{
		Name:     "small",
		Size:     0,
		ImageSet: "ocp-4.19",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreatePoolCustomNamespace(t *testing.T) {
	mgr := newManager()
	err := mgr.CreatePool(context.Background(), PoolOpts{
		Name:      "my-pool",
		Namespace: "custom-ns",
		Size:      1,
		ImageSet:  "ocp-4.19",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListPoolsEmpty(t *testing.T) {
	mgr := newManager()
	pools, err := mgr.ListPools(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pools) != 0 {
		t.Errorf("got %d pools, want 0", len(pools))
	}
}

func TestListPoolsWithExisting(t *testing.T) {
	p1 := clusterPool("pool-1", "pool-1", 3)
	p2 := clusterPool("pool-2", "pool-2", 5)
	mgr := newManager(p1, p2)

	pools, err := mgr.ListPools(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pools) != 2 {
		t.Errorf("got %d pools, want 2", len(pools))
	}
}

func TestGetPoolReturnsInfo(t *testing.T) {
	p := clusterPool("my-pool", "my-pool", 3)
	mgr := newManager(p)

	info, err := mgr.GetPool(context.Background(), "my-pool", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != "my-pool" {
		t.Errorf("Name = %q, want %q", info.Name, "my-pool")
	}
	if info.Size != 3 {
		t.Errorf("Size = %d, want 3", info.Size)
	}
	if info.Ready != 2 {
		t.Errorf("Ready = %d, want 2", info.Ready)
	}
	if info.Standby != 1 {
		t.Errorf("Standby = %d, want 1", info.Standby)
	}
}

func TestGetPoolNotFound(t *testing.T) {
	mgr := newManager()
	_, err := mgr.GetPool(context.Background(), "missing", "")
	if err == nil {
		t.Fatal("expected error for missing pool")
	}
}

func TestDeletePoolRemoves(t *testing.T) {
	p := clusterPool("del-pool", "del-pool", 2)
	mgr := newManager(p)

	err := mgr.DeletePool(context.Background(), "del-pool", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = mgr.GetPool(context.Background(), "del-pool", "")
	if err == nil {
		t.Fatal("pool should not exist after delete")
	}
}

func TestDeletePoolIdempotent(t *testing.T) {
	mgr := newManager()
	err := mgr.DeletePool(context.Background(), "nonexistent", "ns")
	if err != nil {
		t.Fatalf("deleting nonexistent pool should be idempotent: %v", err)
	}
}

func TestClaimCreatesClusterClaim(t *testing.T) {
	p := clusterPool("fast-pool", "fast-pool", 3)
	mgr := newManager(p)

	info, err := mgr.Claim(context.Background(), "fast-pool", "", "my-claim", "48h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != "my-claim" {
		t.Errorf("Name = %q, want %q", info.Name, "my-claim")
	}
	if info.Pool != "fast-pool" {
		t.Errorf("Pool = %q, want %q", info.Pool, "fast-pool")
	}
}

func TestClaimDefaultName(t *testing.T) {
	mgr := newManager()
	info, err := mgr.Claim(context.Background(), "my-pool", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != "my-pool-claim" {
		t.Errorf("Name = %q, want %q", info.Name, "my-pool-claim")
	}
}

func TestClaimIdempotent(t *testing.T) {
	mgr := newManager()
	_, err := mgr.Claim(context.Background(), "p1", "p1", "c1", "")
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	_, err = mgr.Claim(context.Background(), "p1", "p1", "c1", "")
	if err != nil {
		t.Fatalf("second claim should be idempotent: %v", err)
	}
}

func TestReleaseClaimDeletes(t *testing.T) {
	cc := clusterClaim("my-claim", "pool-ns", "my-pool")
	mgr := newManager(cc)

	err := mgr.ReleaseClaim(context.Background(), "my-claim", "pool-ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReleaseClaimRequiresNamespace(t *testing.T) {
	mgr := newManager()
	err := mgr.ReleaseClaim(context.Background(), "my-claim", "")
	if err == nil {
		t.Fatal("expected error when namespace is empty")
	}
	if !strings.Contains(err.Error(), "namespace is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReleaseClaimIdempotent(t *testing.T) {
	mgr := newManager()
	err := mgr.ReleaseClaim(context.Background(), "gone", "ns")
	if err != nil {
		t.Fatalf("releasing nonexistent claim should be idempotent: %v", err)
	}
}

func TestListClaimsEmpty(t *testing.T) {
	mgr := newManager()
	claims, err := mgr.ListClaims(context.Background(), "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(claims) != 0 {
		t.Errorf("got %d claims, want 0", len(claims))
	}
}

func TestListClaimsWithExisting(t *testing.T) {
	cc1 := clusterClaim("claim-1", "pool-ns", "pool-a")
	cc2 := clusterClaim("claim-2", "pool-ns", "pool-b")
	mgr := newManager(cc1, cc2)

	claims, err := mgr.ListClaims(context.Background(), "pool-ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(claims) != 2 {
		t.Errorf("got %d claims, want 2", len(claims))
	}
}

func TestParsePoolInfoFromStatus(t *testing.T) {
	p := clusterPool("test", "ns", 5)
	info := parsePoolInfo(p.Object)

	if info.Name != "test" {
		t.Errorf("Name = %q, want %q", info.Name, "test")
	}
	if info.Size != 5 {
		t.Errorf("Size = %d, want 5", info.Size)
	}
	if info.Ready != 2 {
		t.Errorf("Ready = %d, want 2", info.Ready)
	}
	if info.Standby != 1 {
		t.Errorf("Standby = %d, want 1", info.Standby)
	}
	if info.Claimed != 0 {
		t.Errorf("Claimed = %d, want 0", info.Claimed)
	}
}

func TestParsePoolInfoNoStatus(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      "bare",
			"namespace": "ns",
		},
		"spec": map[string]interface{}{
			"size": int64(3),
		},
	}
	info := parsePoolInfo(obj)
	if info.Name != "bare" {
		t.Errorf("Name = %q, want %q", info.Name, "bare")
	}
	if info.Size != 3 {
		t.Errorf("Size = %d, want 3", info.Size)
	}
	if info.Ready != 0 {
		t.Errorf("Ready = %d, want 0", info.Ready)
	}
}

func TestParseClaimInfoWithStatus(t *testing.T) {
	cc := clusterClaim("my-claim", "ns", "my-pool")
	info := parseClaimInfo(cc.Object)

	if info.Name != "my-claim" {
		t.Errorf("Name = %q, want %q", info.Name, "my-claim")
	}
	if info.Pool != "my-pool" {
		t.Errorf("Pool = %q, want %q", info.Pool, "my-pool")
	}
	if info.Cluster != "pool-abc123" {
		t.Errorf("Cluster = %q, want %q", info.Cluster, "pool-abc123")
	}
	if info.Status != "Running" {
		t.Errorf("Status = %q, want %q", info.Status, "Running")
	}
}

func TestParseClaimInfoPending(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      "pending-claim",
			"namespace": "ns",
		},
		"spec": map[string]interface{}{
			"clusterPoolName": "my-pool",
		},
	}
	info := parseClaimInfo(obj)
	if info.Status != "Pending" {
		t.Errorf("Status = %q, want %q", info.Status, "Pending")
	}
}

func TestBuildClusterPoolSpec(t *testing.T) {
	opts := PoolOpts{
		Name:       "test-pool",
		Namespace:  "test-pool",
		Size:       3,
		Platform:   "aws",
		Region:     "us-east-1",
		ImageSet:   "ocp-4.19",
		BaseDomain: "test.example.com",
	}
	obj := buildClusterPool(opts)

	if obj.GetName() != "test-pool" {
		t.Errorf("Name = %q, want %q", obj.GetName(), "test-pool")
	}

	size, _, _ := unstructured.NestedInt64(obj.Object, "spec", "size")
	if size != 3 {
		t.Errorf("size = %d, want 3", size)
	}

	bd, _, _ := unstructured.NestedString(obj.Object, "spec", "baseDomain")
	if bd != "test.example.com" {
		t.Errorf("baseDomain = %q, want %q", bd, "test.example.com")
	}

	imgRef, _, _ := unstructured.NestedString(obj.Object, "spec", "imageSetRef", "name")
	if imgRef != "ocp-4.19" {
		t.Errorf("imageSetRef.name = %q, want %q", imgRef, "ocp-4.19")
	}

	region, _, _ := unstructured.NestedString(obj.Object, "spec", "platform", "aws", "region")
	if region != "us-east-1" {
		t.Errorf("platform.aws.region = %q, want %q", region, "us-east-1")
	}
}

func TestBuildClusterPoolDefaults(t *testing.T) {
	opts := PoolOpts{
		Name:      "def-pool",
		Namespace: "def-pool",
		Size:      1,
		ImageSet:  "ocp-4.19",
	}
	obj := buildClusterPool(opts)

	bd, _, _ := unstructured.NestedString(obj.Object, "spec", "baseDomain")
	if bd != "example.com" {
		t.Errorf("baseDomain = %q, want default %q", bd, "example.com")
	}

	region, _, _ := unstructured.NestedString(obj.Object, "spec", "platform", "ibmcloud", "region")
	if region != "us-south" {
		t.Errorf("platform default region = %q, want %q", region, "us-south")
	}
}

func TestBuildClusterClaimSpec(t *testing.T) {
	obj := buildClusterClaim("my-pool", "my-claim", "ns", "48h")

	if obj.GetName() != "my-claim" {
		t.Errorf("Name = %q, want %q", obj.GetName(), "my-claim")
	}

	pool, _, _ := unstructured.NestedString(obj.Object, "spec", "clusterPoolName")
	if pool != "my-pool" {
		t.Errorf("clusterPoolName = %q, want %q", pool, "my-pool")
	}

	lifetime, _, _ := unstructured.NestedString(obj.Object, "spec", "lifetime")
	if lifetime != "48h" {
		t.Errorf("lifetime = %q, want %q", lifetime, "48h")
	}
}

func TestBuildClusterClaimNoTTL(t *testing.T) {
	obj := buildClusterClaim("pool", "claim", "ns", "")

	_, found, _ := unstructured.NestedString(obj.Object, "spec", "lifetime")
	if found {
		t.Error("lifetime should not be set when TTL is empty")
	}
}

func TestCreatePoolNamespaceError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "namespaces", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("forbidden")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.CreatePool(context.Background(), PoolOpts{Name: "p1", Size: 1, ImageSet: "ocp"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "ensuring namespace") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestListPoolsError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "clusterpools", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("timeout")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.ListPools(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "listing ClusterPools") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClaimCreateError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "clusterclaims", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("pool full")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.Claim(context.Background(), "p1", "p1", "c1", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "creating ClusterClaim") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestListClaimsError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "clusterclaims", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("timeout")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.ListClaims(context.Background(), "ns")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "listing ClusterClaims") {
		t.Errorf("unexpected error: %v", err)
	}
}
