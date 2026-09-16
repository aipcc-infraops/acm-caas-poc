package automation

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
	client.GVRPolicyAutomation: "PolicyAutomationList",
}

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrKinds, objs...)
	return &client.Client{Dynamic: fake}
}

func newManager(objs ...runtime.Object) *Manager {
	return New(fakeClient(objs...), config.Config{}, discardLogger)
}

func existingAutomation(name, namespace, policyName, mode string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1beta1",
			"kind":       "PolicyAutomation",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":    "true",
					"acmlab.redhat.com/automation": "true",
				},
			},
			"spec": map[string]interface{}{
				"policyRef": policyName,
				"mode":      mode,
				"automationDef": map[string]interface{}{
					"name":   "remediate-template",
					"secret": "tower-creds",
					"type":   "AnsibleJob",
				},
			},
		},
	}
}

func existingAutomationWithStatus(name, namespace, policyName, mode string) *unstructured.Unstructured {
	obj := existingAutomation(name, namespace, policyName, mode)
	obj.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"type":   "Ready",
				"status": "True",
			},
		},
		"lastRun": "2026-09-16T10:00:00Z",
	}
	return obj
}

func TestNewReturnsManager(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)
	if mgr == nil {
		t.Fatal("New returned nil")
	}
}

func TestCreate(t *testing.T) {
	mgr := newManager()
	err := mgr.Create(context.Background(), AutomationOpts{
		Name:        "auto-remediate",
		Namespace:   DefaultNamespace,
		PolicyName:  "image-policy",
		Mode:        "scan",
		TowerSecret: "tower-creds",
		JobTemplate: "remediate-template",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	info, err := mgr.Get(context.Background(), "auto-remediate", DefaultNamespace)
	if err != nil {
		t.Fatalf("Get after Create failed: %v", err)
	}
	if info.PolicyName != "image-policy" {
		t.Errorf("expected policyName=image-policy, got %s", info.PolicyName)
	}
	if info.Mode != "scan" {
		t.Errorf("expected mode=scan, got %s", info.Mode)
	}
}

func TestCreateIdempotent(t *testing.T) {
	mgr := newManager()
	opts := AutomationOpts{
		Name:        "auto-remediate",
		Namespace:   DefaultNamespace,
		PolicyName:  "image-policy",
		TowerSecret: "tower-creds",
		JobTemplate: "remediate-template",
	}
	if err := mgr.Create(context.Background(), opts); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if err := mgr.Create(context.Background(), opts); err != nil {
		t.Fatalf("second Create should be idempotent: %v", err)
	}
}

func TestCreateDefaults(t *testing.T) {
	mgr := newManager()
	err := mgr.Create(context.Background(), AutomationOpts{
		Name:        "auto-default",
		PolicyName:  "test-policy",
		TowerSecret: "tower-creds",
		JobTemplate: "remediate-template",
	})
	if err != nil {
		t.Fatalf("Create with defaults failed: %v", err)
	}

	info, err := mgr.Get(context.Background(), "auto-default", DefaultNamespace)
	if err != nil {
		t.Fatalf("Get after Create failed: %v", err)
	}
	if info.Mode != "scan" {
		t.Errorf("expected default mode=scan, got %s", info.Mode)
	}
	if info.Namespace != DefaultNamespace {
		t.Errorf("expected default namespace=%s, got %s", DefaultNamespace, info.Namespace)
	}
}

func TestCreateWithExtraVars(t *testing.T) {
	mgr := newManager()
	err := mgr.Create(context.Background(), AutomationOpts{
		Name:        "auto-extra",
		PolicyName:  "test-policy",
		TowerSecret: "tower-creds",
		JobTemplate: "remediate-template",
		ExtraVars:   map[string]string{"env": "production", "notify": "true"},
	})
	if err != nil {
		t.Fatalf("Create with extra vars failed: %v", err)
	}
}

func TestGet(t *testing.T) {
	obj := existingAutomationWithStatus("auto-1", DefaultNamespace, "policy-1", "scan")
	mgr := newManager(obj)

	info, err := mgr.Get(context.Background(), "auto-1", DefaultNamespace)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if info.Name != "auto-1" {
		t.Errorf("expected name=auto-1, got %s", info.Name)
	}
	if info.PolicyName != "policy-1" {
		t.Errorf("expected policyName=policy-1, got %s", info.PolicyName)
	}
	if info.Status != "Ready" {
		t.Errorf("expected status=Ready, got %s", info.Status)
	}
	if info.LastRun != "2026-09-16T10:00:00Z" {
		t.Errorf("expected lastRun timestamp, got %s", info.LastRun)
	}
}

func TestGetNotFound(t *testing.T) {
	mgr := newManager()
	_, err := mgr.Get(context.Background(), "nonexistent", DefaultNamespace)
	if err == nil {
		t.Fatal("expected error for nonexistent PolicyAutomation")
	}
}

func TestGetDefaultNamespace(t *testing.T) {
	obj := existingAutomation("auto-1", DefaultNamespace, "policy-1", "scan")
	mgr := newManager(obj)

	info, err := mgr.Get(context.Background(), "auto-1", "")
	if err != nil {
		t.Fatalf("Get with empty namespace failed: %v", err)
	}
	if info.Namespace != DefaultNamespace {
		t.Errorf("expected namespace=%s, got %s", DefaultNamespace, info.Namespace)
	}
}

func TestList(t *testing.T) {
	objs := []runtime.Object{
		existingAutomation("auto-1", DefaultNamespace, "policy-1", "scan"),
		existingAutomation("auto-2", DefaultNamespace, "policy-2", "once"),
	}
	mgr := newManager(objs...)

	infos, err := mgr.List(context.Background(), DefaultNamespace)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("expected 2 automations, got %d", len(infos))
	}
}

func TestListEmpty(t *testing.T) {
	mgr := newManager()
	infos, err := mgr.List(context.Background(), DefaultNamespace)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("expected 0 automations, got %d", len(infos))
	}
}

func TestListDefaultNamespace(t *testing.T) {
	obj := existingAutomation("auto-1", DefaultNamespace, "policy-1", "scan")
	mgr := newManager(obj)

	infos, err := mgr.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List with empty namespace failed: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 automation, got %d", len(infos))
	}
}

func TestDelete(t *testing.T) {
	obj := existingAutomation("auto-1", DefaultNamespace, "policy-1", "scan")
	mgr := newManager(obj)

	removed, err := mgr.Delete(context.Background(), "auto-1", DefaultNamespace)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if !removed {
		t.Error("expected removed=true")
	}

	_, err = mgr.Get(context.Background(), "auto-1", DefaultNamespace)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestDeleteNotFound(t *testing.T) {
	mgr := newManager()
	removed, err := mgr.Delete(context.Background(), "nonexistent", DefaultNamespace)
	if err != nil {
		t.Fatalf("Delete of nonexistent should not error: %v", err)
	}
	if removed {
		t.Error("expected removed=false for nonexistent")
	}
}

func TestDeleteDefaultNamespace(t *testing.T) {
	obj := existingAutomation("auto-1", DefaultNamespace, "policy-1", "scan")
	mgr := newManager(obj)

	removed, err := mgr.Delete(context.Background(), "auto-1", "")
	if err != nil {
		t.Fatalf("Delete with empty namespace failed: %v", err)
	}
	if !removed {
		t.Error("expected removed=true")
	}
}

func TestUpdateMode(t *testing.T) {
	obj := existingAutomation("auto-1", DefaultNamespace, "policy-1", "scan")
	mgr := newManager(obj)

	err := mgr.UpdateMode(context.Background(), "auto-1", DefaultNamespace, "disabled")
	if err != nil {
		t.Fatalf("UpdateMode failed: %v", err)
	}

	info, err := mgr.Get(context.Background(), "auto-1", DefaultNamespace)
	if err != nil {
		t.Fatalf("Get after UpdateMode failed: %v", err)
	}
	if info.Mode != "disabled" {
		t.Errorf("expected mode=disabled, got %s", info.Mode)
	}
}

func TestUpdateModeNotFound(t *testing.T) {
	mgr := newManager()
	err := mgr.UpdateMode(context.Background(), "nonexistent", DefaultNamespace, "scan")
	if err == nil {
		t.Fatal("expected error for nonexistent PolicyAutomation")
	}
}

func TestUpdateModeOnce(t *testing.T) {
	obj := existingAutomation("auto-1", DefaultNamespace, "policy-1", "scan")
	mgr := newManager(obj)

	err := mgr.UpdateMode(context.Background(), "auto-1", DefaultNamespace, "once")
	if err != nil {
		t.Fatalf("UpdateMode to once failed: %v", err)
	}

	info, err := mgr.Get(context.Background(), "auto-1", DefaultNamespace)
	if err != nil {
		t.Fatalf("Get after UpdateMode failed: %v", err)
	}
	if info.Mode != "once" {
		t.Errorf("expected mode=once, got %s", info.Mode)
	}
}

func TestUpdateModeDefaultNamespace(t *testing.T) {
	obj := existingAutomation("auto-1", DefaultNamespace, "policy-1", "scan")
	mgr := newManager(obj)

	err := mgr.UpdateMode(context.Background(), "auto-1", "", "disabled")
	if err != nil {
		t.Fatalf("UpdateMode with empty namespace failed: %v", err)
	}
}

func TestParseAutomationInfoPending(t *testing.T) {
	obj := existingAutomation("auto-1", DefaultNamespace, "policy-1", "scan")
	info := parseAutomationInfo(obj)
	if info.Status != "Pending" {
		t.Errorf("expected status=Pending without status field, got %s", info.Status)
	}
}

func TestParseAutomationInfoReady(t *testing.T) {
	obj := existingAutomationWithStatus("auto-1", DefaultNamespace, "policy-1", "scan")
	info := parseAutomationInfo(obj)
	if info.Status != "Ready" {
		t.Errorf("expected status=Ready, got %s", info.Status)
	}
	if info.LastRun == "" {
		t.Error("expected lastRun to be populated")
	}
}

func TestBuildPolicyAutomation(t *testing.T) {
	opts := AutomationOpts{
		Name:        "auto-test",
		Namespace:   DefaultNamespace,
		PolicyName:  "test-policy",
		Mode:        "scan",
		TowerSecret: "tower-creds",
		JobTemplate: "remediate-template",
		ExtraVars:   map[string]string{"env": "staging"},
	}
	obj := buildPolicyAutomation(opts)

	if obj.GetName() != "auto-test" {
		t.Errorf("expected name=auto-test, got %s", obj.GetName())
	}
	labels := obj.GetLabels()
	if labels["acmlab.redhat.com/automation"] != "true" {
		t.Error("expected automation label")
	}
	if labels["acmlab.redhat.com/managed"] != "true" {
		t.Error("expected managed label")
	}

	spec, _ := obj.Object["spec"].(map[string]interface{})
	if spec["policyRef"] != "test-policy" {
		t.Errorf("expected policyRef=test-policy, got %v", spec["policyRef"])
	}
	if spec["mode"] != "scan" {
		t.Errorf("expected mode=scan, got %v", spec["mode"])
	}

	autoDef, _ := spec["automationDef"].(map[string]interface{})
	if autoDef["name"] != "remediate-template" {
		t.Errorf("expected jobTemplate=remediate-template, got %v", autoDef["name"])
	}
	if autoDef["secret"] != "tower-creds" {
		t.Errorf("expected secret=tower-creds, got %v", autoDef["secret"])
	}
	if autoDef["type"] != "AnsibleJob" {
		t.Errorf("expected type=AnsibleJob, got %v", autoDef["type"])
	}

	extraVars, _ := autoDef["extra_vars"].(map[string]interface{})
	if extraVars["env"] != "staging" {
		t.Errorf("expected extra_vars.env=staging, got %v", extraVars["env"])
	}
}

func TestBuildPolicyAutomationNoExtraVars(t *testing.T) {
	opts := AutomationOpts{
		Name:        "auto-simple",
		Namespace:   DefaultNamespace,
		PolicyName:  "test-policy",
		Mode:        "once",
		TowerSecret: "tower-creds",
		JobTemplate: "remediate-template",
	}
	obj := buildPolicyAutomation(opts)

	spec, _ := obj.Object["spec"].(map[string]interface{})
	autoDef, _ := spec["automationDef"].(map[string]interface{})
	extraVars, _ := autoDef["extra_vars"].(map[string]interface{})

	if extraVars["policy_name"] != "{{ policy_name }}" {
		t.Error("expected default policy_name extra var")
	}
	if extraVars["target_clusters"] != "{{ target_clusters }}" {
		t.Error("expected default target_clusters extra var")
	}
}
