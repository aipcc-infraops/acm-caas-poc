package discovery

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

var gvrKinds = map[schema.GroupVersionResource]string{
	client.GVRDiscoveryConfig:      "DiscoveryConfigList",
	client.GVRDiscoveredCluster:    "DiscoveredClusterList",
	client.GVRSecret:               "SecretList",
	client.GVRNamespace:            "NamespaceList",
	client.GVRManagedCluster:       "ManagedClusterList",
	client.GVRKlusterletAddonConfig: "KlusterletAddonConfigList",
}

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrKinds, objs...)
	return &client.Client{Dynamic: fake}
}

func newManager(objs ...runtime.Object) *Manager {
	return New(fakeClient(objs...), config.Config{}, discardLogger)
}

func discoveredCluster(name, namespace, cloud, version, region, apiURL string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "discovery.open-cluster-management.io/v1alpha1",
			"kind":       "DiscoveredCluster",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"displayName":      name,
				"cloudProvider":    cloud,
				"openshiftVersion": version,
				"region":           region,
				"apiURL":           apiURL,
				"status":           "Active",
			},
		},
	}
}

func TestEnableDiscovery(t *testing.T) {
	mgr := newManager()
	err := mgr.EnableDiscovery(context.Background(), EnableDiscoveryOpts{
		Namespace:  "open-cluster-management",
		OCMToken:   "test-token-123",
		LastActive: 7,
		Versions:   []string{"4.14", "4.15"},
	})
	if err != nil {
		t.Fatalf("EnableDiscovery failed: %v", err)
	}

	secret, err := mgr.client.Get(context.Background(), client.GVRSecret, "open-cluster-management", "ocm-api-token")
	if err != nil {
		t.Fatalf("secret not created: %v", err)
	}
	labels := secret.GetLabels()
	if labels["acmlab.redhat.com/discovery"] != "true" {
		t.Error("secret missing discovery label")
	}

	dc, err := mgr.client.Get(context.Background(), client.GVRDiscoveryConfig, "open-cluster-management", "discovery")
	if err != nil {
		t.Fatalf("DiscoveryConfig not created: %v", err)
	}
	cred, _, _ := unstructured.NestedString(dc.Object, "spec", "credential")
	if cred != "ocm-api-token" {
		t.Errorf("expected credential 'ocm-api-token', got %q", cred)
	}
}

func TestEnableDiscoveryMissingNamespace(t *testing.T) {
	mgr := newManager()
	err := mgr.EnableDiscovery(context.Background(), EnableDiscoveryOpts{
		OCMToken: "token",
	})
	if err == nil {
		t.Fatal("expected error for missing namespace")
	}
}

func TestEnableDiscoveryMissingToken(t *testing.T) {
	mgr := newManager()
	err := mgr.EnableDiscovery(context.Background(), EnableDiscoveryOpts{
		Namespace: "ocm",
	})
	if err == nil {
		t.Fatal("expected error for missing token")
	}
}

func TestDisableDiscovery(t *testing.T) {
	mgr := newManager()
	opts := EnableDiscoveryOpts{
		Namespace:  "ocm",
		OCMToken:   "token",
		LastActive: 7,
	}
	if err := mgr.EnableDiscovery(context.Background(), opts); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := mgr.DisableDiscovery(context.Background(), "ocm"); err != nil {
		t.Fatalf("DisableDiscovery failed: %v", err)
	}
}

func TestDisableDiscoveryNotFound(t *testing.T) {
	mgr := newManager()
	err := mgr.DisableDiscovery(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent discovery config")
	}
}

func TestListDiscovered(t *testing.T) {
	dc1 := discoveredCluster("cluster-a", "ocm", "AWS", "4.14.5", "us-east-1", "https://api.cluster-a.example.com:6443")
	dc2 := discoveredCluster("cluster-b", "ocm", "GCP", "4.15.1", "us-central1", "https://api.cluster-b.example.com:6443")
	mgr := newManager(dc1, dc2)

	clusters, err := mgr.ListDiscovered(context.Background(), "ocm")
	if err != nil {
		t.Fatalf("ListDiscovered failed: %v", err)
	}
	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(clusters))
	}

	found := map[string]bool{}
	for _, c := range clusters {
		found[c.Name] = true
		if c.CloudProvider == "" {
			t.Errorf("cluster %s missing cloud provider", c.Name)
		}
	}
	if !found["cluster-a"] || !found["cluster-b"] {
		t.Errorf("missing expected clusters: %v", found)
	}
}

func TestListDiscoveredEmpty(t *testing.T) {
	mgr := newManager()
	clusters, err := mgr.ListDiscovered(context.Background(), "ocm")
	if err != nil {
		t.Fatalf("ListDiscovered failed: %v", err)
	}
	if len(clusters) != 0 {
		t.Errorf("expected 0 clusters, got %d", len(clusters))
	}
}

func TestImportDiscovered(t *testing.T) {
	dc := discoveredCluster("rosa-prod", "ocm", "AWS", "4.14.5", "us-east-1", "https://api.rosa-prod.example.com:6443")
	mgr := newManager(dc)

	err := mgr.ImportDiscovered(context.Background(), "rosa-prod", "ocm")
	if err != nil {
		t.Fatalf("ImportDiscovered failed: %v", err)
	}

	mc, err := mgr.client.Get(context.Background(), client.GVRManagedCluster, "", "rosa-prod")
	if err != nil {
		t.Fatalf("ManagedCluster not created: %v", err)
	}
	labels := mc.GetLabels()
	if labels["created-via"] != "discovery" {
		t.Error("missing created-via=discovery label")
	}

	_, err = mgr.client.Get(context.Background(), client.GVRKlusterletAddonConfig, "rosa-prod", "rosa-prod")
	if err != nil {
		t.Fatalf("KlusterletAddonConfig not created: %v", err)
	}
}

func TestImportDiscoveredNotFound(t *testing.T) {
	mgr := newManager()
	err := mgr.ImportDiscovered(context.Background(), "nonexistent", "ocm")
	if err == nil {
		t.Fatal("expected error for nonexistent discovered cluster")
	}
}

func TestDiscoveryStatus(t *testing.T) {
	mgr := newManager()
	opts := EnableDiscoveryOpts{
		Namespace:  "ocm",
		OCMToken:   "token",
		LastActive: 14,
	}
	if err := mgr.EnableDiscovery(context.Background(), opts); err != nil {
		t.Fatalf("setup: %v", err)
	}

	status, err := mgr.DiscoveryStatus(context.Background(), "ocm")
	if err != nil {
		t.Fatalf("DiscoveryStatus failed: %v", err)
	}
	if status.Namespace != "ocm" {
		t.Errorf("expected namespace 'ocm', got %q", status.Namespace)
	}
	if status.Credential != "ocm-api-token" {
		t.Errorf("expected credential 'ocm-api-token', got %q", status.Credential)
	}
	if status.LastActive != 14 {
		t.Errorf("expected lastActive 14, got %d", status.LastActive)
	}
}

func TestDiscoveryStatusNotFound(t *testing.T) {
	mgr := newManager()
	_, err := mgr.DiscoveryStatus(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent discovery config")
	}
}
