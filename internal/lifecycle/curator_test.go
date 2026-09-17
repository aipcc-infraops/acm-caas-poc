package lifecycle

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

var curatorLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func fakeCuratorClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRClusterCurator: "ClusterCuratorList",
		}, objs...)
	return &client.Client{Dynamic: fake}
}

func curatorObj(name, namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1beta1", Kind: "ClusterCurator",
	})
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.Object["spec"] = map[string]interface{}{
		"prehook": []interface{}{
			map[string]interface{}{
				"name": "backup-etcd",
				"type": "Job",
			},
		},
		"posthook": []interface{}{
			map[string]interface{}{
				"name": "verify-health",
				"type": "Job",
			},
		},
	}
	return obj
}

func TestApplyCuratorCreatesResource(t *testing.T) {
	c := fakeCuratorClient()
	m := New(c, config.Config{}, curatorLogger)

	err := m.ApplyCurator(context.Background(), CuratorOpts{
		Cluster: "spoke1",
		PreHook: &CuratorHook{
			Name: "backup-etcd",
			Type: "Job",
		},
	})
	if err != nil {
		t.Fatalf("ApplyCurator failed: %v", err)
	}
}

func TestApplyCuratorWithBothHooks(t *testing.T) {
	c := fakeCuratorClient()
	m := New(c, config.Config{}, curatorLogger)

	err := m.ApplyCurator(context.Background(), CuratorOpts{
		Cluster: "spoke1",
		PreHook: &CuratorHook{
			Name: "backup",
			Type: "Job",
		},
		PostHook: &CuratorHook{
			Name: "verify",
			Type: "Job",
		},
	})
	if err != nil {
		t.Fatalf("ApplyCurator failed: %v", err)
	}
}

func TestGetCuratorReturnsParsedInfo(t *testing.T) {
	cc := curatorObj("spoke1", "spoke1")
	c := fakeCuratorClient(cc)
	m := New(c, config.Config{}, curatorLogger)

	info, err := m.GetCurator(context.Background(), "spoke1", "")
	if err != nil {
		t.Fatalf("GetCurator failed: %v", err)
	}
	if info.Name != "spoke1" {
		t.Errorf("Name = %q, want %q", info.Name, "spoke1")
	}
	if info.PreHook == nil {
		t.Fatal("expected PreHook")
	}
	if info.PreHook.Name != "backup-etcd" {
		t.Errorf("PreHook.Name = %q, want %q", info.PreHook.Name, "backup-etcd")
	}
	if info.PostHook == nil {
		t.Fatal("expected PostHook")
	}
	if info.PostHook.Name != "verify-health" {
		t.Errorf("PostHook.Name = %q, want %q", info.PostHook.Name, "verify-health")
	}
}

func TestGetCuratorReturnsErrorForMissing(t *testing.T) {
	c := fakeCuratorClient()
	m := New(c, config.Config{}, curatorLogger)

	_, err := m.GetCurator(context.Background(), "nonexistent", "")
	if err == nil {
		t.Error("expected error for missing curator")
	}
}

func TestRemoveCurator(t *testing.T) {
	cc := curatorObj("spoke1", "spoke1")
	c := fakeCuratorClient(cc)
	m := New(c, config.Config{}, curatorLogger)

	removed, err := m.RemoveCurator(context.Background(), "spoke1", "")
	if err != nil {
		t.Fatalf("RemoveCurator failed: %v", err)
	}
	if !removed {
		t.Error("expected removed = true")
	}
}

func TestRemoveCuratorNotFound(t *testing.T) {
	c := fakeCuratorClient()
	m := New(c, config.Config{}, curatorLogger)

	removed, err := m.RemoveCurator(context.Background(), "nonexistent", "")
	if err != nil {
		t.Fatalf("RemoveCurator failed: %v", err)
	}
	if removed {
		t.Error("expected removed = false")
	}
}

func TestListCurators(t *testing.T) {
	cc1 := curatorObj("spoke1", "spoke1")
	cc2 := curatorObj("spoke2", "spoke2")
	c := fakeCuratorClient(cc1, cc2)
	m := New(c, config.Config{}, curatorLogger)

	curators, err := m.ListCurators(context.Background(), "")
	if err != nil {
		t.Fatalf("ListCurators failed: %v", err)
	}
	if len(curators) != 2 {
		t.Errorf("got %d curators, want 2", len(curators))
	}
}

func TestBuildClusterCurator(t *testing.T) {
	obj := buildClusterCurator(CuratorOpts{
		Cluster: "spoke1",
		PreHook: &CuratorHook{Name: "backup", Type: "Job"},
	})
	if obj.GetName() != "spoke1" {
		t.Errorf("Name = %q, want %q", obj.GetName(), "spoke1")
	}
	labels := obj.GetLabels()
	if labels["acmlab.redhat.com/curator"] != "true" {
		t.Error("missing curator label")
	}
	spec, _ := obj.Object["spec"].(map[string]interface{})
	pre, _ := spec["prehook"].([]interface{})
	if len(pre) != 1 {
		t.Errorf("got %d prehooks, want 1", len(pre))
	}
}

func TestBuildHookSpec(t *testing.T) {
	hook := buildHookSpec(&CuratorHook{
		Name:     "test-hook",
		Type:     "Job",
		Commands: []string{"echo hello"},
		JobTTL:   300,
	})
	if hook["name"] != "test-hook" {
		t.Errorf("name = %v, want %q", hook["name"], "test-hook")
	}
	cmds, _ := hook["commands"].([]interface{})
	if len(cmds) != 1 {
		t.Errorf("got %d commands, want 1", len(cmds))
	}
	if hook["job_ttl"] != int64(300) {
		t.Errorf("job_ttl = %v, want 300", hook["job_ttl"])
	}
}

func TestBuildHookSpecWithImage(t *testing.T) {
	hook := buildHookSpec(&CuratorHook{
		Name:      "ansible-hook",
		Type:      "AnsibleJob",
		Image:     "quay.io/test/runner:latest",
		ExtraVars: map[string]string{"cluster_name": "spoke1"},
	})
	if hook["name"] != "ansible-hook" {
		t.Errorf("name = %v, want %q", hook["name"], "ansible-hook")
	}
	evars, ok := hook["extra_vars"].(map[string]interface{})
	if !ok {
		t.Fatal("expected extra_vars map")
	}
	if evars["cluster_name"] != "spoke1" {
		t.Errorf("extra_vars[cluster_name] = %v, want spoke1", evars["cluster_name"])
	}
}

func TestBuildHookSpecMinimal(t *testing.T) {
	hook := buildHookSpec(&CuratorHook{
		Name: "simple",
	})
	if hook["name"] != "simple" {
		t.Errorf("name = %v, want %q", hook["name"], "simple")
	}
	if _, ok := hook["commands"]; ok {
		t.Error("expected no commands for minimal hook")
	}
	if _, ok := hook["job_ttl"]; ok {
		t.Error("expected no job_ttl for minimal hook")
	}
}

func curatorObjWithStatus(name, namespace string, complete bool) *unstructured.Unstructured {
	obj := curatorObj(name, namespace)
	condStatus := "False"
	if complete {
		condStatus = "True"
	}
	obj.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"type":   "clustercurator-job",
				"status": condStatus,
			},
		},
	}
	return obj
}

func TestParseCuratorInfoComplete(t *testing.T) {
	obj := curatorObjWithStatus("spoke1", "spoke1", true)
	info := parseCuratorInfo(obj.Object)
	if info.Status != "Complete" {
		t.Errorf("Status = %q, want %q", info.Status, "Complete")
	}
}

func TestParseCuratorInfoInProgress(t *testing.T) {
	obj := curatorObjWithStatus("spoke1", "spoke1", false)
	info := parseCuratorInfo(obj.Object)
	if info.Status != "InProgress" {
		t.Errorf("Status = %q, want %q", info.Status, "InProgress")
	}
}

func TestParseCuratorInfoNoSpec(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1beta1", Kind: "ClusterCurator",
	})
	obj.SetName("bare")
	obj.SetNamespace("bare")
	info := parseCuratorInfo(obj.Object)
	if info.PreHook != nil {
		t.Error("expected nil PreHook")
	}
	if info.PostHook != nil {
		t.Error("expected nil PostHook")
	}
	if info.Status != "Pending" {
		t.Errorf("Status = %q, want %q", info.Status, "Pending")
	}
}

func TestApplyCuratorWithNamespace(t *testing.T) {
	c := fakeCuratorClient()
	m := New(c, config.Config{}, curatorLogger)

	err := m.ApplyCurator(context.Background(), CuratorOpts{
		Cluster:   "spoke1",
		Namespace: "custom-ns",
		PostHook: &CuratorHook{
			Name: "verify",
			Type: "Job",
		},
	})
	if err != nil {
		t.Fatalf("ApplyCurator failed: %v", err)
	}
}

func TestGetCuratorWithNamespace(t *testing.T) {
	cc := curatorObj("spoke1", "custom-ns")
	c := fakeCuratorClient(cc)
	m := New(c, config.Config{}, curatorLogger)

	info, err := m.GetCurator(context.Background(), "spoke1", "custom-ns")
	if err != nil {
		t.Fatalf("GetCurator failed: %v", err)
	}
	if info.Namespace != "custom-ns" {
		t.Errorf("Namespace = %q, want %q", info.Namespace, "custom-ns")
	}
}
