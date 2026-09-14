package registry

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

// gvrListKinds maps every GVR used by the registry package to its list kind
// so that the fake dynamic client can handle List calls.
var gvrListKinds = map[schema.GroupVersionResource]string{
	client.GVRManifestWork:                 "ManifestWorkList",
	client.GVRManagedClusterSetBinding:     "ManagedClusterSetBindingList",
	client.GVRPlacement:                    "PlacementList",
	client.GVRSecret:                       "SecretList",
	client.GVRManagedClusterImageRegistry:  "ManagedClusterImageRegistryList",
}

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrListKinds, objs...)
	return &client.Client{Dynamic: fake}
}

func fakeManager(objs ...runtime.Object) *Manager {
	return New(fakeClient(objs...), config.Config{})
}

// manifestWork creates a ManifestWork with embedded container images.
func manifestWork(namespace, name string, images ...string) *unstructured.Unstructured {
	containers := make([]interface{}, len(images))
	for i, img := range images {
		containers[i] = map[string]interface{}{
			"name":  "container-" + img[:5],
			"image": img,
		}
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{
						map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Deployment",
							"spec": map[string]interface{}{
								"template": map[string]interface{}{
									"spec": map[string]interface{}{
										"containers": containers,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	return obj
}

// imageRegistry creates a ManagedClusterImageRegistry resource.
func imageRegistry(namespace, name string, registries []RegistryMapping) *unstructured.Unstructured {
	regs := make([]interface{}, len(registries))
	for i, r := range registries {
		regs[i] = map[string]interface{}{
			"source": r.Source,
			"mirror": r.Mirror,
		}
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "imageregistry.open-cluster-management.io/v1alpha1",
			"kind":       "ManagedClusterImageRegistry",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"registries": regs,
			},
		},
	}
}

// ---------------------------------------------------------------------------
// ListRequiredImages
// ---------------------------------------------------------------------------

func TestListRequiredImagesSuccess(t *testing.T) {
	mw := manifestWork("spoke-1", "klusterlet",
		"registry.redhat.io/multicluster-engine/reg-op@sha256:abc",
		"registry.redhat.io/rhacm2/search@sha256:def",
	)
	mgr := fakeManager(mw)

	images, err := mgr.ListRequiredImages(context.Background(), "spoke-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("got %d images, want 2", len(images))
	}
	if images[0].ManifestWork != "klusterlet" {
		t.Errorf("ManifestWork = %q, want %q", images[0].ManifestWork, "klusterlet")
	}
}

func TestListRequiredImagesDeduplicates(t *testing.T) {
	// Same image in two different ManifestWorks should appear only once.
	img := "registry.redhat.io/multicluster-engine/reg-op@sha256:abc"
	mw1 := manifestWork("spoke-1", "klusterlet", img)
	mw2 := manifestWork("spoke-1", "addon-search", img)
	mgr := fakeManager(mw1, mw2)

	images, err := mgr.ListRequiredImages(context.Background(), "spoke-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("got %d images, want 1 (deduplication)", len(images))
	}
}

func TestListRequiredImagesEmptyCluster(t *testing.T) {
	mgr := fakeManager()

	images, err := mgr.ListRequiredImages(context.Background(), "empty-cluster")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(images) != 0 {
		t.Errorf("got %d images, want 0", len(images))
	}
}

func TestListRequiredImagesListError(t *testing.T) {
	c := fakeClient()
	// Inject a reactor that forces List to fail.
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "manifestworks", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, &fakeError{msg: "connection refused"}
	})
	mgr := New(c, config.Config{})

	_, err := mgr.ListRequiredImages(context.Background(), "spoke-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestListRequiredImagesManifestWithNoContainers(t *testing.T) {
	// A ManifestWork that has manifests but no container images.
	mw := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      "config-only",
				"namespace": "spoke-1",
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{
						map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "ConfigMap",
							"metadata": map[string]interface{}{
								"name": "my-config",
							},
						},
					},
				},
			},
		},
	}
	mgr := fakeManager(mw)

	images, err := mgr.ListRequiredImages(context.Background(), "spoke-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(images) != 0 {
		t.Errorf("got %d images, want 0", len(images))
	}
}

// ---------------------------------------------------------------------------
// ConfigureMirror
// ---------------------------------------------------------------------------

func TestConfigureMirrorSuccessWithPullSecret(t *testing.T) {
	// Create a temp pull secret file.
	dir := t.TempDir()
	psPath := filepath.Join(dir, "pull-secret.json")
	psData := map[string]interface{}{
		"auths": map[string]interface{}{
			"quay.io": map[string]interface{}{
				"auth": "dGVzdDp0ZXN0",
			},
		},
	}
	data, _ := json.Marshal(psData)
	if err := os.WriteFile(psPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	mgr := fakeManager()
	err := mgr.ConfigureMirror(context.Background(), MirrorConfig{
		ClusterName:    "spoke-1",
		MirrorRegistry: "quay.io/myorg",
		PullSecretPath: psPath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfigureMirrorSuccessWithoutPullSecret(t *testing.T) {
	mgr := fakeManager()
	err := mgr.ConfigureMirror(context.Background(), MirrorConfig{
		ClusterName:    "spoke-1",
		MirrorRegistry: "quay.io/myorg",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfigureMirrorWithCustomRegistries(t *testing.T) {
	mgr := fakeManager()
	err := mgr.ConfigureMirror(context.Background(), MirrorConfig{
		ClusterName: "spoke-1",
		Registries: []RegistryMapping{
			{Source: "docker.io/library", Mirror: "mirror.local/library"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfigureMirrorClusterSetBindingError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "managedclustersetbindings", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, &fakeError{msg: "forbidden"}
	})
	mgr := New(c, config.Config{})

	err := mgr.ConfigureMirror(context.Background(), MirrorConfig{
		ClusterName:    "spoke-1",
		MirrorRegistry: "quay.io/myorg",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !containsStr(err.Error(), "ManagedClusterSetBinding") {
		t.Errorf("error should mention ManagedClusterSetBinding: %v", err)
	}
}

func TestConfigureMirrorPlacementError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "placements", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, &fakeError{msg: "forbidden"}
	})
	mgr := New(c, config.Config{})

	err := mgr.ConfigureMirror(context.Background(), MirrorConfig{
		ClusterName:    "spoke-1",
		MirrorRegistry: "quay.io/myorg",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !containsStr(err.Error(), "Placement") {
		t.Errorf("error should mention Placement: %v", err)
	}
}

func TestConfigureMirrorPullSecretFileNotFound(t *testing.T) {
	mgr := fakeManager()
	err := mgr.ConfigureMirror(context.Background(), MirrorConfig{
		ClusterName:    "spoke-1",
		MirrorRegistry: "quay.io/myorg",
		PullSecretPath: "/nonexistent/pull-secret.json",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !containsStr(err.Error(), "pull secret") {
		t.Errorf("error should mention pull secret: %v", err)
	}
}

func TestConfigureMirrorPullSecretInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	psPath := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(psPath, []byte("not json{{{"), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := fakeManager()
	err := mgr.ConfigureMirror(context.Background(), MirrorConfig{
		ClusterName:    "spoke-1",
		MirrorRegistry: "quay.io/myorg",
		PullSecretPath: psPath,
	})
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !containsStr(err.Error(), "pull secret") {
		t.Errorf("error should mention pull secret: %v", err)
	}
}

func TestConfigureMirrorImageRegistryError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "managedclusterimageregistries", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, &fakeError{msg: "quota exceeded"}
	})
	mgr := New(c, config.Config{})

	err := mgr.ConfigureMirror(context.Background(), MirrorConfig{
		ClusterName:    "spoke-1",
		MirrorRegistry: "quay.io/myorg",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !containsStr(err.Error(), "ManagedClusterImageRegistry") {
		t.Errorf("error should mention ManagedClusterImageRegistry: %v", err)
	}
}

// ---------------------------------------------------------------------------
// RemoveMirror
// ---------------------------------------------------------------------------

func TestRemoveMirrorSuccess(t *testing.T) {
	// Pre-create all resources that RemoveMirror will delete.
	mcir := imageRegistry("spoke-1", "spoke-1-image-registry", nil)

	placement := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta1",
			"kind":       "Placement",
			"metadata": map[string]interface{}{
				"name":      "spoke-1-registry-placement",
				"namespace": "spoke-1",
			},
		},
	}
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "spoke-1-registry-pull-secret",
				"namespace": "spoke-1",
			},
		},
	}
	binding := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta2",
			"kind":       "ManagedClusterSetBinding",
			"metadata": map[string]interface{}{
				"name":      "default",
				"namespace": "spoke-1",
			},
		},
	}

	mgr := fakeManager(mcir, placement, secret, binding)
	err := mgr.RemoveMirror(context.Background(), "spoke-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemoveMirrorToleratesNotFound(t *testing.T) {
	// Nothing exists -- all deletes should return NotFound, which is tolerated.
	mgr := fakeManager()
	err := mgr.RemoveMirror(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error (should tolerate NotFound): %v", err)
	}
}

func TestRemoveMirrorImageRegistryError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("delete", "managedclusterimageregistries", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, &fakeError{msg: "server error"}
	})
	mgr := New(c, config.Config{})

	err := mgr.RemoveMirror(context.Background(), "spoke-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !containsStr(err.Error(), "ManagedClusterImageRegistry") {
		t.Errorf("error should mention ManagedClusterImageRegistry: %v", err)
	}
}

func TestRemoveMirrorPlacementError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("delete", "placements", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, &fakeError{msg: "server error"}
	})
	mgr := New(c, config.Config{})

	err := mgr.RemoveMirror(context.Background(), "spoke-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !containsStr(err.Error(), "Placement") {
		t.Errorf("error should mention Placement: %v", err)
	}
}

func TestRemoveMirrorSecretError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("delete", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, &fakeError{msg: "server error"}
	})
	mgr := New(c, config.Config{})

	err := mgr.RemoveMirror(context.Background(), "spoke-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !containsStr(err.Error(), "pull secret") {
		t.Errorf("error should mention pull secret: %v", err)
	}
}

func TestRemoveMirrorBindingError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("delete", "managedclustersetbindings", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, &fakeError{msg: "server error"}
	})
	mgr := New(c, config.Config{})

	err := mgr.RemoveMirror(context.Background(), "spoke-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !containsStr(err.Error(), "ManagedClusterSetBinding") {
		t.Errorf("error should mention ManagedClusterSetBinding: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetMirrorStatus
// ---------------------------------------------------------------------------

func TestGetMirrorStatusFound(t *testing.T) {
	regs := []RegistryMapping{
		{Source: "registry.redhat.io/multicluster-engine", Mirror: "quay.io/myorg/multicluster-engine"},
		{Source: "registry.redhat.io/rhacm2", Mirror: "quay.io/myorg/rhacm2"},
	}
	mcir := imageRegistry("spoke-1", "spoke-1-image-registry", regs)
	mgr := fakeManager(mcir)

	status, err := mgr.GetMirrorStatus(context.Background(), "spoke-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Configured {
		t.Error("expected Configured = true")
	}
	if status.ClusterName != "spoke-1" {
		t.Errorf("ClusterName = %q, want %q", status.ClusterName, "spoke-1")
	}
	if len(status.Registries) != 2 {
		t.Fatalf("got %d registries, want 2", len(status.Registries))
	}
	if status.Registries[0].Source != "registry.redhat.io/multicluster-engine" {
		t.Errorf("unexpected source: %s", status.Registries[0].Source)
	}
}

func TestGetMirrorStatusNotFound(t *testing.T) {
	mgr := fakeManager()

	status, err := mgr.GetMirrorStatus(context.Background(), "spoke-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Configured {
		t.Error("expected Configured = false")
	}
	if len(status.Registries) != 0 {
		t.Error("expected no registries")
	}
}

func TestGetMirrorStatusError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("get", "managedclusterimageregistries", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, &fakeError{msg: "connection refused"}
	})
	mgr := New(c, config.Config{})

	_, err := mgr.GetMirrorStatus(context.Background(), "spoke-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetMirrorStatusEmptyRegistries(t *testing.T) {
	mcir := imageRegistry("spoke-1", "spoke-1-image-registry", nil)
	mgr := fakeManager(mcir)

	status, err := mgr.GetMirrorStatus(context.Background(), "spoke-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Configured {
		t.Error("expected Configured = true")
	}
	if len(status.Registries) != 0 {
		t.Errorf("expected 0 registries, got %d", len(status.Registries))
	}
}

func TestGetMirrorStatusRegistryWithBadEntry(t *testing.T) {
	// Registry list contains a non-map entry which should be skipped.
	mcir := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "imageregistry.open-cluster-management.io/v1alpha1",
			"kind":       "ManagedClusterImageRegistry",
			"metadata": map[string]interface{}{
				"name":      "spoke-1-image-registry",
				"namespace": "spoke-1",
			},
			"spec": map[string]interface{}{
				"registries": []interface{}{
					"not-a-map",
					map[string]interface{}{
						"source": "registry.redhat.io/multicluster-engine",
						"mirror": "quay.io/myorg/multicluster-engine",
					},
				},
			},
		},
	}
	mgr := fakeManager(mcir)

	status, err := mgr.GetMirrorStatus(context.Background(), "spoke-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(status.Registries) != 1 {
		t.Errorf("expected 1 registry (bad entry skipped), got %d", len(status.Registries))
	}
}

// ---------------------------------------------------------------------------
// ensureClusterSetBinding
// ---------------------------------------------------------------------------

func TestEnsureClusterSetBindingAlreadyExists(t *testing.T) {
	existing := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta2",
			"kind":       "ManagedClusterSetBinding",
			"metadata": map[string]interface{}{
				"name":      "default",
				"namespace": "spoke-1",
			},
		},
	}
	mgr := fakeManager(existing)

	err := mgr.ensureClusterSetBinding(context.Background(), "spoke-1")
	if err != nil {
		t.Fatalf("should not error on AlreadyExists: %v", err)
	}
}

func TestEnsureClusterSetBindingCreatesNew(t *testing.T) {
	mgr := fakeManager()
	err := mgr.ensureClusterSetBinding(context.Background(), "spoke-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureClusterSetBindingError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "managedclustersetbindings", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, &fakeError{msg: "forbidden"}
	})
	mgr := New(c, config.Config{})

	err := mgr.ensureClusterSetBinding(context.Background(), "spoke-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// createPlacement
// ---------------------------------------------------------------------------

func TestCreatePlacementAlreadyExists(t *testing.T) {
	existing := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta1",
			"kind":       "Placement",
			"metadata": map[string]interface{}{
				"name":      "spoke-1-registry-placement",
				"namespace": "spoke-1",
			},
		},
	}
	mgr := fakeManager(existing)

	err := mgr.createPlacement(context.Background(), "spoke-1", "spoke-1-registry-placement", "spoke-1")
	if err != nil {
		t.Fatalf("should not error on AlreadyExists: %v", err)
	}
}

func TestCreatePlacementCreatesNew(t *testing.T) {
	mgr := fakeManager()
	err := mgr.createPlacement(context.Background(), "spoke-1", "spoke-1-registry-placement", "spoke-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreatePlacementError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "placements", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, &fakeError{msg: "forbidden"}
	})
	mgr := New(c, config.Config{})

	err := mgr.createPlacement(context.Background(), "spoke-1", "test-placement", "spoke-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// createPullSecret
// ---------------------------------------------------------------------------

func TestCreatePullSecretSuccess(t *testing.T) {
	dir := t.TempDir()
	psPath := filepath.Join(dir, "pull-secret.json")
	psData := map[string]interface{}{
		"auths": map[string]interface{}{
			"quay.io": map[string]interface{}{"auth": "dGVzdDp0ZXN0"},
		},
	}
	data, _ := json.Marshal(psData)
	os.WriteFile(psPath, data, 0644)

	mgr := fakeManager()
	err := mgr.createPullSecret(context.Background(), "spoke-1", "test-secret", psPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreatePullSecretAlreadyExists(t *testing.T) {
	existing := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "test-secret",
				"namespace": "spoke-1",
			},
		},
	}

	dir := t.TempDir()
	psPath := filepath.Join(dir, "pull-secret.json")
	os.WriteFile(psPath, []byte(`{"auths":{}}`), 0644)

	mgr := fakeManager(existing)
	err := mgr.createPullSecret(context.Background(), "spoke-1", "test-secret", psPath)
	if err != nil {
		t.Fatalf("should not error on AlreadyExists: %v", err)
	}
}

func TestCreatePullSecretFileNotFound(t *testing.T) {
	mgr := fakeManager()
	err := mgr.createPullSecret(context.Background(), "spoke-1", "test-secret", "/nonexistent/file.json")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCreatePullSecretInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	psPath := filepath.Join(dir, "bad.json")
	os.WriteFile(psPath, []byte("not-json{{{"), 0644)

	mgr := fakeManager()
	err := mgr.createPullSecret(context.Background(), "spoke-1", "test-secret", psPath)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestCreatePullSecretCreateError(t *testing.T) {
	dir := t.TempDir()
	psPath := filepath.Join(dir, "pull-secret.json")
	os.WriteFile(psPath, []byte(`{"auths":{}}`), 0644)

	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, &fakeError{msg: "forbidden"}
	})
	mgr := New(c, config.Config{})

	err := mgr.createPullSecret(context.Background(), "spoke-1", "test-secret", psPath)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// createImageRegistry
// ---------------------------------------------------------------------------

func TestCreateImageRegistrySuccess(t *testing.T) {
	mgr := fakeManager()
	regs := []RegistryMapping{
		{Source: "registry.redhat.io/multicluster-engine", Mirror: "quay.io/myorg/multicluster-engine"},
	}
	err := mgr.createImageRegistry(context.Background(), "spoke-1", "spoke-1", "placement", "secret", regs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateImageRegistryAlreadyExists(t *testing.T) {
	existing := imageRegistry("spoke-1", "spoke-1-image-registry", nil)
	mgr := fakeManager(existing)

	err := mgr.createImageRegistry(context.Background(), "spoke-1", "spoke-1", "placement", "secret", nil)
	if err != nil {
		t.Fatalf("should not error on AlreadyExists: %v", err)
	}
}

func TestCreateImageRegistryError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "managedclusterimageregistries", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, &fakeError{msg: "quota exceeded"}
	})
	mgr := New(c, config.Config{})

	err := mgr.createImageRegistry(context.Background(), "spoke-1", "spoke-1", "placement", "secret", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// extractImagesFromObject
// ---------------------------------------------------------------------------

func TestExtractImagesFromObject(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "klusterlet",
							"image": "registry.redhat.io/multicluster-engine/registration-operator-rhel9@sha256:abc123",
						},
						map[string]interface{}{
							"name":  "sidecar",
							"image": "registry.redhat.io/rhacm2/search-collector-rhel9@sha256:def456",
						},
					},
				},
			},
		},
	}

	images := extractImagesFromObject(obj)

	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d: %v", len(images), images)
	}
	if images[0] != "registry.redhat.io/multicluster-engine/registration-operator-rhel9@sha256:abc123" {
		t.Errorf("unexpected first image: %s", images[0])
	}
	if images[1] != "registry.redhat.io/rhacm2/search-collector-rhel9@sha256:def456" {
		t.Errorf("unexpected second image: %s", images[1])
	}
}

func TestExtractImagesSkipsNonImageStrings(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "klusterlet",
		},
		"apiVersion": "v1",
	}

	images := extractImagesFromObject(obj)
	if len(images) != 0 {
		t.Errorf("expected no images, got %v", images)
	}
}

func TestExtractImagesSkipsStringWithoutSlash(t *testing.T) {
	// An "image" field whose value has no slash should be skipped.
	obj := map[string]interface{}{
		"image": "busybox",
	}
	images := extractImagesFromObject(obj)
	if len(images) != 0 {
		t.Errorf("expected 0 images for image without slash, got %v", images)
	}
}

func TestExtractImagesNestedDeep(t *testing.T) {
	obj := map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": []interface{}{
					map[string]interface{}{
						"image": "registry.redhat.io/deep/nested@sha256:aaa",
					},
				},
			},
		},
	}
	images := extractImagesFromObject(obj)
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
}

func TestExtractImagesEmptyObject(t *testing.T) {
	images := extractImagesFromObject(map[string]interface{}{})
	if len(images) != 0 {
		t.Errorf("expected 0 images, got %d", len(images))
	}
}

func TestExtractImagesArrayWithNonMapItems(t *testing.T) {
	// Array contains non-map items which should be silently skipped.
	obj := map[string]interface{}{
		"items": []interface{}{
			"just-a-string",
			42,
			map[string]interface{}{
				"image": "registry.redhat.io/valid/image@sha256:bbb",
			},
		},
	}
	images := extractImagesFromObject(obj)
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d: %v", len(images), images)
	}
}

// ---------------------------------------------------------------------------
// DefaultRegistryMappings
// ---------------------------------------------------------------------------

func TestDefaultRegistryMappings(t *testing.T) {
	mappings := DefaultRegistryMappings("quay.io/myorg")

	if len(mappings) != 2 {
		t.Fatalf("expected 2 mappings, got %d", len(mappings))
	}

	if mappings[0].Source != "registry.redhat.io/multicluster-engine" {
		t.Errorf("unexpected source: %s", mappings[0].Source)
	}
	if mappings[0].Mirror != "quay.io/myorg/multicluster-engine" {
		t.Errorf("unexpected mirror: %s", mappings[0].Mirror)
	}
	if mappings[1].Source != "registry.redhat.io/rhacm2" {
		t.Errorf("unexpected source: %s", mappings[1].Source)
	}
	if mappings[1].Mirror != "quay.io/myorg/rhacm2" {
		t.Errorf("unexpected mirror: %s", mappings[1].Mirror)
	}
}

// ---------------------------------------------------------------------------
// GenerateMirrorScript
// ---------------------------------------------------------------------------

func TestGenerateMirrorScript(t *testing.T) {
	images := []RequiredImage{
		{Image: "registry.redhat.io/multicluster-engine/registration-operator-rhel9@sha256:abc", ManifestWork: "test-klusterlet"},
		{Image: "registry.redhat.io/rhacm2/search-collector-rhel9@sha256:def", ManifestWork: "addon-search"},
	}

	script := GenerateMirrorScript(images, "quay.io/myorg")

	if !containsStr(script, "skopeo copy") {
		t.Error("script should contain skopeo copy commands")
	}
	if !containsStr(script, "${TARGET_REGISTRY}/multicluster-engine/registration-operator-rhel9@sha256:abc") {
		t.Error("script should contain target image for MCE")
	}
	if !containsStr(script, "${TARGET_REGISTRY}/rhacm2/search-collector-rhel9@sha256:def") {
		t.Error("script should contain target image for RHACM")
	}
	if !containsStr(script, "#!/bin/bash") {
		t.Error("script should have bash shebang")
	}
}

func TestGenerateMirrorScriptSkipsInvalidImages(t *testing.T) {
	images := []RequiredImage{
		{Image: "no-slash-here", ManifestWork: "test"},
	}

	script := GenerateMirrorScript(images, "quay.io/myorg")

	if containsStr(script, "skopeo copy") {
		t.Error("script should not contain skopeo for images without registry path")
	}
}

func TestGenerateMirrorScriptEmptyImages(t *testing.T) {
	script := GenerateMirrorScript(nil, "quay.io/myorg")
	if !containsStr(script, "#!/bin/bash") {
		t.Error("script should still have shebang")
	}
	if containsStr(script, "skopeo copy") {
		t.Error("script should not contain skopeo commands for empty list")
	}
	if !containsStr(script, "Done") {
		t.Error("script should have closing echo")
	}
}

func TestGenerateMirrorScriptSetsTargetRegistry(t *testing.T) {
	script := GenerateMirrorScript(nil, "mirror.example.com/org")
	if !containsStr(script, `TARGET_REGISTRY="mirror.example.com/org"`) {
		t.Error("script should set TARGET_REGISTRY variable")
	}
}

// ---------------------------------------------------------------------------
// MirrorStatus defaults
// ---------------------------------------------------------------------------

func TestMirrorStatusDefaults(t *testing.T) {
	status := &MirrorStatus{ClusterName: "test", Configured: false}

	if status.Configured {
		t.Error("should not be configured by default")
	}
	if len(status.Registries) != 0 {
		t.Error("should have no registries by default")
	}
}

// ---------------------------------------------------------------------------
// New constructor
// ---------------------------------------------------------------------------

func TestNewReturnsManager(t *testing.T) {
	c := fakeClient()
	cfg := config.Config{}
	mgr := New(c, cfg)
	if mgr == nil {
		t.Fatal("New returned nil")
	}
	if mgr.client != c {
		t.Error("client not set correctly")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// fakeError implements the error interface and is NOT a k8s StatusError,
// so it won't be treated as NotFound or AlreadyExists.
type fakeError struct {
	msg string
}

func (e *fakeError) Error() string { return e.msg }

func containsStr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
