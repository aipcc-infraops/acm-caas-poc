package observability

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

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRNamespace:                 "NamespaceList",
			client.GVRPersistentVolumeClaim:     "PersistentVolumeClaimList",
			client.GVRDeployment:                "DeploymentList",
			client.GVRService:                   "ServiceList",
			client.GVRSecret:                    "SecretList",
			client.GVRMultiClusterObservability: "MultiClusterObservabilityList",
			client.GVRConfigMap:                 "ConfigMapList",
			client.GVRManagedClusterAddOn:       "ManagedClusterAddOnList",
			client.GVRObjectBucketClaim:         "ObjectBucketClaimList",
		}, objs...)
	return &client.Client{Dynamic: fake}
}

func TestSetupCreatesAllResources(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	if err := mgr.Setup(context.Background()); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	ctx := context.Background()

	if _, err := c.Get(ctx, client.GVRNamespace, "", Namespace); err != nil {
		t.Errorf("namespace not created: %v", err)
	}
	if _, err := c.Get(ctx, client.GVRPersistentVolumeClaim, Namespace, MinIOName); err != nil {
		t.Errorf("PVC not created: %v", err)
	}
	if _, err := c.Get(ctx, client.GVRDeployment, Namespace, MinIOName); err != nil {
		t.Errorf("Deployment not created: %v", err)
	}
	if _, err := c.Get(ctx, client.GVRService, Namespace, MinIOName); err != nil {
		t.Errorf("Service not created: %v", err)
	}
	if _, err := c.Get(ctx, client.GVRSecret, Namespace, SecretName); err != nil {
		t.Errorf("Secret not created: %v", err)
	}
	if _, err := c.Get(ctx, client.GVRMultiClusterObservability, "", MCOName); err != nil {
		t.Errorf("MCO not created: %v", err)
	}
}

func TestSetupIsIdempotent(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	if err := mgr.Setup(context.Background()); err != nil {
		t.Fatalf("first Setup failed: %v", err)
	}
	if err := mgr.Setup(context.Background()); err != nil {
		t.Fatalf("second Setup failed (not idempotent): %v", err)
	}
}

func TestTeardownRemovesAllResources(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	if err := mgr.Setup(context.Background()); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	if err := mgr.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown failed: %v", err)
	}

	ctx := context.Background()

	if _, err := c.Get(ctx, client.GVRDeployment, Namespace, MinIOName); err == nil {
		t.Error("Deployment still exists after teardown")
	}
	if _, err := c.Get(ctx, client.GVRService, Namespace, MinIOName); err == nil {
		t.Error("Service still exists after teardown")
	}
	if _, err := c.Get(ctx, client.GVRSecret, Namespace, SecretName); err == nil {
		t.Error("Secret still exists after teardown")
	}
	if _, err := c.Get(ctx, client.GVRMultiClusterObservability, "", MCOName); err == nil {
		t.Error("MCO still exists after teardown")
	}
}

func TestTeardownIsIdempotent(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	if err := mgr.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown on empty cluster failed (not idempotent): %v", err)
	}
}

func TestStatusNotInstalled(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	status, err := mgr.Status(context.Background())
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status != "NotInstalled" {
		t.Errorf("status = %q, want NotInstalled", status)
	}
}

func TestStatusPending(t *testing.T) {
	mco := &unstructured.Unstructured{}
	mco.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "observability.open-cluster-management.io", Version: "v1beta2", Kind: "MultiClusterObservability",
	})
	mco.SetName(MCOName)

	c := fakeClient(mco)
	mgr := New(c, config.Config{}, discardLogger)

	status, err := mgr.Status(context.Background())
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status != "Pending" {
		t.Errorf("status = %q, want Pending", status)
	}
}

func TestStatusReady(t *testing.T) {
	mco := &unstructured.Unstructured{}
	mco.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "observability.open-cluster-management.io", Version: "v1beta2", Kind: "MultiClusterObservability",
	})
	mco.SetName(MCOName)
	mco.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"type":   "Ready",
				"status": "True",
			},
		},
	}

	c := fakeClient(mco)
	mgr := New(c, config.Config{}, discardLogger)

	status, err := mgr.Status(context.Background())
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status != "Ready" {
		t.Errorf("status = %q, want Ready", status)
	}
}

func TestStatusProgressing(t *testing.T) {
	mco := &unstructured.Unstructured{}
	mco.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "observability.open-cluster-management.io", Version: "v1beta2", Kind: "MultiClusterObservability",
	})
	mco.SetName(MCOName)
	mco.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"type":   "Ready",
				"status": "False",
			},
		},
	}

	c := fakeClient(mco)
	mgr := New(c, config.Config{}, discardLogger)

	status, err := mgr.Status(context.Background())
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status != "Progressing" {
		t.Errorf("status = %q, want Progressing", status)
	}
}

func TestStatusGetError(t *testing.T) {
	c := fakeClient()
	fake := c.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("get", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("api unavailable")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.Status(context.Background())
	if err == nil {
		t.Fatal("expected error from Status when API fails")
	}
}

func TestSetupStepError(t *testing.T) {
	c := fakeClient()
	fake := c.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("create", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("create blocked")
	})
	fake.PrependReactor("get", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("get blocked")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.Setup(context.Background())
	if err == nil {
		t.Fatal("expected error from Setup when API fails")
	}
}

func TestTeardownStepError(t *testing.T) {
	c := fakeClient()
	fake := c.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("delete", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("delete blocked")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.Teardown(context.Background())
	if err == nil {
		t.Fatal("expected error from Teardown when delete fails")
	}
}

func TestConfigurePullSecret(t *testing.T) {
	src := &unstructured.Unstructured{}
	src.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"})
	src.SetName("pull-secret")
	src.SetNamespace(PullSecretSourceNS)
	src.Object["type"] = "kubernetes.io/dockerconfigjson"
	src.Object["data"] = map[string]interface{}{".dockerconfigjson": "dGVzdA=="}

	c := fakeClient(src)
	mgr := New(c, config.Config{}, discardLogger)

	if err := mgr.ensureNamespace(context.Background()); err != nil {
		t.Fatalf("namespace: %v", err)
	}
	if err := mgr.ConfigurePullSecret(context.Background()); err != nil {
		t.Fatalf("ConfigurePullSecret failed: %v", err)
	}
	if _, err := c.Get(context.Background(), client.GVRSecret, Namespace, PullSecretName); err != nil {
		t.Errorf("pull secret not found: %v", err)
	}
}

func TestConfigureOBCStorage(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	if err := mgr.ConfigureOBCStorage(context.Background(), StorageOpts{Type: "obc", StorageClass: "gp3-csi"}); err != nil {
		t.Fatalf("ConfigureOBCStorage failed: %v", err)
	}
	if _, err := c.Get(context.Background(), client.GVRObjectBucketClaim, Namespace, OBCName); err != nil {
		t.Errorf("OBC not found: %v", err)
	}
}

func TestDeployCustomRules(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	rules := "groups:\n- name: custom\n  rules:\n  - alert: HighCPU\n    expr: cpu > 80"
	if err := mgr.DeployCustomRules(context.Background(), CustomRuleOpts{Rules: rules}); err != nil {
		t.Fatalf("DeployCustomRules failed: %v", err)
	}
	if _, err := c.Get(context.Background(), client.GVRConfigMap, Namespace, CustomRulesCM); err != nil {
		t.Errorf("custom rules CM not found: %v", err)
	}
}

func TestRemoveCustomRules(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	_ = mgr.DeployCustomRules(context.Background(), CustomRuleOpts{Rules: "test"})
	if err := mgr.RemoveCustomRules(context.Background()); err != nil {
		t.Fatalf("RemoveCustomRules failed: %v", err)
	}
	_, err := c.Get(context.Background(), client.GVRConfigMap, Namespace, CustomRulesCM)
	if err == nil {
		t.Error("custom rules CM still exists")
	}
}

func TestDeployDashboard(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	if err := mgr.DeployDashboard(context.Background(), DashboardOpts{Name: "gpu-overview", JSON: `{"title":"GPU"}`}); err != nil {
		t.Fatalf("DeployDashboard failed: %v", err)
	}
	obj, err := c.Get(context.Background(), client.GVRConfigMap, Namespace, "gpu-overview")
	if err != nil {
		t.Fatalf("dashboard CM not found: %v", err)
	}
	labels := obj.GetLabels()
	if labels[DashboardLabelKey] != DashboardLabelValue {
		t.Errorf("missing dashboard label")
	}
}

func TestRemoveDashboard(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	_ = mgr.DeployDashboard(context.Background(), DashboardOpts{Name: "temp", JSON: "{}"})
	if err := mgr.RemoveDashboard(context.Background(), "temp"); err != nil {
		t.Fatalf("RemoveDashboard failed: %v", err)
	}
}

func TestConfigureMetricsAllowlist(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	if err := mgr.ConfigureMetricsAllowlist(context.Background(), MetricsOpts{Metrics: []string{"node_cpu_seconds_total", "container_memory_rss"}}); err != nil {
		t.Fatalf("ConfigureMetricsAllowlist failed: %v", err)
	}
	if _, err := c.Get(context.Background(), client.GVRConfigMap, Namespace, MetricsAllowlistCM); err != nil {
		t.Errorf("metrics allowlist CM not found: %v", err)
	}
}

func TestListAddonHealth(t *testing.T) {
	addon := &unstructured.Unstructured{}
	addon.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "addon.open-cluster-management.io", Version: "v1alpha1", Kind: "ManagedClusterAddOn",
	})
	addon.SetName("observability-controller")
	addon.SetNamespace("spoke1")
	addon.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{"type": "Available", "status": "True"},
			map[string]interface{}{"type": "Degraded", "status": "False"},
		},
	}

	c := fakeClient(addon)
	mgr := New(c, config.Config{}, discardLogger)

	health, err := mgr.ListAddonHealth(context.Background())
	if err != nil {
		t.Fatalf("ListAddonHealth failed: %v", err)
	}
	if len(health) != 1 {
		t.Fatalf("got %d addons, want 1", len(health))
	}
	if !health[0].Available {
		t.Error("expected Available=true")
	}
	if health[0].Degraded {
		t.Error("expected Degraded=false")
	}
}

func TestListAddonHealthEmpty(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	health, err := mgr.ListAddonHealth(context.Background())
	if err != nil {
		t.Fatalf("ListAddonHealth failed: %v", err)
	}
	if len(health) != 0 {
		t.Errorf("got %d, want 0", len(health))
	}
}

func TestConfigureRetention(t *testing.T) {
	mco := &unstructured.Unstructured{}
	mco.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "observability.open-cluster-management.io", Version: "v1beta2", Kind: "MultiClusterObservability",
	})
	mco.SetName(MCOName)
	mco.Object["spec"] = map[string]interface{}{}

	c := fakeClient(mco)
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.ConfigureRetention(context.Background(), RetentionOpts{RetentionInLocal: "24h", BlockDuration: "2h", DeleteDelay: "48h"})
	if err != nil {
		t.Fatalf("ConfigureRetention failed: %v", err)
	}
	obj, _ := c.Get(context.Background(), client.GVRMultiClusterObservability, "", MCOName)
	spec := obj.Object["spec"].(map[string]interface{})
	ret := spec["retentionConfig"].(map[string]interface{})
	if ret["retentionInLocal"] != "24h" {
		t.Errorf("retentionInLocal = %v, want 24h", ret["retentionInLocal"])
	}
}

func TestStatusConditionBadType(t *testing.T) {
	mco := &unstructured.Unstructured{}
	mco.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "observability.open-cluster-management.io", Version: "v1beta2", Kind: "MultiClusterObservability",
	})
	mco.SetName(MCOName)
	mco.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			"not-a-map",
			map[string]interface{}{
				"type":   "SomethingElse",
				"status": "True",
			},
		},
	}

	c := fakeClient(mco)
	mgr := New(c, config.Config{}, discardLogger)

	status, err := mgr.Status(context.Background())
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status != "Progressing" {
		t.Errorf("status = %q, want Progressing (no Ready condition)", status)
	}
}
