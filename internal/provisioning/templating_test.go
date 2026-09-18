package provisioning

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

func fakeTemplateClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRClusterDeploymentCustomization: "ClusterDeploymentCustomizationList",
			client.GVRClusterDeployment:              "ClusterDeploymentList",
		}, objs...)
	return &client.Client{Dynamic: fake}
}

func templateObj(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeploymentCustomization",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": map[string]interface{}{
				"installConfigPatches": []interface{}{
					map[string]interface{}{"op": "replace", "path": "/networking/clusterNetwork/0/cidr", "value": "10.128.0.0/14"},
				},
			},
		},
	}
}

func clusterDeploymentObj(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": name,
			},
		},
	}
}

func TestCreateTemplate(t *testing.T) {
	c := fakeTemplateClient()
	m := New(c, config.Config{}, discardLogger)

	patches := []TemplatePatch{
		{Op: "replace", Path: "/networking/clusterNetwork/0/cidr", Value: "10.128.0.0/14"},
	}
	if err := m.CreateTemplate(context.Background(), "small-cluster", patches); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}
}

func TestCreateTemplateIdempotent(t *testing.T) {
	c := fakeTemplateClient()
	m := New(c, config.Config{}, discardLogger)

	patches := []TemplatePatch{{Op: "replace", Path: "/p", Value: "v"}}
	if err := m.CreateTemplate(context.Background(), "tmpl", patches); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := m.CreateTemplate(context.Background(), "tmpl", patches); err != nil {
		t.Fatalf("second create should be idempotent: %v", err)
	}
}

func TestGetTemplate(t *testing.T) {
	tmpl := templateObj("gpu-large")
	c := fakeTemplateClient(tmpl)
	m := New(c, config.Config{}, discardLogger)

	obj, err := m.GetTemplate(context.Background(), "gpu-large")
	if err != nil {
		t.Fatalf("GetTemplate failed: %v", err)
	}
	meta := obj["metadata"].(map[string]interface{})
	if meta["name"] != "gpu-large" {
		t.Errorf("expected name gpu-large, got %v", meta["name"])
	}
}

func TestGetTemplateNotFound(t *testing.T) {
	c := fakeTemplateClient()
	m := New(c, config.Config{}, discardLogger)

	_, err := m.GetTemplate(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
}

func TestListTemplates(t *testing.T) {
	c := fakeTemplateClient(templateObj("small"), templateObj("large"))
	m := New(c, config.Config{}, discardLogger)

	list, err := m.ListTemplates(context.Background())
	if err != nil {
		t.Fatalf("ListTemplates failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 templates, got %d", len(list))
	}
}

func TestRemoveTemplate(t *testing.T) {
	c := fakeTemplateClient(templateObj("old"))
	m := New(c, config.Config{}, discardLogger)

	if err := m.RemoveTemplate(context.Background(), "old"); err != nil {
		t.Fatalf("RemoveTemplate failed: %v", err)
	}
}

func TestApplyTemplate(t *testing.T) {
	tmpl := templateObj("gpu-large")
	cd := clusterDeploymentObj("spoke1")
	c := fakeTemplateClient(tmpl, cd)
	m := New(c, config.Config{}, discardLogger)

	if err := m.ApplyTemplate(context.Background(), "spoke1", "gpu-large"); err != nil {
		t.Fatalf("ApplyTemplate failed: %v", err)
	}
}

func TestApplyTemplateNotFound(t *testing.T) {
	cd := clusterDeploymentObj("spoke1")
	c := fakeTemplateClient(cd)
	m := New(c, config.Config{}, discardLogger)

	err := m.ApplyTemplate(context.Background(), "spoke1", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
}

func TestBuildClusterDeploymentCustomization(t *testing.T) {
	patches := []TemplatePatch{
		{Op: "replace", Path: "/p", Value: "v"},
	}
	obj := buildClusterDeploymentCustomization("test", patches)
	if obj.GetName() != "test" {
		t.Errorf("expected name test, got %s", obj.GetName())
	}
	labels := obj.GetLabels()
	if labels["acmlab.redhat.com/cluster-template"] != "true" {
		t.Error("expected cluster-template label")
	}
}
