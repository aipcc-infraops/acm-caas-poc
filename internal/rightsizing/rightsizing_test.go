package rightsizing

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
	client.GVRManifestWork:     "ManifestWorkList",
	client.GVRManagedCluster:   "ManagedClusterList",
}

func managedClusterObj(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": name,
			},
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

func rightsizingMW(cluster string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      rightsizingMWName(cluster),
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":    "true",
					"acmlab.redhat.com/rightsizing": "true",
				},
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

func TestNewReturnsManager(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)
	if mgr == nil {
		t.Fatal("New returned nil")
	}
}

func TestEnableCreatesManifestWork(t *testing.T) {
	mgr := newManager(managedClusterObj("spoke1"))
	err := mgr.Enable(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("Enable failed: %v", err)
	}

	_, err = mgr.client.Get(context.Background(), client.GVRManifestWork, "spoke1", "rightsizing-spoke1")
	if err != nil {
		t.Fatalf("ManifestWork not found: %v", err)
	}
}

func TestEnableIdempotent(t *testing.T) {
	mgr := newManager(managedClusterObj("spoke1"))
	if err := mgr.Enable(context.Background(), "spoke1"); err != nil {
		t.Fatalf("first enable: %v", err)
	}
	if err := mgr.Enable(context.Background(), "spoke1"); err != nil {
		t.Fatalf("second enable should be idempotent: %v", err)
	}
}

func TestEnableError(t *testing.T) {
	c := fakeClient(managedClusterObj("spoke1"))
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "manifestworks", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("forbidden")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.Enable(context.Background(), "spoke1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "creating right-sizing ManifestWork") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDisableExisting(t *testing.T) {
	mgr := newManager(managedClusterObj("spoke1"))
	if err := mgr.Enable(context.Background(), "spoke1"); err != nil {
		t.Fatalf("Enable: %v", err)
	}

	removed, err := mgr.Disable(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("Disable failed: %v", err)
	}
	if !removed {
		t.Error("Disable should return true for existing")
	}
}

func TestDisableNotFound(t *testing.T) {
	mgr := newManager()
	removed, err := mgr.Disable(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("Disable failed: %v", err)
	}
	if removed {
		t.Error("Disable should return false for nonexistent")
	}
}

func TestDisableDeleteError(t *testing.T) {
	mgr := newManager(managedClusterObj("spoke1"))
	if err := mgr.Enable(context.Background(), "spoke1"); err != nil {
		t.Fatalf("Enable: %v", err)
	}

	fake := mgr.client.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("delete", "manifestworks", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("blocked")
	})

	_, err := mgr.Disable(context.Background(), "spoke1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAdviseNotEnabled(t *testing.T) {
	mgr := newManager()
	_, err := mgr.Advise(context.Background(), "spoke1")
	if err == nil {
		t.Fatal("expected error when not enabled")
	}
	if !strings.Contains(err.Error(), "not enabled") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAdviseReturnsRecommendations(t *testing.T) {
	mw := rightsizingMW("spoke1")
	mgr := newManager(mw)

	recs, err := mgr.Advise(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("Advise failed: %v", err)
	}
	if len(recs) == 0 {
		t.Error("expected at least one recommendation")
	}
	if recs[0].Savings == "" {
		t.Error("recommendation should have savings")
	}
}

func TestAdjustDryRun(t *testing.T) {
	mw := rightsizingMW("spoke1")
	mgr := newManager(mw)

	results, err := mgr.Adjust(context.Background(), "spoke1", true)
	if err != nil {
		t.Fatalf("Adjust dry-run failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected results")
	}
	for _, r := range results {
		if !r.DryRun {
			t.Error("all results should be dry run")
		}
	}
}

func TestAdjustApply(t *testing.T) {
	mw := rightsizingMW("spoke1")
	mgr := newManager(mw)

	results, err := mgr.Adjust(context.Background(), "spoke1", false)
	if err != nil {
		t.Fatalf("Adjust apply failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected results")
	}
	for _, r := range results {
		if r.DryRun {
			t.Error("results should not be dry run")
		}
	}
}

func TestAdjustNotEnabled(t *testing.T) {
	mgr := newManager()
	_, err := mgr.Adjust(context.Background(), "spoke1", false)
	if err == nil {
		t.Fatal("expected error when not enabled")
	}
}

func TestListEmpty(t *testing.T) {
	mgr := newManager()
	infos, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("got %d infos, want 0", len(infos))
	}
}

func TestListWithExisting(t *testing.T) {
	mw := rightsizingMW("spoke1")
	mgr := newManager(mw)

	infos, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(infos) != 1 {
		t.Errorf("got %d infos, want 1", len(infos))
	}
	if infos[0].Cluster != "spoke1" {
		t.Errorf("Cluster = %q, want spoke1", infos[0].Cluster)
	}
	if infos[0].Status != "Active" {
		t.Errorf("Status = %q, want Active", infos[0].Status)
	}
}

func TestListError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "manifestworks", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("timeout")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.List(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildRightsizingManifestWork(t *testing.T) {
	mw := buildRightsizingManifestWork("spoke1")
	if mw.GetName() != "rightsizing-spoke1" {
		t.Errorf("Name = %q", mw.GetName())
	}
	if mw.GetNamespace() != "spoke1" {
		t.Errorf("Namespace = %q", mw.GetNamespace())
	}
	labels := mw.GetLabels()
	if labels["acmlab.redhat.com/rightsizing"] != "true" {
		t.Error("rightsizing label missing")
	}
}

func TestBuildAdjustManifestWork(t *testing.T) {
	rec := Recommendation{
		Namespace:      "default",
		Workload:       "myapp",
		Container:      "main",
		RecommendedCPU: "250m",
		RecommendedMem: "256Mi",
	}
	mw := buildAdjustManifestWork("spoke1", rec)
	if mw.GetName() != "rightsizing-adjust-myapp-spoke1" {
		t.Errorf("Name = %q", mw.GetName())
	}
	labels := mw.GetLabels()
	if labels["acmlab.redhat.com/workload"] != "myapp" {
		t.Errorf("workload label = %q", labels["acmlab.redhat.com/workload"])
	}
}

func TestParseRightsizingInfoActive(t *testing.T) {
	mw := rightsizingMW("spoke1")
	info := parseRightsizingInfo(mw.Object)
	if info.Cluster != "spoke1" {
		t.Errorf("Cluster = %q", info.Cluster)
	}
	if info.Status != "Active" {
		t.Errorf("Status = %q, want Active", info.Status)
	}
}

func TestParseRightsizingInfoPending(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"namespace": "spoke2",
		},
	}
	info := parseRightsizingInfo(obj)
	if info.Status != "Pending" {
		t.Errorf("Status = %q, want Pending", info.Status)
	}
}

func TestRightsizingMWName(t *testing.T) {
	tests := []struct {
		cluster string
		want    string
	}{
		{"spoke1", "rightsizing-spoke1"},
		{"my-cluster", "rightsizing-my-cluster"},
	}
	for _, tt := range tests {
		got := rightsizingMWName(tt.cluster)
		if got != tt.want {
			t.Errorf("rightsizingMWName(%q) = %q, want %q", tt.cluster, got, tt.want)
		}
	}
}
