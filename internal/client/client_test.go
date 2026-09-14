package client

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func newFakeClient(objs ...runtime.Object) *Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			{Group: "test.io", Version: "v1", Resource: "things"}:  "ThingList",
			{Group: "test.io", Version: "v1", Resource: "globals"}: "GlobalList",
		}, objs...)
	return &Client{Dynamic: fake}
}

var testGVR = schema.GroupVersionResource{Group: "test.io", Version: "v1", Resource: "things"}
var testGVK = schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Thing"}
var clusterGVR = schema.GroupVersionResource{Group: "test.io", Version: "v1", Resource: "globals"}
var clusterGVK = schema.GroupVersionKind{Group: "test.io", Version: "v1", Kind: "Global"}

func newObj(name, namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(testGVK)
	obj.SetName(name)
	if namespace != "" {
		obj.SetNamespace(namespace)
	}
	return obj
}

func TestCreateAndGetNamespacedResource(t *testing.T) {
	c := newFakeClient()

	obj := newObj("test-thing", "default")
	created, err := c.Create(context.Background(), testGVR, "default", obj)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if created.GetName() != "test-thing" {
		t.Errorf("created name = %q, want %q", created.GetName(), "test-thing")
	}

	got, err := c.Get(context.Background(), testGVR, "default", "test-thing")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.GetName() != "test-thing" {
		t.Errorf("got name = %q, want %q", got.GetName(), "test-thing")
	}
}

func TestCreateAndGetClusterScopedResource(t *testing.T) {
	c := newFakeClient()

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(clusterGVK)
	obj.SetName("cluster-thing")

	_, err := c.Create(context.Background(), clusterGVR, "", obj)
	if err != nil {
		t.Fatalf("Create cluster-scoped failed: %v", err)
	}

	got, err := c.Get(context.Background(), clusterGVR, "", "cluster-thing")
	if err != nil {
		t.Fatalf("Get cluster-scoped failed: %v", err)
	}
	if got.GetName() != "cluster-thing" {
		t.Errorf("name = %q, want %q", got.GetName(), "cluster-thing")
	}
}

func TestListReturnsCreatedResources(t *testing.T) {
	c := newFakeClient()

	for _, name := range []string{"thing-1", "thing-2", "thing-3"} {
		obj := newObj(name, "ns-1")
		if _, err := c.Create(context.Background(), testGVR, "ns-1", obj); err != nil {
			t.Fatalf("Create %s failed: %v", name, err)
		}
	}

	list, err := c.List(context.Background(), testGVR, "ns-1", "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list.Items) != 3 {
		t.Errorf("List returned %d items, want 3", len(list.Items))
	}
}

func TestListWithLabelSelector(t *testing.T) {
	c := newFakeClient()

	labeled := newObj("labeled", "ns-1")
	labeled.SetLabels(map[string]string{"env": "dev"})
	if _, err := c.Create(context.Background(), testGVR, "ns-1", labeled); err != nil {
		t.Fatalf("Create labeled failed: %v", err)
	}

	unlabeled := newObj("unlabeled", "ns-1")
	if _, err := c.Create(context.Background(), testGVR, "ns-1", unlabeled); err != nil {
		t.Fatalf("Create unlabeled failed: %v", err)
	}

	list, err := c.List(context.Background(), testGVR, "ns-1", "env=dev")
	if err != nil {
		t.Fatalf("List with selector failed: %v", err)
	}
	if len(list.Items) != 1 {
		t.Errorf("List with selector returned %d items, want 1", len(list.Items))
	}
}

func TestDeleteRemovesResource(t *testing.T) {
	c := newFakeClient()

	obj := newObj("to-delete", "default")
	if _, err := c.Create(context.Background(), testGVR, "default", obj); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := c.Delete(context.Background(), testGVR, "default", "to-delete"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := c.Get(context.Background(), testGVR, "default", "to-delete")
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestDeleteNonexistentReturnsError(t *testing.T) {
	c := newFakeClient()

	err := c.Delete(context.Background(), testGVR, "default", "nonexistent")
	if err == nil {
		t.Error("expected error deleting nonexistent resource, got nil")
	}
}

func TestGetNonexistentReturnsError(t *testing.T) {
	c := newFakeClient()

	_, err := c.Get(context.Background(), testGVR, "default", "nonexistent")
	if err == nil {
		t.Error("expected error getting nonexistent resource, got nil")
	}
}

func TestPatchUpdatesResource(t *testing.T) {
	c := newFakeClient()

	obj := newObj("to-patch", "default")
	if _, err := c.Create(context.Background(), testGVR, "default", obj); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	patch := []byte(`{"metadata":{"labels":{"patched":"true"}}}`)
	patched, err := c.Patch(context.Background(), testGVR, "default", "to-patch", "application/merge-patch+json", patch)
	if err != nil {
		t.Fatalf("Patch failed: %v", err)
	}
	if patched.GetLabels()["patched"] != "true" {
		t.Errorf("label patched = %q, want %q", patched.GetLabels()["patched"], "true")
	}
}

func TestWatchReturnsWatcher(t *testing.T) {
	c := newFakeClient()

	w, err := c.Watch(context.Background(), testGVR, "default", metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}
	defer w.Stop()

	if w.ResultChan() == nil {
		t.Error("Watch returned nil channel")
	}
}

func TestListEmptyNamespaceReturnsEmpty(t *testing.T) {
	c := newFakeClient()

	list, err := c.List(context.Background(), testGVR, "empty-ns", "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("List returned %d items, want 0", len(list.Items))
	}
}

func TestUpdateModifiesResource(t *testing.T) {
	c := newFakeClient()

	obj := newObj("to-update", "default")
	if _, err := c.Create(context.Background(), testGVR, "default", obj); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	obj.SetLabels(map[string]string{"updated": "true"})
	updated, err := c.Update(context.Background(), testGVR, "default", obj)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.GetLabels()["updated"] != "true" {
		t.Errorf("label updated = %q, want %q", updated.GetLabels()["updated"], "true")
	}

	got, err := c.Get(context.Background(), testGVR, "default", "to-update")
	if err != nil {
		t.Fatalf("Get after update failed: %v", err)
	}
	if got.GetLabels()["updated"] != "true" {
		t.Errorf("persisted label updated = %q, want %q", got.GetLabels()["updated"], "true")
	}
}

func TestNewFromContextInvalidPathReturnsError(t *testing.T) {
	_, err := NewFromContext("/nonexistent/path/kubeconfig.yaml", "")
	if err == nil {
		t.Error("expected error for invalid kubeconfig path, got nil")
	}
}

func TestNewFromContextInvalidPathWithContextReturnsError(t *testing.T) {
	_, err := NewFromContext("/nonexistent/path/kubeconfig.yaml", "some-context")
	if err == nil {
		t.Error("expected error for invalid kubeconfig path with context, got nil")
	}
}

func TestGetClusterType(t *testing.T) {
	tests := []struct {
		name        string
		distType    string
		wantType    ClusterType
	}{
		{"OCP cluster", "OCP", ClusterTypeOCP},
		{"Kubernetes cluster", "kubernetes", ClusterTypeKubernetes},
		{"empty distribution", "", ClusterTypeUnknown},
		{"other distribution", "EKS", ClusterType("EKS")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			infoGVK := schema.GroupVersionKind{
				Group:   "internal.open-cluster-management.io",
				Version: "v1beta1",
				Kind:    "ManagedClusterInfo",
			}
			scheme := runtime.NewScheme()
			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(infoGVK)
			obj.SetName("test-cluster")
			obj.SetNamespace("test-cluster")
			if tt.distType != "" {
				unstructured.SetNestedField(obj.Object, tt.distType, "status", "distributionInfo", "type")
			}

			fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
				map[schema.GroupVersionResource]string{
					GVRManagedClusterInfo: "ManagedClusterInfoList",
				}, obj)
			c := &Client{Dynamic: fake}

			got, err := c.GetClusterType(context.Background(), "test-cluster")
			if err != nil {
				t.Fatalf("GetClusterType failed: %v", err)
			}
			if got != tt.wantType {
				t.Errorf("GetClusterType = %q, want %q", got, tt.wantType)
			}
		})
	}
}

func writeMinimalKubeconfig(t *testing.T, dir, context string) string {
	t.Helper()
	content := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: ` + context + `
current-context: ` + context + `
users:
- name: test-user
  user:
    token: fake-token
`
	path := filepath.Join(dir, "kubeconfig")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writing kubeconfig: %v", err)
	}
	return path
}

func TestNewFromDefaultWithEnvKubeconfig(t *testing.T) {
	dir := t.TempDir()
	kc := writeMinimalKubeconfig(t, dir, "default")
	t.Setenv("KUBECONFIG", kc)

	c, err := NewFromDefault()
	if err != nil {
		t.Fatalf("NewFromDefault failed: %v", err)
	}
	if c == nil || c.Dynamic == nil {
		t.Error("expected non-nil client with Dynamic interface")
	}
}

func TestNewFromDefaultNoKubeconfig(t *testing.T) {
	t.Setenv("KUBECONFIG", "/nonexistent/kubeconfig.yaml")
	t.Setenv("HOME", "/nonexistent-home")

	_, err := NewFromDefault()
	if err == nil {
		t.Error("expected error with no valid kubeconfig, got nil")
	}
}

func TestNewFromContextValidKubeconfig(t *testing.T) {
	dir := t.TempDir()
	kc := writeMinimalKubeconfig(t, dir, "my-ctx")

	c, err := NewFromContext(kc, "my-ctx")
	if err != nil {
		t.Fatalf("NewFromContext failed: %v", err)
	}
	if c == nil || c.Dynamic == nil {
		t.Error("expected non-nil client with Dynamic interface")
	}
}

func TestGetClusterTypeNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			GVRManagedClusterInfo: "ManagedClusterInfoList",
		})
	c := &Client{Dynamic: fake}

	got, err := c.GetClusterType(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent cluster, got nil")
	}
	if got != ClusterTypeUnknown {
		t.Errorf("got type %q, want empty string for unknown", got)
	}
}
