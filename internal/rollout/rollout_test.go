package rollout

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
	client.GVRManifestWorkReplicaSet: "ManifestWorkReplicaSetList",
	client.GVRPlacement:              "PlacementList",
}

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrKinds, objs...)
	return &client.Client{Dynamic: fake}
}

func newManager(objs ...runtime.Object) *Manager {
	return New(fakeClient(objs...), config.Config{}, discardLogger)
}

func mwrs(name, namespace, strategy string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1alpha1",
			"kind":       "ManifestWorkReplicaSet",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"placementRefs": []interface{}{
					map[string]interface{}{
						"name": "gpu-placement",
						"rolloutStrategy": map[string]interface{}{
							"type": strategy,
						},
					},
				},
				"manifestWorkTemplate": map[string]interface{}{
					"workload": map[string]interface{}{
						"manifests": []interface{}{},
					},
				},
			},
		},
	}
}

func mwrsWithStatus(name, namespace string, applied, total, failed int64) *unstructured.Unstructured {
	obj := mwrs(name, namespace, "Progressive")
	obj.Object["status"] = map[string]interface{}{
		"summary": map[string]interface{}{
			"applied": applied,
			"total":   total,
			"failed":  failed,
		},
		"conditions": []interface{}{
			map[string]interface{}{
				"type":   "PlacementVerified",
				"status": "True",
			},
		},
	}
	return obj
}

func TestCreateRollout(t *testing.T) {
	mgr := newManager()
	opts := RolloutOpts{
		Name:          "kueue-v12",
		PlacementName: "gpu-placement",
		Strategy:      "Progressive",
		MaxConcurrency: 2,
		MaxFailures:   "10%",
	}
	if err := mgr.Create(context.Background(), opts); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	obj, err := mgr.client.Get(context.Background(), client.GVRManifestWorkReplicaSet, DefaultNamespace, "kueue-v12")
	if err != nil {
		t.Fatalf("Get after create failed: %v", err)
	}
	if obj.GetName() != "kueue-v12" {
		t.Errorf("expected name kueue-v12, got %s", obj.GetName())
	}
}

func TestCreateDefaultsNamespace(t *testing.T) {
	mgr := newManager()
	opts := RolloutOpts{
		Name:          "test-rollout",
		PlacementName: "my-placement",
	}
	if err := mgr.Create(context.Background(), opts); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	_, err := mgr.client.Get(context.Background(), client.GVRManifestWorkReplicaSet, DefaultNamespace, "test-rollout")
	if err != nil {
		t.Fatalf("expected rollout in default namespace: %v", err)
	}
}

func TestCreateDefaultsStrategy(t *testing.T) {
	mgr := newManager()
	opts := RolloutOpts{
		Name:          "test-rollout",
		PlacementName: "my-placement",
	}
	if err := mgr.Create(context.Background(), opts); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	obj, err := mgr.client.Get(context.Background(), client.GVRManifestWorkReplicaSet, DefaultNamespace, "test-rollout")
	if err != nil {
		t.Fatal(err)
	}
	refs, _, _ := unstructured.NestedSlice(obj.Object, "spec", "placementRefs")
	if len(refs) == 0 {
		t.Fatal("no placementRefs")
	}
	ref := refs[0].(map[string]interface{})
	rs := ref["rolloutStrategy"].(map[string]interface{})
	if rs["type"] != "All" {
		t.Errorf("expected default strategy All, got %v", rs["type"])
	}
}

func TestGetRollout(t *testing.T) {
	obj := mwrsWithStatus("kueue-v12", DefaultNamespace, 5, 10, 0)
	mgr := newManager(obj)

	info, err := mgr.Get(context.Background(), "kueue-v12", "")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if info.Name != "kueue-v12" {
		t.Errorf("expected name kueue-v12, got %s", info.Name)
	}
	if info.Strategy != "Progressive" {
		t.Errorf("expected strategy Progressive, got %s", info.Strategy)
	}
	if info.Applied != 5 {
		t.Errorf("expected applied=5, got %d", info.Applied)
	}
	if info.Total != 10 {
		t.Errorf("expected total=10, got %d", info.Total)
	}
	if info.Status != "Active" {
		t.Errorf("expected status Active, got %s", info.Status)
	}
}

func TestGetRolloutNotFound(t *testing.T) {
	mgr := newManager()
	_, err := mgr.Get(context.Background(), "nonexistent", "")
	if err == nil {
		t.Fatal("expected error for nonexistent rollout")
	}
}

func TestListRollouts(t *testing.T) {
	obj1 := mwrs("rollout-a", DefaultNamespace, "All")
	obj2 := mwrs("rollout-b", DefaultNamespace, "Progressive")
	mgr := newManager(obj1, obj2)

	infos, err := mgr.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("expected 2 rollouts, got %d", len(infos))
	}
}

func TestListRolloutsEmpty(t *testing.T) {
	mgr := newManager()
	infos, err := mgr.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("expected 0 rollouts, got %d", len(infos))
	}
}

func TestDeleteRolloutExisting(t *testing.T) {
	obj := mwrs("kueue-v12", DefaultNamespace, "Progressive")
	mgr := newManager(obj)

	removed, err := mgr.Delete(context.Background(), "kueue-v12", "")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if !removed {
		t.Error("Delete should return true for existing rollout")
	}
}

func TestDeleteRolloutNotFound(t *testing.T) {
	mgr := newManager()

	removed, err := mgr.Delete(context.Background(), "nonexistent", "")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if removed {
		t.Error("Delete should return false for nonexistent rollout")
	}
}

func TestDeleteRolloutError(t *testing.T) {
	obj := mwrs("kueue-v12", DefaultNamespace, "Progressive")
	c := fakeClient(obj)
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("delete", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("delete blocked")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.Delete(context.Background(), "kueue-v12", "")
	if err == nil {
		t.Fatal("expected error from Delete when delete fails")
	}
}

func TestUpdateStrategy(t *testing.T) {
	obj := mwrs("kueue-v12", DefaultNamespace, "All")
	mgr := newManager(obj)

	strategy := StrategyOpts{
		Type:           "Progressive",
		MaxConcurrency: 3,
		MaxFailures:    "20%",
	}
	if err := mgr.UpdateStrategy(context.Background(), "kueue-v12", "", strategy); err != nil {
		t.Fatalf("UpdateStrategy failed: %v", err)
	}

	updated, err := mgr.Get(context.Background(), "kueue-v12", "")
	if err != nil {
		t.Fatalf("Get after update failed: %v", err)
	}
	if updated.Strategy != "Progressive" {
		t.Errorf("expected strategy Progressive, got %s", updated.Strategy)
	}
}

func TestUpdateStrategyNotFound(t *testing.T) {
	mgr := newManager()
	strategy := StrategyOpts{Type: "Progressive"}
	err := mgr.UpdateStrategy(context.Background(), "nonexistent", "", strategy)
	if err == nil {
		t.Fatal("expected error for nonexistent rollout")
	}
}

func TestUpdateStrategyNoPlacementRefs(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1alpha1",
			"kind":       "ManifestWorkReplicaSet",
			"metadata": map[string]interface{}{
				"name":      "no-refs",
				"namespace": DefaultNamespace,
			},
			"spec": map[string]interface{}{
				"placementRefs": []interface{}{},
			},
		},
	}
	mgr := newManager(obj)

	strategy := StrategyOpts{Type: "Progressive"}
	err := mgr.UpdateStrategy(context.Background(), "no-refs", "", strategy)
	if err == nil {
		t.Fatal("expected error for empty placementRefs")
	}
}

func TestParseRolloutInfoComplete(t *testing.T) {
	obj := mwrsWithStatus("test", DefaultNamespace, 10, 10, 0)
	info := parseRolloutInfo("test", obj.Object)
	if info.Status != "Complete" {
		t.Errorf("expected Complete, got %s", info.Status)
	}
}

func TestParseRolloutInfoDegraded(t *testing.T) {
	obj := mwrsWithStatus("test", DefaultNamespace, 8, 10, 2)
	info := parseRolloutInfo("test", obj.Object)
	if info.Status != "Degraded" {
		t.Errorf("expected Degraded, got %s", info.Status)
	}
}

func TestParseRolloutInfoPending(t *testing.T) {
	obj := mwrs("test", DefaultNamespace, "Progressive")
	info := parseRolloutInfo("test", obj.Object)
	if info.Status != "Pending" {
		t.Errorf("expected Pending, got %s", info.Status)
	}
}

func TestParseRolloutInfoPlacementNotFound(t *testing.T) {
	obj := mwrs("test", DefaultNamespace, "Progressive")
	obj.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"type":   "PlacementVerified",
				"status": "False",
			},
		},
	}
	info := parseRolloutInfo("test", obj.Object)
	if info.Status != "PlacementNotFound" {
		t.Errorf("expected PlacementNotFound, got %s", info.Status)
	}
}

func TestBuildManifestWorkReplicaSet(t *testing.T) {
	opts := RolloutOpts{
		Name:             "kueue-v12",
		Namespace:        DefaultNamespace,
		PlacementName:    "gpu-placement",
		Strategy:         "Progressive",
		MaxConcurrency:   2,
		MaxFailures:      "10%",
		ProgressDeadline: "10m",
		Manifests: []map[string]interface{}{
			{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]interface{}{"name": "test"}},
		},
	}
	obj := buildManifestWorkReplicaSet(opts)
	if obj.GetName() != "kueue-v12" {
		t.Errorf("expected name kueue-v12, got %s", obj.GetName())
	}

	refs, _, _ := unstructured.NestedSlice(obj.Object, "spec", "placementRefs")
	if len(refs) != 1 {
		t.Fatalf("expected 1 placementRef, got %d", len(refs))
	}
	ref := refs[0].(map[string]interface{})
	if ref["name"] != "gpu-placement" {
		t.Errorf("expected placement gpu-placement, got %v", ref["name"])
	}
	rs := ref["rolloutStrategy"].(map[string]interface{})
	if rs["type"] != "Progressive" {
		t.Errorf("expected Progressive, got %v", rs["type"])
	}
	prog := rs["progressive"].(map[string]interface{})
	if prog["maxConcurrency"] != int64(2) {
		t.Errorf("expected maxConcurrency=2, got %v", prog["maxConcurrency"])
	}
	if prog["maxFailures"] != "10%" {
		t.Errorf("expected maxFailures=10%%, got %v", prog["maxFailures"])
	}
}

func TestBuildRolloutStrategyAll(t *testing.T) {
	strategy := buildRolloutStrategy("All", 0, "", "")
	if strategy["type"] != "All" {
		t.Errorf("expected All, got %v", strategy["type"])
	}
	if _, ok := strategy["progressive"]; ok {
		t.Error("All strategy should not have progressive config")
	}
}

func TestBuildRolloutStrategyProgressivePerGroup(t *testing.T) {
	strategy := buildRolloutStrategy("ProgressivePerGroup", 5, "20%", "15m")
	if strategy["type"] != "ProgressivePerGroup" {
		t.Errorf("expected ProgressivePerGroup, got %v", strategy["type"])
	}
	ppg, ok := strategy["progressivePerGroup"].(map[string]interface{})
	if !ok {
		t.Fatal("expected progressivePerGroup config")
	}
	if ppg["maxConcurrency"] != int64(5) {
		t.Errorf("expected maxConcurrency=5, got %v", ppg["maxConcurrency"])
	}
}

func TestCreateWithManifests(t *testing.T) {
	mgr := newManager()
	opts := RolloutOpts{
		Name:          "with-manifests",
		PlacementName: "my-placement",
		Strategy:      "All",
		Manifests: []map[string]interface{}{
			{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]interface{}{"name": "cm1"}},
			{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]interface{}{"name": "cm2"}},
		},
	}
	if err := mgr.Create(context.Background(), opts); err != nil {
		t.Fatalf("Create with manifests failed: %v", err)
	}
}

func TestCreateError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("create blocked")
	})
	mgr := New(c, config.Config{}, discardLogger)

	opts := RolloutOpts{
		Name:          "fail",
		PlacementName: "test",
	}
	err := mgr.Create(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error from Create")
	}
}

func TestListError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("list blocked")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.List(context.Background(), "")
	if err == nil {
		t.Fatal("expected error from List")
	}
}
