package virtualization

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

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRManifestWork: "ManifestWorkList",
		}, objs...)
	return &client.Client{Dynamic: fake}
}

func TestDeploy(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	opts := VMOpts{Name: "test-vm", Cluster: "spoke1", CPU: "4", Memory: "8Gi", Image: "rhel9:latest", DiskSize: "40Gi"}
	if err := mgr.Deploy(context.Background(), opts); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}
	obj, err := c.Get(context.Background(), client.GVRManifestWork, "spoke1", "vm-test-vm-spoke1")
	if err != nil {
		t.Fatalf("ManifestWork not found: %v", err)
	}
	labels := obj.GetLabels()
	if labels[LabelVM] != "true" {
		t.Errorf("missing VM label")
	}
	if labels[LabelVMName] != "test-vm" {
		t.Errorf("VM name label = %q, want test-vm", labels[LabelVMName])
	}
}

func TestDeployDefaultValues(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	opts := VMOpts{Name: "defaults", Cluster: "spoke1"}
	if err := mgr.Deploy(context.Background(), opts); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}
	obj, err := c.Get(context.Background(), client.GVRManifestWork, "spoke1", "vm-defaults-spoke1")
	if err != nil {
		t.Fatalf("ManifestWork not found: %v", err)
	}
	labels := obj.GetLabels()
	if labels["acmlab.redhat.com/vm-image"] != DefaultImage {
		t.Errorf("default image not applied")
	}
}

func TestRemove(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	opts := VMOpts{Name: "rm-vm", Cluster: "spoke1"}
	_ = mgr.Deploy(context.Background(), opts)

	if err := mgr.Remove(context.Background(), "rm-vm", "spoke1"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	_, err := c.Get(context.Background(), client.GVRManifestWork, "spoke1", "vm-rm-vm-spoke1")
	if err == nil {
		t.Error("ManifestWork still exists after remove")
	}
}

func TestRemoveNotFound(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	if err := mgr.Remove(context.Background(), "nope", "spoke1"); err != nil {
		t.Fatalf("Remove non-existent should not error: %v", err)
	}
}

func TestStart(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	opts := VMOpts{Name: "start-vm", Cluster: "spoke1"}
	_ = mgr.Deploy(context.Background(), opts)

	if err := mgr.Start(context.Background(), "start-vm", "spoke1"); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	obj, _ := c.Get(context.Background(), client.GVRManifestWork, "spoke1", "vm-start-vm-spoke1")
	manifests, _ := obj.Object["spec"].(map[string]interface{})["workload"].(map[string]interface{})["manifests"].([]interface{})
	vm := manifests[0].(map[string]interface{})
	running := vm["spec"].(map[string]interface{})["running"].(bool)
	if !running {
		t.Error("VM should be running after Start")
	}
}

func TestStop(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	opts := VMOpts{Name: "stop-vm", Cluster: "spoke1"}
	_ = mgr.Deploy(context.Background(), opts)

	if err := mgr.Stop(context.Background(), "stop-vm", "spoke1"); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	obj, _ := c.Get(context.Background(), client.GVRManifestWork, "spoke1", "vm-stop-vm-spoke1")
	manifests, _ := obj.Object["spec"].(map[string]interface{})["workload"].(map[string]interface{})["manifests"].([]interface{})
	vm := manifests[0].(map[string]interface{})
	running := vm["spec"].(map[string]interface{})["running"].(bool)
	if running {
		t.Error("VM should not be running after Stop")
	}
}

func TestList(t *testing.T) {
	mw := &unstructured.Unstructured{}
	mw.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "work.open-cluster-management.io", Version: "v1", Kind: "ManifestWork",
	})
	mw.SetName("vm-web-spoke1")
	mw.SetNamespace("spoke1")
	mw.SetLabels(map[string]string{
		LabelVM:     "true",
		LabelVMName: "web",
	})

	c := fakeClient(mw)
	mgr := New(c, config.Config{}, discardLogger)

	vms, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(vms) != 1 {
		t.Fatalf("got %d VMs, want 1", len(vms))
	}
	if vms[0].Name != "web" {
		t.Errorf("name = %q, want web", vms[0].Name)
	}
}

func TestListEmpty(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	vms, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(vms) != 0 {
		t.Errorf("got %d VMs, want 0", len(vms))
	}
}

func TestStatus(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	opts := VMOpts{Name: "status-vm", Cluster: "spoke1", CPU: "4", Memory: "8Gi", Image: "rhel9:latest"}
	_ = mgr.Deploy(context.Background(), opts)

	detail, err := mgr.Status(context.Background(), "status-vm", "spoke1")
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if detail.Name != "status-vm" {
		t.Errorf("name = %q, want status-vm", detail.Name)
	}
	if detail.Cluster != "spoke1" {
		t.Errorf("cluster = %q, want spoke1", detail.Cluster)
	}
}
