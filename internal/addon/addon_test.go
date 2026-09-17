package addon

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
	client.GVRClusterManagementAddOn: "ClusterManagementAddOnList",
	client.GVRAddOnDeploymentConfig:  "AddOnDeploymentConfigList",
}

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrKinds, objs...)
	return &client.Client{Dynamic: fake}
}

func newManager(objs ...runtime.Object) *Manager {
	return New(fakeClient(objs...), config.Config{}, discardLogger)
}

func clusterManagementAddOn(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "addon.open-cluster-management.io/v1alpha1",
			"kind":       "ClusterManagementAddOn",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": map[string]interface{}{
				"addOnMeta": map[string]interface{}{
					"displayName": name + " Display",
					"description": "Test add-on",
				},
			},
		},
	}
}

func deploymentConfig(name, ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "addon.open-cluster-management.io/v1alpha1",
			"kind":       "AddOnDeploymentConfig",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"customizedVariables": []interface{}{},
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

func TestListAddOnsEmpty(t *testing.T) {
	mgr := newManager()
	addons, err := mgr.ListAddOns(context.Background())
	if err != nil {
		t.Fatalf("ListAddOns failed: %v", err)
	}
	if len(addons) != 0 {
		t.Errorf("got %d addons, want 0", len(addons))
	}
}

func TestListAddOnsWithExisting(t *testing.T) {
	a1 := clusterManagementAddOn("work-manager")
	a2 := clusterManagementAddOn("cluster-proxy")
	mgr := newManager(a1, a2)

	addons, err := mgr.ListAddOns(context.Background())
	if err != nil {
		t.Fatalf("ListAddOns failed: %v", err)
	}
	if len(addons) != 2 {
		t.Errorf("got %d addons, want 2", len(addons))
	}
}

func TestListAddOnsError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "clustermanagementaddons", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("timeout")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.ListAddOns(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetAddOn(t *testing.T) {
	a := clusterManagementAddOn("work-manager")
	mgr := newManager(a)

	detail, err := mgr.GetAddOn(context.Background(), "work-manager")
	if err != nil {
		t.Fatalf("GetAddOn failed: %v", err)
	}
	if detail.Name != "work-manager" {
		t.Errorf("Name = %q", detail.Name)
	}
	if detail.DisplayName != "work-manager Display" {
		t.Errorf("DisplayName = %q", detail.DisplayName)
	}
}

func TestGetAddOnNotFound(t *testing.T) {
	mgr := newManager()
	_, err := mgr.GetAddOn(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent addon")
	}
}

func TestConfigureAddOn(t *testing.T) {
	mgr := newManager()
	opts := AddOnConfigOpts{
		Name:      "my-config",
		Namespace: DefaultNamespace,
		Values: map[string]string{
			"replica-count": "3",
			"log-level":     "debug",
		},
	}
	err := mgr.ConfigureAddOn(context.Background(), opts)
	if err != nil {
		t.Fatalf("ConfigureAddOn failed: %v", err)
	}

	_, err = mgr.client.Get(context.Background(), client.GVRAddOnDeploymentConfig, DefaultNamespace, "my-config")
	if err != nil {
		t.Fatalf("AddOnDeploymentConfig not found: %v", err)
	}
}

func TestConfigureAddOnDefaultNamespace(t *testing.T) {
	mgr := newManager()
	opts := AddOnConfigOpts{
		Name:   "my-config",
		Values: map[string]string{"key": "val"},
	}
	err := mgr.ConfigureAddOn(context.Background(), opts)
	if err != nil {
		t.Fatalf("ConfigureAddOn failed: %v", err)
	}

	_, err = mgr.client.Get(context.Background(), client.GVRAddOnDeploymentConfig, DefaultNamespace, "my-config")
	if err != nil {
		t.Fatalf("AddOnDeploymentConfig not found in default namespace: %v", err)
	}
}

func TestConfigureAddOnIdempotent(t *testing.T) {
	mgr := newManager()
	opts := AddOnConfigOpts{Name: "cfg1", Values: map[string]string{"k": "v"}}
	if err := mgr.ConfigureAddOn(context.Background(), opts); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := mgr.ConfigureAddOn(context.Background(), opts); err != nil {
		t.Fatalf("second should be idempotent: %v", err)
	}
}

func TestConfigureAddOnError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "addondeploymentconfigs", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("forbidden")
	})
	mgr := New(c, config.Config{}, discardLogger)

	opts := AddOnConfigOpts{Name: "cfg", Values: map[string]string{"k": "v"}}
	err := mgr.ConfigureAddOn(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "creating AddOnDeploymentConfig") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRemoveConfig(t *testing.T) {
	dc := deploymentConfig("my-config", DefaultNamespace)
	mgr := newManager(dc)

	err := mgr.RemoveConfig(context.Background(), "my-config", DefaultNamespace)
	if err != nil {
		t.Fatalf("RemoveConfig failed: %v", err)
	}
}

func TestRemoveConfigNotFound(t *testing.T) {
	mgr := newManager()
	err := mgr.RemoveConfig(context.Background(), "nonexistent", DefaultNamespace)
	if err == nil {
		t.Fatal("expected error for nonexistent config")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestListConfigsEmpty(t *testing.T) {
	mgr := newManager()
	configs, err := mgr.ListConfigs(context.Background())
	if err != nil {
		t.Fatalf("ListConfigs failed: %v", err)
	}
	if len(configs) != 0 {
		t.Errorf("got %d configs, want 0", len(configs))
	}
}

func TestListConfigsWithExisting(t *testing.T) {
	dc := deploymentConfig("cfg1", DefaultNamespace)
	mgr := newManager(dc)

	configs, err := mgr.ListConfigs(context.Background())
	if err != nil {
		t.Fatalf("ListConfigs failed: %v", err)
	}
	if len(configs) != 1 {
		t.Errorf("got %d configs, want 1", len(configs))
	}
}

func TestParseAddOnInfo(t *testing.T) {
	a := clusterManagementAddOn("work-manager")
	info := parseAddOnInfo(a.Object)
	if info.Name != "work-manager" {
		t.Errorf("Name = %q", info.Name)
	}
	if info.DisplayName != "work-manager Display" {
		t.Errorf("DisplayName = %q", info.DisplayName)
	}
	if info.Status != "Available" {
		t.Errorf("Status = %q", info.Status)
	}
}

func TestParseAddOnInfoNoDisplayName(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "test-addon",
		},
		"spec": map[string]interface{}{},
	}
	info := parseAddOnInfo(obj)
	if info.DisplayName != "test-addon" {
		t.Errorf("DisplayName = %q, want fallback to name", info.DisplayName)
	}
}

func TestParseConfigInfo(t *testing.T) {
	dc := deploymentConfig("my-config", "test-ns")
	info := parseConfigInfo(dc.Object)
	if info.Name != "my-config" {
		t.Errorf("Name = %q", info.Name)
	}
	if info.Namespace != "test-ns" {
		t.Errorf("Namespace = %q", info.Namespace)
	}
}

func TestBuildAddOnDeploymentConfig(t *testing.T) {
	opts := AddOnConfigOpts{
		Name:             "test-config",
		Namespace:        "test-ns",
		InstallNamespace: "custom-ns",
		Values:           map[string]string{"key1": "val1"},
	}
	cfg := buildAddOnDeploymentConfig(opts)
	if cfg.GetName() != "test-config" {
		t.Errorf("Name = %q", cfg.GetName())
	}
	if cfg.GetNamespace() != "test-ns" {
		t.Errorf("Namespace = %q", cfg.GetNamespace())
	}

	installNs, _, _ := unstructured.NestedString(cfg.Object, "spec", "agentInstallNamespace")
	if installNs != "custom-ns" {
		t.Errorf("agentInstallNamespace = %q, want custom-ns", installNs)
	}
}

func TestBuildAddOnDeploymentConfigNoInstallNs(t *testing.T) {
	opts := AddOnConfigOpts{
		Name:   "test",
		Values: map[string]string{},
	}
	cfg := buildAddOnDeploymentConfig(opts)
	_, found, _ := unstructured.NestedString(cfg.Object, "spec", "agentInstallNamespace")
	if found {
		t.Error("agentInstallNamespace should not be set when empty")
	}
}
