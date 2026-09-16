package gitops

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
	client.GVRApplicationSet: "ApplicationSetList",
}

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrKinds, objs...)
	return &client.Client{Dynamic: fake}
}

func newManager(objs ...runtime.Object) *Manager {
	return New(fakeClient(objs...), config.Config{}, discardLogger)
}

func sampleAppSet(name, ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "ApplicationSet",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":   "true",
					"acmlab.redhat.com/gitops":    "true",
					"acmlab.redhat.com/generator": "placement",
				},
			},
			"spec": map[string]interface{}{
				"generators": []interface{}{},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "{{name}}-" + name,
					},
					"spec": map[string]interface{}{
						"project": "default",
						"source": map[string]interface{}{
							"repoURL":        "https://github.com/example/repo.git",
							"path":           "manifests/base",
							"targetRevision": "main",
						},
						"destination": map[string]interface{}{
							"server":    "{{server}}",
							"namespace": "default",
						},
					},
				},
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "ResourcesUpToDate",
						"status": "True",
					},
				},
				"resources": []interface{}{
					map[string]interface{}{"name": "spoke1-myapp"},
					map[string]interface{}{"name": "spoke2-myapp"},
				},
			},
		},
	}
}

func TestNewReturnsManager(t *testing.T) {
	mgr := newManager()
	if mgr == nil {
		t.Fatal("New returned nil")
	}
}

func TestCreate(t *testing.T) {
	mgr := newManager()
	err := mgr.Create(context.Background(), AppSetOpts{
		Name:    "my-app",
		RepoURL: "https://github.com/example/repo.git",
		Path:    "manifests/base",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	obj, err := mgr.client.Get(context.Background(), client.GVRApplicationSet, DefaultNamespace, "my-app")
	if err != nil {
		t.Fatalf("ApplicationSet not found: %v", err)
	}
	if obj.GetName() != "my-app" {
		t.Errorf("name = %q, want my-app", obj.GetName())
	}
}

func TestCreateIdempotent(t *testing.T) {
	mgr := newManager()
	opts := AppSetOpts{
		Name:    "my-app",
		RepoURL: "https://github.com/example/repo.git",
		Path:    "manifests/base",
	}
	if err := mgr.Create(context.Background(), opts); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := mgr.Create(context.Background(), opts); err != nil {
		t.Fatalf("second create should be idempotent: %v", err)
	}
}

func TestCreateError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "applicationsets", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("forbidden")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.Create(context.Background(), AppSetOpts{
		Name:    "my-app",
		RepoURL: "https://github.com/example/repo.git",
		Path:    "manifests/base",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "creating ApplicationSet") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGet(t *testing.T) {
	appSet := sampleAppSet("my-app", DefaultNamespace)
	mgr := newManager(appSet)

	info, err := mgr.Get(context.Background(), "my-app", "")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if info.Name != "my-app" {
		t.Errorf("Name = %q, want my-app", info.Name)
	}
	if info.RepoURL != "https://github.com/example/repo.git" {
		t.Errorf("RepoURL = %q", info.RepoURL)
	}
	if info.Path != "manifests/base" {
		t.Errorf("Path = %q", info.Path)
	}
	if info.Generator != "placement" {
		t.Errorf("Generator = %q, want placement", info.Generator)
	}
	if info.Status != "Synced" {
		t.Errorf("Status = %q, want Synced", info.Status)
	}
	if info.AppCount != 2 {
		t.Errorf("AppCount = %d, want 2", info.AppCount)
	}
}

func TestGetNotFound(t *testing.T) {
	mgr := newManager()
	_, err := mgr.Get(context.Background(), "missing", "")
	if err == nil {
		t.Fatal("expected error for missing ApplicationSet")
	}
}

func TestList(t *testing.T) {
	a1 := sampleAppSet("app1", DefaultNamespace)
	a2 := sampleAppSet("app2", DefaultNamespace)
	mgr := newManager(a1, a2)

	infos, err := mgr.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(infos) != 2 {
		t.Errorf("got %d, want 2", len(infos))
	}
}

func TestListEmpty(t *testing.T) {
	mgr := newManager()
	infos, err := mgr.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("got %d, want 0", len(infos))
	}
}

func TestListError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "applicationsets", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("timeout")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.List(context.Background(), "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "listing ApplicationSets") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDelete(t *testing.T) {
	appSet := sampleAppSet("my-app", DefaultNamespace)
	mgr := newManager(appSet)

	removed, err := mgr.Delete(context.Background(), "my-app", "")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if !removed {
		t.Error("Delete should return true for existing ApplicationSet")
	}

	_, err = mgr.client.Get(context.Background(), client.GVRApplicationSet, DefaultNamespace, "my-app")
	if err == nil {
		t.Error("ApplicationSet should not exist after delete")
	}
}

func TestDeleteNotFound(t *testing.T) {
	mgr := newManager()
	removed, err := mgr.Delete(context.Background(), "missing", "")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if removed {
		t.Error("Delete should return false for nonexistent ApplicationSet")
	}
}

func TestDeleteError(t *testing.T) {
	appSet := sampleAppSet("my-app", DefaultNamespace)
	mgr := newManager(appSet)

	fake := mgr.client.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("delete", "applicationsets", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("delete blocked")
	})

	_, err := mgr.Delete(context.Background(), "my-app", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "deleting ApplicationSet") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSync(t *testing.T) {
	appSet := sampleAppSet("my-app", DefaultNamespace)
	mgr := newManager(appSet)

	err := mgr.Sync(context.Background(), "my-app", "")
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	obj, _ := mgr.client.Get(context.Background(), client.GVRApplicationSet, DefaultNamespace, "my-app")
	annotations := obj.GetAnnotations()
	if _, ok := annotations["argocd.argoproj.io/refresh"]; !ok {
		t.Error("refresh annotation should be set after sync")
	}
}

func TestSyncNotFound(t *testing.T) {
	mgr := newManager()
	err := mgr.Sync(context.Background(), "missing", "")
	if err == nil {
		t.Fatal("expected error for missing ApplicationSet")
	}
}

func TestSyncPatchError(t *testing.T) {
	appSet := sampleAppSet("my-app", DefaultNamespace)
	mgr := newManager(appSet)

	fake := mgr.client.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("patch", "applicationsets", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("patch denied")
	})

	err := mgr.Sync(context.Background(), "my-app", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "patching ApplicationSet") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCreateDefaults(t *testing.T) {
	mgr := newManager()
	err := mgr.Create(context.Background(), AppSetOpts{
		Name:    "defaults-test",
		RepoURL: "https://github.com/example/repo.git",
		Path:    "k8s",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	obj, _ := mgr.client.Get(context.Background(), client.GVRApplicationSet, DefaultNamespace, "defaults-test")
	if obj.GetNamespace() != DefaultNamespace {
		t.Errorf("namespace = %q, want %s", obj.GetNamespace(), DefaultNamespace)
	}
	labels := obj.GetLabels()
	if labels["acmlab.redhat.com/generator"] != "placement" {
		t.Errorf("generator label = %q, want placement", labels["acmlab.redhat.com/generator"])
	}

	rev, _, _ := unstructured.NestedString(obj.Object, "spec", "template", "spec", "source", "targetRevision")
	if rev != "main" {
		t.Errorf("revision = %q, want main", rev)
	}

	proj, _, _ := unstructured.NestedString(obj.Object, "spec", "template", "spec", "project")
	if proj != "default" {
		t.Errorf("project = %q, want default", proj)
	}
}

func TestCreateClusterGenerator(t *testing.T) {
	mgr := newManager()
	err := mgr.Create(context.Background(), AppSetOpts{
		Name:      "cluster-gen",
		RepoURL:   "https://github.com/example/repo.git",
		Path:      "k8s",
		Generator: "cluster",
		LabelSelector: map[string]string{
			"env": "prod",
		},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	obj, _ := mgr.client.Get(context.Background(), client.GVRApplicationSet, DefaultNamespace, "cluster-gen")
	labels := obj.GetLabels()
	if labels["acmlab.redhat.com/generator"] != "cluster" {
		t.Errorf("generator label = %q, want cluster", labels["acmlab.redhat.com/generator"])
	}

	generators, _, _ := unstructured.NestedSlice(obj.Object, "spec", "generators")
	if len(generators) != 1 {
		t.Fatalf("expected 1 generator, got %d", len(generators))
	}
	gen := generators[0].(map[string]interface{})
	if _, ok := gen["clusters"]; !ok {
		t.Error("expected clusters generator")
	}
}

func TestCreateWithCustomNamespace(t *testing.T) {
	mgr := newManager()
	err := mgr.Create(context.Background(), AppSetOpts{
		Name:      "custom-ns",
		Namespace: "argocd",
		RepoURL:   "https://github.com/example/repo.git",
		Path:      "k8s",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	obj, err := mgr.client.Get(context.Background(), client.GVRApplicationSet, "argocd", "custom-ns")
	if err != nil {
		t.Fatalf("not found in custom namespace: %v", err)
	}
	if obj.GetNamespace() != "argocd" {
		t.Errorf("namespace = %q, want argocd", obj.GetNamespace())
	}
}

func TestBuildApplicationSetPlacementGenerator(t *testing.T) {
	opts := AppSetOpts{
		Name:      "test-app",
		Namespace: DefaultNamespace,
		RepoURL:   "https://github.com/example/repo.git",
		Path:      "manifests",
		Revision:  "main",
		Generator: "placement",
		Project:   "default",
		LabelSelector: map[string]string{
			"env": "staging",
		},
	}
	appSet := buildApplicationSet(opts)

	if appSet.GetName() != "test-app" {
		t.Errorf("Name = %q", appSet.GetName())
	}

	generators, _, _ := unstructured.NestedSlice(appSet.Object, "spec", "generators")
	if len(generators) != 1 {
		t.Fatalf("generators count = %d, want 1", len(generators))
	}
	gen := generators[0].(map[string]interface{})
	if _, ok := gen["clusterDecisionResource"]; !ok {
		t.Error("expected clusterDecisionResource generator for placement type")
	}
}

func TestBuildApplicationSetClusterGenerator(t *testing.T) {
	opts := AppSetOpts{
		Name:      "cluster-test",
		Namespace: DefaultNamespace,
		RepoURL:   "https://github.com/example/repo.git",
		Path:      "k8s",
		Revision:  "main",
		Generator: "cluster",
		Project:   "default",
	}
	appSet := buildApplicationSet(opts)

	generators, _, _ := unstructured.NestedSlice(appSet.Object, "spec", "generators")
	gen := generators[0].(map[string]interface{})
	if _, ok := gen["clusters"]; !ok {
		t.Error("expected clusters generator for cluster type")
	}
}

func TestBuildGeneratorPlacement(t *testing.T) {
	opts := AppSetOpts{
		Generator:     "placement",
		LabelSelector: map[string]string{"env": "prod"},
	}
	gen := buildGenerator(opts)
	cdr, ok := gen["clusterDecisionResource"].(map[string]interface{})
	if !ok {
		t.Fatal("expected clusterDecisionResource")
	}
	if cdr["configMapRef"] != "acm-placement" {
		t.Errorf("configMapRef = %v", cdr["configMapRef"])
	}
	if cdr["requeueAfterSeconds"] != int64(180) {
		t.Errorf("requeueAfterSeconds = %v", cdr["requeueAfterSeconds"])
	}
}

func TestBuildGeneratorCluster(t *testing.T) {
	opts := AppSetOpts{
		Generator:     "cluster",
		LabelSelector: map[string]string{"env": "prod"},
	}
	gen := buildGenerator(opts)
	clusters, ok := gen["clusters"].(map[string]interface{})
	if !ok {
		t.Fatal("expected clusters key")
	}
	sel, ok := clusters["selector"].(map[string]interface{})
	if !ok {
		t.Fatal("expected selector")
	}
	ml, _ := sel["matchLabels"].(map[string]interface{})
	if ml["env"] != "prod" {
		t.Errorf("matchLabels env = %v", ml["env"])
	}
}

func TestParseAppSetInfoSynced(t *testing.T) {
	appSet := sampleAppSet("my-app", DefaultNamespace)
	info := parseAppSetInfo(appSet.Object)
	if info.Status != "Synced" {
		t.Errorf("Status = %q, want Synced", info.Status)
	}
	if info.AppCount != 2 {
		t.Errorf("AppCount = %d, want 2", info.AppCount)
	}
}

func TestParseAppSetInfoPending(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      "pending-app",
			"namespace": DefaultNamespace,
			"labels": map[string]interface{}{
				"acmlab.redhat.com/generator": "placement",
			},
		},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"source": map[string]interface{}{
						"repoURL": "https://github.com/example/repo.git",
						"path":    "k8s",
					},
				},
			},
		},
	}
	info := parseAppSetInfo(obj)
	if info.Status != "Pending" {
		t.Errorf("Status = %q, want Pending", info.Status)
	}
	if info.AppCount != 0 {
		t.Errorf("AppCount = %d, want 0", info.AppCount)
	}
}

func TestParseAppSetInfoNoMetadata(t *testing.T) {
	info := parseAppSetInfo(map[string]interface{}{})
	if info.Name != "" {
		t.Errorf("Name = %q, want empty", info.Name)
	}
	if info.Status != "Pending" {
		t.Errorf("Status = %q, want Pending", info.Status)
	}
}

func TestApplyDefaults(t *testing.T) {
	opts := &AppSetOpts{Name: "test"}
	applyDefaults(opts)
	if opts.Namespace != DefaultNamespace {
		t.Errorf("Namespace = %q, want %s", opts.Namespace, DefaultNamespace)
	}
	if opts.Revision != "main" {
		t.Errorf("Revision = %q, want main", opts.Revision)
	}
	if opts.Generator != "placement" {
		t.Errorf("Generator = %q, want placement", opts.Generator)
	}
	if opts.Project != "default" {
		t.Errorf("Project = %q, want default", opts.Project)
	}
}

func TestApplyDefaultsPreservesExisting(t *testing.T) {
	opts := &AppSetOpts{
		Name:      "test",
		Namespace: "argocd",
		Revision:  "v2.0",
		Generator: "cluster",
		Project:   "infra",
	}
	applyDefaults(opts)
	if opts.Namespace != "argocd" {
		t.Errorf("Namespace = %q, want argocd", opts.Namespace)
	}
	if opts.Revision != "v2.0" {
		t.Errorf("Revision = %q, want v2.0", opts.Revision)
	}
	if opts.Generator != "cluster" {
		t.Errorf("Generator = %q, want cluster", opts.Generator)
	}
	if opts.Project != "infra" {
		t.Errorf("Project = %q, want infra", opts.Project)
	}
}

func TestGetWithCustomNamespace(t *testing.T) {
	appSet := sampleAppSet("my-app", "argocd")
	mgr := newManager(appSet)

	info, err := mgr.Get(context.Background(), "my-app", "argocd")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if info.Namespace != "argocd" {
		t.Errorf("Namespace = %q, want argocd", info.Namespace)
	}
}

func TestDeleteWithCustomNamespace(t *testing.T) {
	appSet := sampleAppSet("my-app", "argocd")
	mgr := newManager(appSet)

	removed, err := mgr.Delete(context.Background(), "my-app", "argocd")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if !removed {
		t.Error("Delete should return true")
	}
}
