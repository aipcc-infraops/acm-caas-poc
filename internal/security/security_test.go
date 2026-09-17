package security

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
	client.GVRManifestWork:      "ManifestWorkList",
	client.GVRPolicy:            "PolicyList",
	client.GVRPlacement:         "PlacementList",
	client.GVRPlacementBinding:  "PlacementBindingList",
}

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrKinds, objs...)
	return &client.Client{Dynamic: fake}
}

func newManager(objs ...runtime.Object) *Manager {
	return New(fakeClient(objs...), config.Config{}, discardLogger)
}

func baselineManifestWork(cluster, level string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      "security-baseline-" + cluster,
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":            "true",
					"acmlab.redhat.com/security-baseline":  "true",
					"acmlab.redhat.com/security-level":     level,
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{},
				},
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Applied",
						"status": "True",
					},
				},
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

func TestApplyBaselineCreatesResources(t *testing.T) {
	mgr := newManager()
	err := mgr.ApplyBaseline(context.Background(), "spoke1", "cis-level1", "")
	if err != nil {
		t.Fatalf("ApplyBaseline failed: %v", err)
	}

	_, err = mgr.client.Get(context.Background(), client.GVRManifestWork, "spoke1", "security-baseline-spoke1")
	if err != nil {
		t.Fatalf("ManifestWork not found: %v", err)
	}

	_, err = mgr.client.Get(context.Background(), client.GVRPolicy, DefaultNamespace, "gatekeeper-health-spoke1")
	if err != nil {
		t.Fatalf("Health policy not found: %v", err)
	}

	_, err = mgr.client.Get(context.Background(), client.GVRPlacement, DefaultNamespace, "gatekeeper-health-spoke1-placement")
	if err != nil {
		t.Fatalf("Placement not found: %v", err)
	}

	_, err = mgr.client.Get(context.Background(), client.GVRPlacementBinding, DefaultNamespace, "gatekeeper-health-spoke1-placement-binding")
	if err != nil {
		t.Fatalf("PlacementBinding not found: %v", err)
	}
}

func TestApplyBaselineDefaultLevel(t *testing.T) {
	mgr := newManager()
	err := mgr.ApplyBaseline(context.Background(), "spoke2", "", "")
	if err != nil {
		t.Fatalf("ApplyBaseline with default level failed: %v", err)
	}

	obj, err := mgr.client.Get(context.Background(), client.GVRManifestWork, "spoke2", "security-baseline-spoke2")
	if err != nil {
		t.Fatalf("ManifestWork not found: %v", err)
	}
	labels := obj.GetLabels()
	if labels["acmlab.redhat.com/security-level"] != "cis-level1" {
		t.Errorf("level = %q, want cis-level1", labels["acmlab.redhat.com/security-level"])
	}
}

func TestApplyBaselineWithClusterSet(t *testing.T) {
	mgr := newManager()
	err := mgr.ApplyBaseline(context.Background(), "spoke1", "cis-level1", "team-gpu")
	if err != nil {
		t.Fatalf("ApplyBaseline failed: %v", err)
	}

	placement, err := mgr.client.Get(context.Background(), client.GVRPlacement, DefaultNamespace, "gatekeeper-health-spoke1-placement")
	if err != nil {
		t.Fatalf("Placement not found: %v", err)
	}
	cs, _, _ := unstructured.NestedStringSlice(placement.Object, "spec", "clusterSets")
	if len(cs) != 1 || cs[0] != "team-gpu" {
		t.Errorf("clusterSets = %v, want [team-gpu]", cs)
	}
}

func TestApplyBaselineIdempotent(t *testing.T) {
	mgr := newManager()
	if err := mgr.ApplyBaseline(context.Background(), "spoke1", "cis-level1", ""); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if err := mgr.ApplyBaseline(context.Background(), "spoke1", "cis-level1", ""); err != nil {
		t.Fatalf("second apply should be idempotent: %v", err)
	}
}

func TestGetStatusReturnsInfo(t *testing.T) {
	mw := baselineManifestWork("spoke1", "cis-level1")
	mgr := newManager(mw)

	status, err := mgr.GetStatus(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if status.Cluster != "spoke1" {
		t.Errorf("Cluster = %q, want spoke1", status.Cluster)
	}
	if status.Level != "cis-level1" {
		t.Errorf("Level = %q, want cis-level1", status.Level)
	}
	if !status.Applied {
		t.Error("Applied should be true")
	}
}

func TestGetStatusNotFound(t *testing.T) {
	mgr := newManager()
	status, err := mgr.GetStatus(context.Background(), "missing")
	if err != nil {
		t.Fatalf("GetStatus should return status with level=none for missing baseline, got error: %v", err)
	}
	if status.Level != "none" {
		t.Errorf("expected level=none, got %q", status.Level)
	}
	if status.Applied {
		t.Error("expected Applied=false")
	}
}

func TestListBaselinesEmpty(t *testing.T) {
	mgr := newManager()
	infos, err := mgr.ListBaselines(context.Background())
	if err != nil {
		t.Fatalf("ListBaselines failed: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("got %d baselines, want 0", len(infos))
	}
}

func TestListBaselinesWithExisting(t *testing.T) {
	mw1 := baselineManifestWork("spoke1", "cis-level1")
	mw2 := baselineManifestWork("spoke2", "cis-level1")
	mgr := newManager(mw1, mw2)

	infos, err := mgr.ListBaselines(context.Background())
	if err != nil {
		t.Fatalf("ListBaselines failed: %v", err)
	}
	if len(infos) != 2 {
		t.Errorf("got %d baselines, want 2", len(infos))
	}
}

func TestRemoveBaselineExisting(t *testing.T) {
	mgr := newManager()
	if err := mgr.ApplyBaseline(context.Background(), "spoke1", "cis-level1", ""); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	removed, err := mgr.RemoveBaseline(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("RemoveBaseline failed: %v", err)
	}
	if !removed {
		t.Error("RemoveBaseline should return true for existing baseline")
	}

	_, err = mgr.client.Get(context.Background(), client.GVRManifestWork, "spoke1", "security-baseline-spoke1")
	if err == nil {
		t.Error("ManifestWork should not exist after remove")
	}
}

func TestRemoveBaselineNotFound(t *testing.T) {
	mgr := newManager()
	removed, err := mgr.RemoveBaseline(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("RemoveBaseline failed: %v", err)
	}
	if removed {
		t.Error("RemoveBaseline should return false for nonexistent baseline")
	}
}

func TestApplyBaselineManifestWorkError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "manifestworks", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("forbidden")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.ApplyBaseline(context.Background(), "spoke1", "cis-level1", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "creating security baseline ManifestWork") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestApplyBaselinePolicyError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "policies", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("forbidden")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.ApplyBaseline(context.Background(), "spoke1", "cis-level1", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "creating Gatekeeper health policy") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestListBaselinesError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "manifestworks", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("timeout")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.ListBaselines(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "listing security baselines") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRemoveBaselineDeleteError(t *testing.T) {
	mgr := newManager()
	if err := mgr.ApplyBaseline(context.Background(), "spoke1", "cis-level1", ""); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	fake := mgr.client.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("delete", "manifestworks", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("delete blocked")
	})

	_, err := mgr.RemoveBaseline(context.Background(), "spoke1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "removing baseline ManifestWork") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuildGatekeeperManifestWorkStructure(t *testing.T) {
	mw := buildGatekeeperManifestWork("spoke1", "cis-level1")

	if mw.GetName() != "security-baseline-spoke1" {
		t.Errorf("Name = %q, want security-baseline-spoke1", mw.GetName())
	}
	if mw.GetNamespace() != "spoke1" {
		t.Errorf("Namespace = %q, want spoke1", mw.GetNamespace())
	}

	labels := mw.GetLabels()
	if labels["acmlab.redhat.com/security-level"] != "cis-level1" {
		t.Errorf("security-level label = %q, want cis-level1", labels["acmlab.redhat.com/security-level"])
	}

	manifests, _, _ := unstructured.NestedSlice(mw.Object, "spec", "workload", "manifests")
	if len(manifests) != 8 {
		t.Errorf("manifests count = %d, want 8 (4 templates + 4 constraints)", len(manifests))
	}
}

func TestBuildGatekeeperHealthPolicy(t *testing.T) {
	pol := buildGatekeeperHealthPolicy("spoke1", "")
	if pol.GetName() != "gatekeeper-health-spoke1" {
		t.Errorf("Name = %q, want gatekeeper-health-spoke1", pol.GetName())
	}
	if pol.GetNamespace() != DefaultNamespace {
		t.Errorf("Namespace = %q, want %s", pol.GetNamespace(), DefaultNamespace)
	}
}

func TestBuildHealthPlacement(t *testing.T) {
	p := buildHealthPlacement("spoke1", "team-gpu")
	if p.GetName() != "gatekeeper-health-spoke1-placement" {
		t.Errorf("Name = %q", p.GetName())
	}
	cs, _, _ := unstructured.NestedStringSlice(p.Object, "spec", "clusterSets")
	if len(cs) != 1 || cs[0] != "team-gpu" {
		t.Errorf("clusterSets = %v", cs)
	}
}

func TestBuildHealthPlacementNoClusterSet(t *testing.T) {
	p := buildHealthPlacement("spoke1", "")
	_, found, _ := unstructured.NestedStringSlice(p.Object, "spec", "clusterSets")
	if found {
		t.Error("clusterSets should not be set when empty")
	}
}

func TestBuildHealthPlacementBinding(t *testing.T) {
	b := buildHealthPlacementBinding("spoke1")
	if b.GetName() != "gatekeeper-health-spoke1-placement-binding" {
		t.Errorf("Name = %q", b.GetName())
	}
}

func TestBuildConstraints(t *testing.T) {
	constraints := buildConstraints(defaultAllowedRepos)
	if len(constraints) != 4 {
		t.Errorf("constraints count = %d, want 4", len(constraints))
	}
	kinds := map[string]bool{}
	for _, c := range constraints {
		kinds[c["kind"].(string)] = true
	}
	expected := []string{"K8sPSPPrivilegedContainer", "K8sPSPHostNamespace", "K8sRequiredResources", "K8sAllowedRepos"}
	for _, e := range expected {
		if !kinds[e] {
			t.Errorf("missing constraint kind: %s", e)
		}
	}
}

func TestParseBaselineStatusNoStatus(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				"acmlab.redhat.com/security-level": "cis-level1",
			},
		},
	}
	bs := parseBaselineStatus("spoke1", obj)
	if bs.Cluster != "spoke1" {
		t.Errorf("Cluster = %q", bs.Cluster)
	}
	if bs.Level != "cis-level1" {
		t.Errorf("Level = %q", bs.Level)
	}
	if bs.Applied {
		t.Error("Applied should be false without status")
	}
}

func TestParseBaselineInfoApplied(t *testing.T) {
	mw := baselineManifestWork("spoke1", "cis-level1")
	info := parseBaselineInfo(mw.Object)
	if info.Cluster != "spoke1" {
		t.Errorf("Cluster = %q", info.Cluster)
	}
	if info.Level != "cis-level1" {
		t.Errorf("Level = %q", info.Level)
	}
	if info.Status != "Applied" {
		t.Errorf("Status = %q, want Applied", info.Status)
	}
}

func TestParseBaselineInfoPending(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"namespace": "spoke2",
			"labels": map[string]interface{}{
				"acmlab.redhat.com/security-level": "cis-level1",
			},
		},
	}
	info := parseBaselineInfo(obj)
	if info.Status != "Pending" {
		t.Errorf("Status = %q, want Pending", info.Status)
	}
}

func TestNestedMapValid(t *testing.T) {
	obj := map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": "value",
			},
		},
	}
	m, ok := nestedMap(obj, "a", "b")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if m["c"] != "value" {
		t.Errorf("got %v", m["c"])
	}
}

func TestNestedMapInvalid(t *testing.T) {
	obj := map[string]interface{}{
		"a": "not a map",
	}
	_, ok := nestedMap(obj, "a", "b")
	if ok {
		t.Error("expected ok=false for invalid path")
	}
}

func TestApplyCustomPolicyFromInline(t *testing.T) {
	mgr := newManager()
	rego := "package deny_latest\nviolation[{\"msg\": msg}] {\n  c := input.review.object.spec.containers[_]\n  endswith(c.image, \":latest\")\n  msg := \"latest tag not allowed\"\n}"
	opts := CustomPolicyOpts{
		Cluster:    "spoke1",
		RegoInline: rego,
	}
	err := mgr.ApplyCustomPolicy(context.Background(), opts)
	if err != nil {
		t.Fatalf("ApplyCustomPolicy failed: %v", err)
	}

	mw, err := mgr.client.Get(context.Background(), client.GVRManifestWork, "spoke1", "custom-rego-deny_latest-spoke1")
	if err != nil {
		t.Fatalf("ManifestWork not found: %v", err)
	}
	labels := mw.GetLabels()
	if labels["acmlab.redhat.com/custom-rego"] != "true" {
		t.Error("custom-rego label missing")
	}
}

func TestApplyCustomPolicyWithName(t *testing.T) {
	mgr := newManager()
	rego := "package my_policy\nviolation[{\"msg\": \"denied\"}] { true }"
	opts := CustomPolicyOpts{
		Name:       "block-stuff",
		Cluster:    "spoke1",
		RegoInline: rego,
	}
	err := mgr.ApplyCustomPolicy(context.Background(), opts)
	if err != nil {
		t.Fatalf("ApplyCustomPolicy failed: %v", err)
	}

	_, err = mgr.client.Get(context.Background(), client.GVRManifestWork, "spoke1", "custom-rego-block-stuff-spoke1")
	if err != nil {
		t.Fatalf("ManifestWork not found: %v", err)
	}
}

func TestApplyCustomPolicyNoRego(t *testing.T) {
	mgr := newManager()
	opts := CustomPolicyOpts{Cluster: "spoke1"}
	err := mgr.ApplyCustomPolicy(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when no rego provided")
	}
}

func TestApplyCustomPolicyNoPackage(t *testing.T) {
	mgr := newManager()
	opts := CustomPolicyOpts{
		Cluster:    "spoke1",
		RegoInline: "violation[{\"msg\": \"bad\"}] { true }",
	}
	err := mgr.ApplyCustomPolicy(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when no package declaration")
	}
}

func TestRemoveCustomPolicy(t *testing.T) {
	mgr := newManager()
	rego := "package test_remove\nviolation[{\"msg\": \"x\"}] { true }"
	opts := CustomPolicyOpts{Cluster: "spoke1", RegoInline: rego}
	if err := mgr.ApplyCustomPolicy(context.Background(), opts); err != nil {
		t.Fatalf("apply: %v", err)
	}

	err := mgr.RemoveCustomPolicy(context.Background(), "test_remove", "spoke1")
	if err != nil {
		t.Fatalf("RemoveCustomPolicy failed: %v", err)
	}
}

func TestRemoveCustomPolicyNotFound(t *testing.T) {
	mgr := newManager()
	err := mgr.RemoveCustomPolicy(context.Background(), "nonexistent", "spoke1")
	if err == nil {
		t.Fatal("expected error for nonexistent policy")
	}
}

func TestListCustomPolicies(t *testing.T) {
	mgr := newManager()
	rego := "package list_test\nviolation[{\"msg\": \"x\"}] { true }"
	opts := CustomPolicyOpts{Cluster: "spoke1", RegoInline: rego}
	if err := mgr.ApplyCustomPolicy(context.Background(), opts); err != nil {
		t.Fatalf("apply: %v", err)
	}

	policies, err := mgr.ListCustomPolicies(context.Background())
	if err != nil {
		t.Fatalf("ListCustomPolicies failed: %v", err)
	}
	if len(policies) != 1 {
		t.Errorf("got %d policies, want 1", len(policies))
	}
}

func TestExtractPackageName(t *testing.T) {
	tests := []struct {
		rego string
		want string
	}{
		{"package deny_latest\nviolation[{}] { true }", "deny_latest"},
		{"# comment\npackage my.policy\nviolation[{}] { true }", "my.policy"},
		{"violation[{}] { true }", ""},
	}
	for _, tt := range tests {
		got := extractPackageName(tt.rego)
		if got != tt.want {
			t.Errorf("extractPackageName(%q) = %q, want %q", tt.rego[:20], got, tt.want)
		}
	}
}

func TestToCamelCase(t *testing.T) {
	tests := []struct {
		pkg  string
		want string
	}{
		{"deny_latest", "DenyLatest"},
		{"k8s.required.labels", "K8sRequiredLabels"},
		{"simple", "Simple"},
	}
	for _, tt := range tests {
		got := toCamelCase(tt.pkg)
		if got != tt.want {
			t.Errorf("toCamelCase(%q) = %q, want %q", tt.pkg, got, tt.want)
		}
	}
}

func TestToKebab(t *testing.T) {
	tests := []struct {
		pkg  string
		want string
	}{
		{"deny_latest", "deny-latest"},
		{"k8s.required.labels", "k8s-required-labels"},
	}
	for _, tt := range tests {
		got := toKebab(tt.pkg)
		if got != tt.want {
			t.Errorf("toKebab(%q) = %q, want %q", tt.pkg, got, tt.want)
		}
	}
}

func kyvernoManifestWork(cluster, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      "kyverno-policy-" + name + "-" + cluster,
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":        "true",
					"acmlab.redhat.com/kyverno-policy": "true",
					"acmlab.redhat.com/policy-name":    name,
				},
			},
		},
	}
}

func TestApplyKyvernoPolicyFromInline(t *testing.T) {
	mgr := newManager()
	yaml := "apiVersion: kyverno.io/v1\nkind: ClusterPolicy\nmetadata:\n  name: disallow-latest\nspec:\n  rules:\n  - name: deny-latest\n    match:\n      resources:\n        kinds:\n        - Pod"
	opts := KyvernoPolicyOpts{
		Cluster:    "spoke1",
		PolicyYAML: yaml,
	}
	err := mgr.ApplyKyvernoPolicy(context.Background(), opts)
	if err != nil {
		t.Fatalf("ApplyKyvernoPolicy failed: %v", err)
	}

	mw, err := mgr.client.Get(context.Background(), client.GVRManifestWork, "spoke1", "kyverno-policy-disallow-latest-spoke1")
	if err != nil {
		t.Fatalf("ManifestWork not found: %v", err)
	}
	labels := mw.GetLabels()
	if labels["acmlab.redhat.com/kyverno-policy"] != "true" {
		t.Error("kyverno-policy label missing")
	}
}

func TestApplyKyvernoPolicyWithName(t *testing.T) {
	mgr := newManager()
	yaml := "apiVersion: kyverno.io/v1\nkind: ClusterPolicy\nmetadata:\n  name: original-name\nspec: {}"
	opts := KyvernoPolicyOpts{
		Name:       "custom-name",
		Cluster:    "spoke1",
		PolicyYAML: yaml,
	}
	err := mgr.ApplyKyvernoPolicy(context.Background(), opts)
	if err != nil {
		t.Fatalf("ApplyKyvernoPolicy failed: %v", err)
	}

	_, err = mgr.client.Get(context.Background(), client.GVRManifestWork, "spoke1", "kyverno-policy-custom-name-spoke1")
	if err != nil {
		t.Fatalf("ManifestWork not found: %v", err)
	}
}

func TestApplyKyvernoPolicyNoYAML(t *testing.T) {
	mgr := newManager()
	opts := KyvernoPolicyOpts{Cluster: "spoke1"}
	err := mgr.ApplyKyvernoPolicy(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when no policy YAML provided")
	}
}

func TestApplyKyvernoPolicyNoName(t *testing.T) {
	mgr := newManager()
	opts := KyvernoPolicyOpts{
		Cluster:    "spoke1",
		PolicyYAML: "apiVersion: kyverno.io/v1\nkind: ClusterPolicy\nspec: {}",
	}
	err := mgr.ApplyKyvernoPolicy(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when no name can be determined")
	}
}

func TestRemoveKyvernoPolicy(t *testing.T) {
	mgr := newManager()
	yaml := "apiVersion: kyverno.io/v1\nkind: ClusterPolicy\nmetadata:\n  name: to-remove\nspec: {}"
	opts := KyvernoPolicyOpts{Cluster: "spoke1", PolicyYAML: yaml}
	if err := mgr.ApplyKyvernoPolicy(context.Background(), opts); err != nil {
		t.Fatalf("apply: %v", err)
	}

	err := mgr.RemoveKyvernoPolicy(context.Background(), "to-remove", "spoke1")
	if err != nil {
		t.Fatalf("RemoveKyvernoPolicy failed: %v", err)
	}
}

func TestRemoveKyvernoPolicyNotFound(t *testing.T) {
	mgr := newManager()
	err := mgr.RemoveKyvernoPolicy(context.Background(), "nonexistent", "spoke1")
	if err == nil {
		t.Fatal("expected error for nonexistent policy")
	}
}

func TestListKyvernoPolicies(t *testing.T) {
	mw := kyvernoManifestWork("spoke1", "deny-latest")
	mgr := newManager(mw)

	policies, err := mgr.ListKyvernoPolicies(context.Background())
	if err != nil {
		t.Fatalf("ListKyvernoPolicies failed: %v", err)
	}
	if len(policies) != 1 {
		t.Errorf("got %d policies, want 1", len(policies))
	}
}

func TestExtractKyvernoPolicyName(t *testing.T) {
	tests := []struct {
		yaml string
		want string
	}{
		{"apiVersion: kyverno.io/v1\nkind: ClusterPolicy\nmetadata:\n  name: disallow-latest\nspec: {}", "disallow-latest"},
		{"apiVersion: kyverno.io/v1\nkind: ClusterPolicy\nspec: {}", ""},
	}
	for _, tt := range tests {
		got := extractKyvernoPolicyName(tt.yaml)
		if got != tt.want {
			t.Errorf("extractKyvernoPolicyName() = %q, want %q", got, tt.want)
		}
	}
}

func TestBuildKyvernoManifestWork(t *testing.T) {
	mw := buildKyvernoManifestWork("spoke1", "deny-latest", "apiVersion: kyverno.io/v1\nkind: ClusterPolicy")
	if mw.GetName() != "kyverno-policy-deny-latest-spoke1" {
		t.Errorf("Name = %q", mw.GetName())
	}
	labels := mw.GetLabels()
	if labels["acmlab.redhat.com/kyverno-policy"] != "true" {
		t.Error("kyverno-policy label missing")
	}
	if labels["acmlab.redhat.com/policy-name"] != "deny-latest" {
		t.Errorf("policy-name label = %q", labels["acmlab.redhat.com/policy-name"])
	}
}

func TestApplyInvalidLevel(t *testing.T) {
	mgr := newManager()
	err := mgr.ApplyBaseline(context.Background(), "spoke1", "garbage", "")
	if err == nil {
		t.Fatal("expected error for invalid level")
	}
	if !strings.Contains(err.Error(), "invalid security level") {
		t.Errorf("unexpected error: %v", err)
	}
}
