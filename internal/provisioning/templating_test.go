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

func templateObj(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeploymentCustomization",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
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
	if err := m.CreateTemplate(context.Background(), "small-cluster", "hive-test", patches); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}
}

func TestCreateTemplateIdempotent(t *testing.T) {
	c := fakeTemplateClient()
	m := New(c, config.Config{}, discardLogger)

	patches := []TemplatePatch{{Op: "replace", Path: "/p", Value: "v"}}
	if err := m.CreateTemplate(context.Background(), "tmpl", "hive-test", patches); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := m.CreateTemplate(context.Background(), "tmpl", "hive-test", patches); err != nil {
		t.Fatalf("second create should be idempotent: %v", err)
	}
}

func TestCreateTemplateWithNamespace(t *testing.T) {
	c := fakeTemplateClient()
	m := New(c, config.Config{}, discardLogger)

	patches := []TemplatePatch{{Op: "replace", Path: "/p", Value: "v"}}
	if err := m.CreateTemplate(context.Background(), "gpu-template", "my-ns", patches); err != nil {
		t.Fatalf("CreateTemplate failed: %v", err)
	}

	obj, err := c.Get(context.Background(), client.GVRClusterDeploymentCustomization, "my-ns", "gpu-template")
	if err != nil {
		t.Fatalf("resource not found in namespace my-ns: %v", err)
	}
	if obj.GetNamespace() != "my-ns" {
		t.Errorf("namespace = %q, want my-ns", obj.GetNamespace())
	}
}

func TestGetTemplate(t *testing.T) {
	tmpl := templateObj("gpu-large", "hive-test")
	c := fakeTemplateClient(tmpl)
	m := New(c, config.Config{}, discardLogger)

	obj, err := m.GetTemplate(context.Background(), "gpu-large", "hive-test")
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

	_, err := m.GetTemplate(context.Background(), "nonexistent", "hive-test")
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
}

func TestListTemplates(t *testing.T) {
	c := fakeTemplateClient(templateObj("small", "hive-test"), templateObj("large", "hive-test"))
	m := New(c, config.Config{}, discardLogger)

	list, err := m.ListTemplates(context.Background(), "hive-test")
	if err != nil {
		t.Fatalf("ListTemplates failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 templates, got %d", len(list))
	}
}

func TestListTemplatesInNamespace(t *testing.T) {
	c := fakeTemplateClient(
		templateObj("t1", "ns-a"),
		templateObj("t2", "ns-b"),
	)
	m := New(c, config.Config{}, discardLogger)

	listA, err := m.ListTemplates(context.Background(), "ns-a")
	if err != nil {
		t.Fatalf("ListTemplates ns-a failed: %v", err)
	}
	if len(listA) != 1 {
		t.Errorf("expected 1 template in ns-a, got %d", len(listA))
	}

	listB, err := m.ListTemplates(context.Background(), "ns-b")
	if err != nil {
		t.Fatalf("ListTemplates ns-b failed: %v", err)
	}
	if len(listB) != 1 {
		t.Errorf("expected 1 template in ns-b, got %d", len(listB))
	}
}

func TestRemoveTemplate(t *testing.T) {
	c := fakeTemplateClient(templateObj("old", "hive-test"))
	m := New(c, config.Config{}, discardLogger)

	if err := m.RemoveTemplate(context.Background(), "old", "hive-test"); err != nil {
		t.Fatalf("RemoveTemplate failed: %v", err)
	}
}

func TestApplyTemplate(t *testing.T) {
	tmpl := templateObj("gpu-large", "spoke1")
	cd := clusterDeploymentObj("spoke1")
	c := fakeTemplateClient(tmpl, cd)
	m := New(c, config.Config{}, discardLogger)

	if err := m.ApplyTemplate(context.Background(), "spoke1", "gpu-large", "spoke1"); err != nil {
		t.Fatalf("ApplyTemplate failed: %v", err)
	}
}

func TestApplyTemplateDefaultNamespace(t *testing.T) {
	tmpl := templateObj("gpu-large", "spoke1")
	cd := clusterDeploymentObj("spoke1")
	c := fakeTemplateClient(tmpl, cd)
	m := New(c, config.Config{}, discardLogger)

	if err := m.ApplyTemplate(context.Background(), "spoke1", "gpu-large", ""); err != nil {
		t.Fatalf("ApplyTemplate with empty templateNamespace failed: %v", err)
	}
}

func TestApplyTemplateNotFound(t *testing.T) {
	cd := clusterDeploymentObj("spoke1")
	c := fakeTemplateClient(cd)
	m := New(c, config.Config{}, discardLogger)

	err := m.ApplyTemplate(context.Background(), "spoke1", "nonexistent", "spoke1")
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
}

func TestBuildClusterDeploymentCustomization(t *testing.T) {
	patches := []TemplatePatch{
		{Op: "replace", Path: "/p", Value: "v"},
	}
	obj := buildClusterDeploymentCustomization("test", "hive-test", patches)
	if obj.GetName() != "test" {
		t.Errorf("expected name test, got %s", obj.GetName())
	}
	if obj.GetNamespace() != "hive-test" {
		t.Errorf("expected namespace hive-test, got %s", obj.GetNamespace())
	}
	labels := obj.GetLabels()
	if labels["acmlab.redhat.com/cluster-template"] != "true" {
		t.Error("expected cluster-template label")
	}
}

func TestBuildClusterDeploymentCustomizationTypedValues(t *testing.T) {
	patches := []TemplatePatch{
		{Op: "replace", Path: "/compute/0/replicas", Value: float64(2)},
		{Op: "replace", Path: "/networking/clusterNetwork/0", Value: map[string]interface{}{"cidr": "10.132.0.0/14", "hostPrefix": float64(23)}},
		{Op: "replace", Path: "/metadata/name", Value: "my-cluster"},
	}
	obj := buildClusterDeploymentCustomization("typed-test", "ns", patches)

	spec, _ := obj.Object["spec"].(map[string]interface{})
	patchList, _ := spec["installConfigPatches"].([]interface{})
	if len(patchList) != 3 {
		t.Fatalf("expected 3 patches, got %d", len(patchList))
	}

	p0 := patchList[0].(map[string]interface{})
	if v, ok := p0["value"].(float64); !ok || v != 2 {
		t.Errorf("expected numeric value 2, got %v (%T)", p0["value"], p0["value"])
	}

	p1 := patchList[1].(map[string]interface{})
	if m, ok := p1["value"].(map[string]interface{}); !ok {
		t.Errorf("expected map value, got %T", p1["value"])
	} else if m["cidr"] != "10.132.0.0/14" {
		t.Errorf("expected cidr=10.132.0.0/14, got %v", m["cidr"])
	}

	p2 := patchList[2].(map[string]interface{})
	if v, ok := p2["value"].(string); !ok || v != "my-cluster" {
		t.Errorf("expected string value 'my-cluster', got %v (%T)", p2["value"], p2["value"])
	}
}
