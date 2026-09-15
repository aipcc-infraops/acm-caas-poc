package policy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
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

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRPolicy:           "PolicyList",
			client.GVRPlacement:        "PlacementList",
			client.GVRPlacementBinding: "PlacementBindingList",
			client.GVRManagedCluster:   "ManagedClusterList",
		}, objs...)
	return &client.Client{Dynamic: fake}
}

func TestApplyCreatesAllResources(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	opts := PolicyOpts{
		Name:              "test-policy",
		Namespace:         DefaultNamespace,
		RemediationAction: "inform",
		ClusterLabels:     map[string]string{"env": "dev"},
	}
	if err := mgr.Apply(context.Background(), opts); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	ctx := context.Background()
	if _, err := c.Get(ctx, client.GVRPolicy, DefaultNamespace, "test-policy"); err != nil {
		t.Errorf("policy not created: %v", err)
	}
	if _, err := c.Get(ctx, client.GVRPlacement, DefaultNamespace, "test-policy-placement"); err != nil {
		t.Errorf("placement not created: %v", err)
	}
	if _, err := c.Get(ctx, client.GVRPlacementBinding, DefaultNamespace, "test-policy-placement-binding"); err != nil {
		t.Errorf("placement binding not created: %v", err)
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	opts := PolicyOpts{
		Name:      "test-policy",
		Namespace: DefaultNamespace,
	}
	if err := mgr.Apply(context.Background(), opts); err != nil {
		t.Fatalf("first Apply failed: %v", err)
	}
	if err := mgr.Apply(context.Background(), opts); err != nil {
		t.Fatalf("second Apply failed (not idempotent): %v", err)
	}
}

func TestApplyWithRegistries(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	opts := PolicyOpts{
		Name:              "registry-policy",
		Namespace:         DefaultNamespace,
		RemediationAction: "enforce",
		AllowedRegistries: []string{"registry.redhat.io", "quay.io/myorg"},
	}
	if err := mgr.Apply(context.Background(), opts); err != nil {
		t.Fatalf("Apply with registries failed: %v", err)
	}

	obj, err := c.Get(context.Background(), client.GVRPolicy, DefaultNamespace, "registry-policy")
	if err != nil {
		t.Fatalf("policy not created: %v", err)
	}

	spec, _ := obj.Object["spec"].(map[string]interface{})
	if spec["remediationAction"] != "enforce" {
		t.Errorf("remediationAction = %v, want enforce", spec["remediationAction"])
	}
}

func TestApplyDefaultsNamespace(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	opts := PolicyOpts{Name: "test-policy"}
	if err := mgr.Apply(context.Background(), opts); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if _, err := c.Get(context.Background(), client.GVRPolicy, DefaultNamespace, "test-policy"); err != nil {
		t.Errorf("policy not in default namespace: %v", err)
	}
}

func TestRemoveDeletesAllResources(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	opts := PolicyOpts{Name: "test-policy", Namespace: DefaultNamespace}
	if err := mgr.Apply(context.Background(), opts); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if err := mgr.Remove(context.Background(), "test-policy", DefaultNamespace); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	ctx := context.Background()
	if _, err := c.Get(ctx, client.GVRPolicy, DefaultNamespace, "test-policy"); err == nil {
		t.Error("policy still exists after remove")
	}
	if _, err := c.Get(ctx, client.GVRPlacement, DefaultNamespace, "test-policy-placement"); err == nil {
		t.Error("placement still exists after remove")
	}
	if _, err := c.Get(ctx, client.GVRPlacementBinding, DefaultNamespace, "test-policy-placement-binding"); err == nil {
		t.Error("placement binding still exists after remove")
	}
}

func TestRemoveIsIdempotent(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	if err := mgr.Remove(context.Background(), "nonexistent", DefaultNamespace); err != nil {
		t.Fatalf("Remove on empty cluster failed (not idempotent): %v", err)
	}
}

func TestListPolicies(t *testing.T) {
	pol := &unstructured.Unstructured{}
	pol.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "policy.open-cluster-management.io", Version: "v1", Kind: "Policy",
	})
	pol.SetName("test-pol")
	pol.SetNamespace(DefaultNamespace)
	pol.Object["spec"] = map[string]interface{}{
		"remediationAction": "enforce",
		"disabled":          false,
	}

	c := fakeClient(pol)
	mgr := New(c, config.Config{}, discardLogger)

	policies, err := mgr.List(context.Background(), DefaultNamespace)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("got %d policies, want 1", len(policies))
	}
	if policies[0].Name != "test-pol" {
		t.Errorf("name = %q, want test-pol", policies[0].Name)
	}
	if policies[0].RemediationAction != "enforce" {
		t.Errorf("remediation = %q, want enforce", policies[0].RemediationAction)
	}
}

func TestGetPolicy(t *testing.T) {
	pol := &unstructured.Unstructured{}
	pol.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "policy.open-cluster-management.io", Version: "v1", Kind: "Policy",
	})
	pol.SetName("my-policy")
	pol.SetNamespace(DefaultNamespace)
	pol.Object["spec"] = map[string]interface{}{
		"remediationAction": "inform",
		"disabled":          true,
	}
	pol.Object["status"] = map[string]interface{}{
		"compliant": "NonCompliant",
		"status": []interface{}{
			map[string]interface{}{
				"clustername": "infraops1",
				"compliant":   "NonCompliant",
			},
		},
	}

	c := fakeClient(pol)
	mgr := New(c, config.Config{}, discardLogger)

	info, err := mgr.Get(context.Background(), "my-policy", DefaultNamespace)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if info.Compliant != "NonCompliant" {
		t.Errorf("compliant = %q, want NonCompliant", info.Compliant)
	}
	if !info.Disabled {
		t.Error("expected disabled=true")
	}
	if len(info.ClusterCompliance) != 1 {
		t.Fatalf("got %d cluster compliance, want 1", len(info.ClusterCompliance))
	}
	if info.ClusterCompliance[0].ClusterName != "infraops1" {
		t.Errorf("cluster = %q, want infraops1", info.ClusterCompliance[0].ClusterName)
	}
	if info.ClusterCompliance[0].ComplianceState != "NonCompliant" {
		t.Errorf("state = %q, want NonCompliant", info.ClusterCompliance[0].ComplianceState)
	}
}

func TestSetRemediation(t *testing.T) {
	pol := &unstructured.Unstructured{}
	pol.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "policy.open-cluster-management.io", Version: "v1", Kind: "Policy",
	})
	pol.SetName("my-policy")
	pol.SetNamespace(DefaultNamespace)
	pol.Object["spec"] = map[string]interface{}{
		"remediationAction": "inform",
	}

	c := fakeClient(pol)
	mgr := New(c, config.Config{}, discardLogger)

	if err := mgr.SetRemediation(context.Background(), "my-policy", DefaultNamespace, "enforce"); err != nil {
		t.Fatalf("SetRemediation failed: %v", err)
	}

	obj, _ := c.Get(context.Background(), client.GVRPolicy, DefaultNamespace, "my-policy")
	spec, _ := obj.Object["spec"].(map[string]interface{})
	if spec["remediationAction"] != "enforce" {
		t.Errorf("remediationAction = %v, want enforce", spec["remediationAction"])
	}
}

func TestSetDisabled(t *testing.T) {
	pol := &unstructured.Unstructured{}
	pol.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "policy.open-cluster-management.io", Version: "v1", Kind: "Policy",
	})
	pol.SetName("my-policy")
	pol.SetNamespace(DefaultNamespace)
	pol.Object["spec"] = map[string]interface{}{
		"disabled": false,
	}

	c := fakeClient(pol)
	mgr := New(c, config.Config{}, discardLogger)

	if err := mgr.SetDisabled(context.Background(), "my-policy", DefaultNamespace, true); err != nil {
		t.Fatalf("SetDisabled failed: %v", err)
	}

	obj, _ := c.Get(context.Background(), client.GVRPolicy, DefaultNamespace, "my-policy")
	spec, _ := obj.Object["spec"].(map[string]interface{})
	if spec["disabled"] != true {
		t.Errorf("disabled = %v, want true", spec["disabled"])
	}
}

func TestAggregateComplianceMixed(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{"name": "test", "namespace": "ns"},
		"spec":     map[string]interface{}{"remediationAction": "inform"},
		"status": map[string]interface{}{
			"status": []interface{}{
				map[string]interface{}{"clustername": "c1", "compliant": "Compliant"},
				map[string]interface{}{"clustername": "c2", "compliant": "NonCompliant"},
			},
		},
	}
	info := parsePolicyInfo(obj)
	if info.Compliant != "NonCompliant" {
		t.Errorf("compliant = %q, want NonCompliant (one cluster non-compliant)", info.Compliant)
	}
}

func TestAggregateComplianceAllCompliant(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{"name": "test", "namespace": "ns"},
		"spec":     map[string]interface{}{"remediationAction": "inform"},
		"status": map[string]interface{}{
			"status": []interface{}{
				map[string]interface{}{"clustername": "c1", "compliant": "Compliant"},
				map[string]interface{}{"clustername": "c2", "compliant": "Compliant"},
			},
		},
	}
	info := parsePolicyInfo(obj)
	if info.Compliant != "Compliant" {
		t.Errorf("compliant = %q, want Compliant", info.Compliant)
	}
}

func TestAggregateCompliancePending(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{"name": "test", "namespace": "ns"},
		"spec":     map[string]interface{}{"remediationAction": "inform"},
		"status": map[string]interface{}{
			"status": []interface{}{
				map[string]interface{}{"clustername": "c1"},
			},
		},
	}
	info := parsePolicyInfo(obj)
	if info.Compliant != "Pending" {
		t.Errorf("compliant = %q, want Pending", info.Compliant)
	}
}

func TestParsePolicyInfoNoStatus(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      "test",
			"namespace": "ns",
		},
		"spec": map[string]interface{}{
			"remediationAction": "inform",
			"disabled":          false,
		},
	}
	info := parsePolicyInfo(obj)
	if info.Compliant != "" {
		t.Errorf("compliant = %q, want empty", info.Compliant)
	}
	if len(info.ClusterCompliance) != 0 {
		t.Errorf("got %d cluster compliance, want 0", len(info.ClusterCompliance))
	}
}

func TestApplyError(t *testing.T) {
	c := fakeClient()
	fake := c.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("get", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("api unavailable")
	})
	mgr := New(c, config.Config{}, discardLogger)

	opts := PolicyOpts{Name: "test-policy", Namespace: DefaultNamespace}
	err := mgr.Apply(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error from Apply when API fails")
	}
}

func TestRemoveError(t *testing.T) {
	c := fakeClient()
	fake := c.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("delete", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("delete blocked")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.Remove(context.Background(), "test", DefaultNamespace)
	if err == nil {
		t.Fatal("expected error from Remove when delete fails")
	}
}

func TestRemoveDefaultsNamespace(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)
	if err := mgr.Remove(context.Background(), "x", ""); err != nil {
		t.Fatalf("Remove with empty ns failed: %v", err)
	}
}

func TestListError(t *testing.T) {
	c := fakeClient()
	fake := c.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("list", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("list blocked")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.List(context.Background(), DefaultNamespace)
	if err == nil {
		t.Fatal("expected error from List when API fails")
	}
}

func TestListDefaultsNamespace(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)
	policies, err := mgr.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List with empty ns failed: %v", err)
	}
	if len(policies) != 0 {
		t.Errorf("got %d policies, want 0", len(policies))
	}
}

func TestGetError(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.Get(context.Background(), "nonexistent", DefaultNamespace)
	if err == nil {
		t.Fatal("expected error from Get when policy doesn't exist")
	}
}

func TestGetDefaultsNamespace(t *testing.T) {
	pol := &unstructured.Unstructured{}
	pol.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "policy.open-cluster-management.io", Version: "v1", Kind: "Policy",
	})
	pol.SetName("my-policy")
	pol.SetNamespace(DefaultNamespace)
	pol.Object["spec"] = map[string]interface{}{"remediationAction": "inform"}

	c := fakeClient(pol)
	mgr := New(c, config.Config{}, discardLogger)

	info, err := mgr.Get(context.Background(), "my-policy", "")
	if err != nil {
		t.Fatalf("Get with empty ns failed: %v", err)
	}
	if info.Name != "my-policy" {
		t.Errorf("name = %q, want my-policy", info.Name)
	}
}

func TestSetRemediationError(t *testing.T) {
	c := fakeClient()
	fake := c.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("patch", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("patch blocked")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.SetRemediation(context.Background(), "test", DefaultNamespace, "enforce")
	if err == nil {
		t.Fatal("expected error from SetRemediation when patch fails")
	}
}

func TestSetRemediationDefaultsNamespace(t *testing.T) {
	pol := &unstructured.Unstructured{}
	pol.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "policy.open-cluster-management.io", Version: "v1", Kind: "Policy",
	})
	pol.SetName("my-policy")
	pol.SetNamespace(DefaultNamespace)
	pol.Object["spec"] = map[string]interface{}{"remediationAction": "inform"}

	c := fakeClient(pol)
	mgr := New(c, config.Config{}, discardLogger)

	if err := mgr.SetRemediation(context.Background(), "my-policy", "", "enforce"); err != nil {
		t.Fatalf("SetRemediation with empty ns failed: %v", err)
	}
}

func TestSetDisabledError(t *testing.T) {
	c := fakeClient()
	fake := c.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("patch", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("patch blocked")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.SetDisabled(context.Background(), "test", DefaultNamespace, true)
	if err == nil {
		t.Fatal("expected error from SetDisabled when patch fails")
	}
}

func TestSetDisabledDefaultsNamespace(t *testing.T) {
	pol := &unstructured.Unstructured{}
	pol.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "policy.open-cluster-management.io", Version: "v1", Kind: "Policy",
	})
	pol.SetName("my-policy")
	pol.SetNamespace(DefaultNamespace)
	pol.Object["spec"] = map[string]interface{}{"disabled": false}

	c := fakeClient(pol)
	mgr := New(c, config.Config{}, discardLogger)

	if err := mgr.SetDisabled(context.Background(), "my-policy", "", true); err != nil {
		t.Fatalf("SetDisabled with empty ns failed: %v", err)
	}
}

func TestApplyOperatorPolicy(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	opts := PolicyOpts{
		Name:            "gpu-pin",
		Namespace:       DefaultNamespace,
		OperatorName:    "gpu-sharing-operator",
		OperatorVersion: "2.17.3",
		OperatorChannel: "stable-2.17",
		ClusterLabels:   map[string]string{"gpu": "true"},
	}
	if err := mgr.Apply(context.Background(), opts); err != nil {
		t.Fatalf("Apply OperatorPolicy failed: %v", err)
	}

	obj, err := c.Get(context.Background(), client.GVRPolicy, DefaultNamespace, "gpu-pin")
	if err != nil {
		t.Fatalf("policy not created: %v", err)
	}

	templates, _, _ := unstructured.NestedSlice(obj.Object, "spec", "policy-templates")
	if len(templates) != 1 {
		t.Fatalf("got %d policy-templates, want 1", len(templates))
	}

	tmpl := templates[0].(map[string]interface{})
	objDef := tmpl["objectDefinition"].(map[string]interface{})

	if objDef["kind"] != "OperatorPolicy" {
		t.Errorf("kind = %v, want OperatorPolicy", objDef["kind"])
	}
	if objDef["apiVersion"] != "policy.open-cluster-management.io/v1beta1" {
		t.Errorf("apiVersion = %v, want policy.open-cluster-management.io/v1beta1", objDef["apiVersion"])
	}

	spec := objDef["spec"].(map[string]interface{})
	sub := spec["subscription"].(map[string]interface{})
	if sub["name"] != "gpu-sharing-operator" {
		t.Errorf("subscription.name = %v, want gpu-sharing-operator", sub["name"])
	}
	if sub["channel"] != "stable-2.17" {
		t.Errorf("subscription.channel = %v, want stable-2.17", sub["channel"])
	}

	versions := spec["versions"].([]interface{})
	if len(versions) != 1 || versions[0] != "2.17.3" {
		t.Errorf("versions = %v, want [2.17.3]", versions)
	}
}

func TestApplyOperatorPolicyNoVersion(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	opts := PolicyOpts{
		Name:         "operator-no-ver",
		Namespace:    DefaultNamespace,
		OperatorName: "my-operator",
	}
	if err := mgr.Apply(context.Background(), opts); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	obj, _ := c.Get(context.Background(), client.GVRPolicy, DefaultNamespace, "operator-no-ver")
	templates, _, _ := unstructured.NestedSlice(obj.Object, "spec", "policy-templates")
	tmpl := templates[0].(map[string]interface{})
	spec := tmpl["objectDefinition"].(map[string]interface{})["spec"].(map[string]interface{})

	if _, ok := spec["versions"]; ok {
		t.Error("versions should not be set when OperatorVersion is empty")
	}
}

func TestApplyCertificatePolicy(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	opts := PolicyOpts{
		Name:           "cert-check",
		Namespace:      DefaultNamespace,
		CertExpiryDays: 30,
		CertNamespaces: []string{"openshift-config", "openshift-ingress", "kube-system"},
	}
	if err := mgr.Apply(context.Background(), opts); err != nil {
		t.Fatalf("Apply CertificatePolicy failed: %v", err)
	}

	obj, err := c.Get(context.Background(), client.GVRPolicy, DefaultNamespace, "cert-check")
	if err != nil {
		t.Fatalf("policy not created: %v", err)
	}

	templates, _, _ := unstructured.NestedSlice(obj.Object, "spec", "policy-templates")
	if len(templates) != 1 {
		t.Fatalf("got %d policy-templates, want 1", len(templates))
	}

	tmpl := templates[0].(map[string]interface{})
	objDef := tmpl["objectDefinition"].(map[string]interface{})

	if objDef["kind"] != "CertificatePolicy" {
		t.Errorf("kind = %v, want CertificatePolicy", objDef["kind"])
	}

	spec := objDef["spec"].(map[string]interface{})
	if spec["minimumDuration"] != "720h" {
		t.Errorf("minimumDuration = %v, want 720h", spec["minimumDuration"])
	}
	if spec["severity"] != "high" {
		t.Errorf("severity = %v, want high", spec["severity"])
	}

	nsSelector := spec["namespaceSelector"].(map[string]interface{})
	include := nsSelector["include"].([]interface{})
	if len(include) != 3 {
		t.Errorf("got %d namespaces, want 3", len(include))
	}
}

func TestApplyCertificatePolicyDefaults(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	opts := PolicyOpts{
		Name:           "cert-default",
		Namespace:      DefaultNamespace,
		CertExpiryDays: 14,
	}
	if err := mgr.Apply(context.Background(), opts); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	obj, _ := c.Get(context.Background(), client.GVRPolicy, DefaultNamespace, "cert-default")
	templates, _, _ := unstructured.NestedSlice(obj.Object, "spec", "policy-templates")
	tmpl := templates[0].(map[string]interface{})
	spec := tmpl["objectDefinition"].(map[string]interface{})["spec"].(map[string]interface{})

	if spec["minimumDuration"] != "336h" {
		t.Errorf("minimumDuration = %v, want 336h (14 days)", spec["minimumDuration"])
	}

	nsSelector := spec["namespaceSelector"].(map[string]interface{})
	include := nsSelector["include"].([]interface{})
	if len(include) != 2 {
		t.Errorf("got %d default namespaces, want 2", len(include))
	}
}

func TestBuildPolicyTemplatesConfigurationPolicy(t *testing.T) {
	opts := PolicyOpts{Name: "test"}
	templates := buildPolicyTemplates(opts, "inform")
	if len(templates) != 1 {
		t.Fatalf("got %d templates, want 1", len(templates))
	}
	tmpl := templates[0].(map[string]interface{})
	objDef := tmpl["objectDefinition"].(map[string]interface{})
	if objDef["kind"] != "ConfigurationPolicy" {
		t.Errorf("default kind = %v, want ConfigurationPolicy", objDef["kind"])
	}
}

func TestBuildPolicyTemplatesOperatorPolicy(t *testing.T) {
	opts := PolicyOpts{Name: "test", OperatorName: "my-op"}
	templates := buildPolicyTemplates(opts, "inform")
	tmpl := templates[0].(map[string]interface{})
	objDef := tmpl["objectDefinition"].(map[string]interface{})
	if objDef["kind"] != "OperatorPolicy" {
		t.Errorf("kind = %v, want OperatorPolicy", objDef["kind"])
	}
}

func TestBuildPolicyTemplatesCertificatePolicy(t *testing.T) {
	opts := PolicyOpts{Name: "test", CertExpiryDays: 7}
	templates := buildPolicyTemplates(opts, "inform")
	tmpl := templates[0].(map[string]interface{})
	objDef := tmpl["objectDefinition"].(map[string]interface{})
	if objDef["kind"] != "CertificatePolicy" {
		t.Errorf("kind = %v, want CertificatePolicy", objDef["kind"])
	}
}

func TestParsePolicyInfoBadStatusEntry(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{"name": "test", "namespace": "ns"},
		"spec":     map[string]interface{}{"remediationAction": "inform"},
		"status": map[string]interface{}{
			"status": []interface{}{
				"not-a-map",
				map[string]interface{}{"clustername": "c1", "compliant": "Compliant"},
			},
		},
	}
	info := parsePolicyInfo(obj)
	if len(info.ClusterCompliance) != 1 {
		t.Errorf("got %d cluster compliance, want 1 (bad entry skipped)", len(info.ClusterCompliance))
	}
}

func TestApplyWithClusterSet(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	opts := PolicyOpts{
		Name:       "gpu-driver-check",
		Namespace:  DefaultNamespace,
		ClusterSet: "team-serving",
	}
	if err := mgr.Apply(context.Background(), opts); err != nil {
		t.Fatalf("Apply with ClusterSet failed: %v", err)
	}

	placement, err := c.Get(context.Background(), client.GVRPlacement, DefaultNamespace, "gpu-driver-check-placement")
	if err != nil {
		t.Fatalf("placement not created: %v", err)
	}

	clusterSets, found, _ := unstructured.NestedSlice(placement.Object, "spec", "clusterSets")
	if !found {
		t.Fatal("placement has no spec.clusterSets")
	}
	if len(clusterSets) != 1 || clusterSets[0] != "team-serving" {
		t.Errorf("clusterSets = %v, want [team-serving]", clusterSets)
	}
}

func TestApplyWithClusterSetAndLabels(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	opts := PolicyOpts{
		Name:          "combined",
		Namespace:     DefaultNamespace,
		ClusterSet:    "team-gpu",
		ClusterLabels: map[string]string{"gpu": "true"},
	}
	if err := mgr.Apply(context.Background(), opts); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	placement, _ := c.Get(context.Background(), client.GVRPlacement, DefaultNamespace, "combined-placement")
	clusterSets, found, _ := unstructured.NestedSlice(placement.Object, "spec", "clusterSets")
	if !found || len(clusterSets) != 1 {
		t.Error("expected clusterSets to be set")
	}
	predicates, found, _ := unstructured.NestedSlice(placement.Object, "spec", "predicates")
	if !found || len(predicates) == 0 {
		t.Error("expected predicates to be set alongside clusterSets")
	}
}

func TestComplianceReport(t *testing.T) {
	pol := &unstructured.Unstructured{}
	pol.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "policy.open-cluster-management.io", Version: "v1", Kind: "Policy",
	})
	pol.SetName("test-pol")
	pol.SetNamespace(DefaultNamespace)
	pol.Object["spec"] = map[string]interface{}{"remediationAction": "inform"}
	pol.Object["status"] = map[string]interface{}{
		"compliant": "NonCompliant",
		"status": []interface{}{
			map[string]interface{}{"clustername": "spoke1", "compliant": "Compliant"},
			map[string]interface{}{"clustername": "spoke2", "compliant": "NonCompliant"},
			map[string]interface{}{"clustername": "spoke3", "compliant": "Compliant"},
		},
	}

	mc1 := &unstructured.Unstructured{}
	mc1.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	mc1.SetName("spoke1")
	mc1.SetLabels(map[string]string{"cluster.open-cluster-management.io/clusterset": "team-serving"})

	mc2 := &unstructured.Unstructured{}
	mc2.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	mc2.SetName("spoke2")
	mc2.SetLabels(map[string]string{"cluster.open-cluster-management.io/clusterset": "team-serving"})

	mc3 := &unstructured.Unstructured{}
	mc3.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	mc3.SetName("spoke3")

	c := fakeClient(pol, mc1, mc2, mc3)
	mgr := New(c, config.Config{}, discardLogger)

	report, err := mgr.ComplianceReport(context.Background(), DefaultNamespace)
	if err != nil {
		t.Fatalf("ComplianceReport failed: %v", err)
	}

	if len(report) != 2 {
		t.Fatalf("got %d sets, want 2 (team-serving + default)", len(report))
	}

	setMap := map[string]ClusterSetCompliance{}
	for _, r := range report {
		setMap[r.ClusterSet] = r
	}

	serving := setMap["team-serving"]
	if serving.Total != 2 {
		t.Errorf("team-serving total = %d, want 2", serving.Total)
	}
	if serving.Compliant != 1 {
		t.Errorf("team-serving compliant = %d, want 1", serving.Compliant)
	}
	if serving.NonCompliant != 1 {
		t.Errorf("team-serving noncompliant = %d, want 1", serving.NonCompliant)
	}

	def := setMap["default"]
	if def.Total != 1 {
		t.Errorf("default total = %d, want 1", def.Total)
	}
	if def.Compliant != 1 {
		t.Errorf("default compliant = %d, want 1", def.Compliant)
	}
}

func TestComplianceReportEmpty(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)

	report, err := mgr.ComplianceReport(context.Background(), DefaultNamespace)
	if err != nil {
		t.Fatalf("ComplianceReport failed: %v", err)
	}
	if len(report) != 0 {
		t.Errorf("got %d sets, want 0", len(report))
	}
}

func TestComplianceReportListError(t *testing.T) {
	c := fakeClient()
	fake := c.Dynamic.(*dynamicfake.FakeDynamicClient)
	fake.PrependReactor("list", "managedclusters", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("list blocked")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.ComplianceReport(context.Background(), DefaultNamespace)
	if err == nil {
		t.Fatal("expected error when cluster list fails")
	}
}
