package access

import (
	"context"
	"fmt"
	"io"
	"log/slog"
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
	client.GVRManagedServiceAccount: "ManagedServiceAccountList",
	client.GVRManagedClusterAddOn:   "ManagedClusterAddOnList",
	client.GVRSecret:                "SecretList",
	client.GVRManagedCluster:        "ManagedClusterList",
}

func managedClusterObj(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata":   map[string]interface{}{"name": name},
		},
	}
}

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrKinds, objs...)
	return &client.Client{Dynamic: fake}
}

func newManager(objs ...runtime.Object) *Manager {
	return New(fakeClient(objs...), config.Config{}, discardLogger)
}

func msaObj(cluster string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "authentication.open-cluster-management.io/v1beta1",
			"kind":       "ManagedServiceAccount",
			"metadata": map[string]interface{}{
				"name":      msaName,
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"rotation": map[string]interface{}{
					"enabled":  true,
					"validity": "720h",
				},
			},
		},
	}
}

func addonObj(name, cluster string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "addon.open-cluster-management.io/v1alpha1",
			"kind":       "ManagedClusterAddOn",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": cluster,
			},
			"spec": map[string]interface{}{
				"installNamespace": "open-cluster-management-agent-addon",
			},
		},
	}
}

func msaWithToken(cluster string) *unstructured.Unstructured {
	obj := msaObj(cluster)
	obj.Object["status"] = map[string]interface{}{
		"tokenSecretRef": map[string]interface{}{
			"name": msaName,
		},
		"expirationTimestamp": "2026-10-16T12:00:00Z",
		"conditions": []interface{}{
			map[string]interface{}{
				"type":   "TokenReported",
				"status": "True",
			},
		},
	}
	return obj
}

func proxyAddonHealthy(cluster string) *unstructured.Unstructured {
	obj := addonObj("cluster-proxy", cluster)
	obj.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"type":   "Available",
				"status": "True",
			},
		},
	}
	return obj
}

func TestEnableCreatesResources(t *testing.T) {
	mgr := newManager()
	opts := AccessOpts{TTL: "720h"}

	if err := mgr.Enable(context.Background(), "spoke1", opts); err != nil {
		t.Fatalf("Enable failed: %v", err)
	}

	ctx := context.Background()
	_, err := mgr.client.Get(ctx, client.GVRManagedServiceAccount, "spoke1", msaName)
	if err != nil {
		t.Errorf("ManagedServiceAccount not found: %v", err)
	}
	_, err = mgr.client.Get(ctx, client.GVRManagedClusterAddOn, "spoke1", "cluster-proxy")
	if err != nil {
		t.Errorf("cluster-proxy addon not found: %v", err)
	}
	_, err = mgr.client.Get(ctx, client.GVRManagedClusterAddOn, "spoke1", "managed-serviceaccount")
	if err != nil {
		t.Errorf("managed-serviceaccount addon not found: %v", err)
	}
}

func TestEnableIdempotent(t *testing.T) {
	mgr := newManager()
	opts := AccessOpts{TTL: "720h"}

	if err := mgr.Enable(context.Background(), "spoke1", opts); err != nil {
		t.Fatalf("First Enable failed: %v", err)
	}
	if err := mgr.Enable(context.Background(), "spoke1", opts); err != nil {
		t.Fatalf("Second Enable failed (not idempotent): %v", err)
	}
}

func TestEnableDefaultTTL(t *testing.T) {
	mgr := newManager()
	opts := AccessOpts{}

	if err := mgr.Enable(context.Background(), "spoke2", opts); err != nil {
		t.Fatalf("Enable failed: %v", err)
	}

	obj, err := mgr.client.Get(context.Background(), client.GVRManagedServiceAccount, "spoke2", msaName)
	if err != nil {
		t.Fatalf("ManagedServiceAccount not found: %v", err)
	}
	spec, _ := obj.Object["spec"].(map[string]interface{})
	rotation, _ := spec["rotation"].(map[string]interface{})
	validity, _ := rotation["validity"].(string)
	if validity != "720h" {
		t.Errorf("Expected default TTL 720h, got %s", validity)
	}
}

func TestDisableExisting(t *testing.T) {
	mgr := newManager(
		msaObj("spoke1"),
		addonObj("cluster-proxy", "spoke1"),
		addonObj("managed-serviceaccount", "spoke1"),
	)

	removed, err := mgr.Disable(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("Disable failed: %v", err)
	}
	if !removed {
		t.Error("Disable should return true for existing access")
	}

	ctx := context.Background()
	_, err = mgr.client.Get(ctx, client.GVRManagedServiceAccount, "spoke1", msaName)
	if err == nil {
		t.Error("ManagedServiceAccount still exists after Disable")
	}
}

func TestDisableNonexistent(t *testing.T) {
	mgr := newManager()

	removed, err := mgr.Disable(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("Disable on empty cluster failed: %v", err)
	}
	if removed {
		t.Error("Disable should return false for nonexistent access")
	}
}

func TestGetStatusEnabled(t *testing.T) {
	mgr := newManager(
		managedClusterObj("spoke1"),
		msaWithToken("spoke1"),
		proxyAddonHealthy("spoke1"),
	)

	status, err := mgr.GetStatus(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if !status.Enabled {
		t.Error("Expected Enabled=true")
	}
	if !status.TokenAvailable {
		t.Error("Expected TokenAvailable=true")
	}
	if !status.AddonHealthy {
		t.Error("Expected AddonHealthy=true")
	}
	if status.TokenRotation != "720h" {
		t.Errorf("Expected TokenRotation=720h, got %s", status.TokenRotation)
	}
	if status.LastRotation != "2026-10-16T12:00:00Z" {
		t.Errorf("Expected LastRotation timestamp, got %s", status.LastRotation)
	}
}

func TestGetStatusDisabled(t *testing.T) {
	mgr := newManager(managedClusterObj("spoke1"))

	status, err := mgr.GetStatus(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if status.Enabled {
		t.Error("Expected Enabled=false for cluster without MSA")
	}
	if status.TokenAvailable {
		t.Error("Expected TokenAvailable=false")
	}
}

func TestGetStatusWithoutProxy(t *testing.T) {
	mgr := newManager(managedClusterObj("spoke1"), msaObj("spoke1"))

	status, err := mgr.GetStatus(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if !status.Enabled {
		t.Error("Expected Enabled=true")
	}
	if status.AddonHealthy {
		t.Error("Expected AddonHealthy=false when proxy not present")
	}
}

func TestList(t *testing.T) {
	mgr := newManager(
		msaWithToken("spoke1"),
		msaObj("spoke2"),
	)

	infos, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("Expected 2 access entries, got %d", len(infos))
	}

	found := map[string]bool{}
	for _, info := range infos {
		found[info.Cluster] = true
		if !info.Enabled {
			t.Errorf("Expected Enabled=true for %s", info.Cluster)
		}
	}
	if !found["spoke1"] || !found["spoke2"] {
		t.Error("Expected both spoke1 and spoke2 in list")
	}
}

func TestListEmpty(t *testing.T) {
	mgr := newManager()

	infos, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("Expected 0 entries, got %d", len(infos))
	}
}

func TestEnableError(t *testing.T) {
	c := fakeClient()
	fake := c.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("create", "managedclusteraddons", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("addon creation blocked")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.Enable(context.Background(), "spoke1", AccessOpts{})
	if err == nil {
		t.Fatal("Expected error from Enable when addon creation fails")
	}
}

func TestDisableError(t *testing.T) {
	c := fakeClient(msaObj("spoke1"))
	fake := c.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("delete", "managedserviceaccounts", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("delete blocked")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.Disable(context.Background(), "spoke1")
	if err == nil {
		t.Fatal("Expected error from Disable when delete fails")
	}
}

func TestGetStatusError(t *testing.T) {
	c := fakeClient()
	fake := c.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("get", "managedserviceaccounts", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("connection refused")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.GetStatus(context.Background(), "spoke1")
	if err == nil {
		t.Fatal("Expected error from GetStatus when get fails")
	}
}

func TestListError(t *testing.T) {
	c := fakeClient()
	fake := c.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("list", "managedserviceaccounts", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("connection refused")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.List(context.Background())
	if err == nil {
		t.Fatal("Expected error from List when list fails")
	}
}

func TestBuildManagedServiceAccount(t *testing.T) {
	obj := buildManagedServiceAccount("spoke1", AccessOpts{TTL: "48h"})
	if obj.GetName() != msaName {
		t.Errorf("Expected name %s, got %s", msaName, obj.GetName())
	}
	if obj.GetNamespace() != "spoke1" {
		t.Errorf("Expected namespace spoke1, got %s", obj.GetNamespace())
	}
	spec, _ := obj.Object["spec"].(map[string]interface{})
	rotation, _ := spec["rotation"].(map[string]interface{})
	if rotation["validity"] != "48h" {
		t.Errorf("Expected TTL 48h, got %v", rotation["validity"])
	}
}

func TestBuildManagedServiceAccountDefaultTTL(t *testing.T) {
	obj := buildManagedServiceAccount("spoke1", AccessOpts{})
	spec, _ := obj.Object["spec"].(map[string]interface{})
	rotation, _ := spec["rotation"].(map[string]interface{})
	if rotation["validity"] != "720h" {
		t.Errorf("Expected default TTL 720h, got %v", rotation["validity"])
	}
}

func TestBuildClusterProxyAddOn(t *testing.T) {
	obj := buildClusterProxyAddOn("spoke2")
	if obj.GetName() != "cluster-proxy" {
		t.Errorf("Expected name cluster-proxy, got %s", obj.GetName())
	}
	if obj.GetNamespace() != "spoke2" {
		t.Errorf("Expected namespace spoke2, got %s", obj.GetNamespace())
	}
}

func TestBuildManagedServiceAccountAddOn(t *testing.T) {
	obj := buildManagedServiceAccountAddOn("spoke1")
	if obj.GetName() != "managed-serviceaccount" {
		t.Errorf("Expected name managed-serviceaccount, got %s", obj.GetName())
	}
}

func TestParseAccessStatusNoStatus(t *testing.T) {
	obj := msaObj("spoke1").Object
	status := parseAccessStatus("spoke1", obj, nil)
	if !status.Enabled {
		t.Error("Expected Enabled=true")
	}
	if status.TokenAvailable {
		t.Error("Expected TokenAvailable=false when no status")
	}
	if status.AddonHealthy {
		t.Error("Expected AddonHealthy=false when no proxy")
	}
}

func TestParseAccessInfoWithToken(t *testing.T) {
	obj := msaWithToken("spoke1").Object
	info := parseAccessInfo("spoke1", obj)
	if !info.TokenAvailable {
		t.Error("Expected TokenAvailable=true")
	}
	if info.AddonStatus != "TokenReported" {
		t.Errorf("Expected AddonStatus=TokenReported, got %s", info.AddonStatus)
	}
}

func TestParseAccessInfoPending(t *testing.T) {
	obj := msaObj("spoke1").Object
	info := parseAccessInfo("spoke1", obj)
	if info.TokenAvailable {
		t.Error("Expected TokenAvailable=false")
	}
	if info.AddonStatus != "Pending" {
		t.Errorf("Expected AddonStatus=Pending, got %s", info.AddonStatus)
	}
}

func TestAddonIsHealthy(t *testing.T) {
	healthy := proxyAddonHealthy("spoke1").Object
	if !addonIsHealthy(healthy) {
		t.Error("Expected healthy addon to return true")
	}

	unhealthy := addonObj("cluster-proxy", "spoke1").Object
	if addonIsHealthy(unhealthy) {
		t.Error("Expected unhealthy addon to return false")
	}
}

func TestAddonIsHealthyNoStatus(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{"name": "test"},
	}
	if addonIsHealthy(obj) {
		t.Error("Expected false for addon without status")
	}
}
