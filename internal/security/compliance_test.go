package security

import (
	"context"
	"fmt"
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

func complianceFakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	kinds := map[schema.GroupVersionResource]string{
		client.GVRManifestWork:     "ManifestWorkList",
		client.GVRPolicy:           "PolicyList",
		client.GVRPlacement:        "PlacementList",
		client.GVRPlacementBinding: "PlacementBindingList",
		client.GVROperatorPolicy:   "OperatorPolicyList",
	}
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, kinds, objs...)
	return &client.Client{Dynamic: fake}
}

func complianceManager(objs ...runtime.Object) *Manager {
	return New(complianceFakeClient(objs...), config.Config{}, discardLogger)
}

func TestDeployComplianceOperator(t *testing.T) {
	mgr := complianceManager()
	err := mgr.DeployComplianceOperator(context.Background(), "spoke1", "")
	if err != nil {
		t.Fatalf("DeployComplianceOperator failed: %v", err)
	}

	opName := complianceOperatorPolicyName("spoke1")
	_, err = mgr.client.Get(context.Background(), client.GVROperatorPolicy, DefaultNamespace, opName)
	if err != nil {
		t.Fatalf("OperatorPolicy not found: %v", err)
	}

	_, err = mgr.client.Get(context.Background(), client.GVRPolicy, DefaultNamespace, opName+"-health")
	if err != nil {
		t.Fatalf("Health policy not found: %v", err)
	}

	_, err = mgr.client.Get(context.Background(), client.GVRPlacement, DefaultNamespace, opName+"-placement")
	if err != nil {
		t.Fatalf("Placement not found: %v", err)
	}

	_, err = mgr.client.Get(context.Background(), client.GVRPlacementBinding, DefaultNamespace, opName+"-placement-binding")
	if err != nil {
		t.Fatalf("PlacementBinding not found: %v", err)
	}
}

func TestDeployComplianceOperatorWithClusterSet(t *testing.T) {
	mgr := complianceManager()
	err := mgr.DeployComplianceOperator(context.Background(), "spoke1", "team-gpu")
	if err != nil {
		t.Fatalf("DeployComplianceOperator failed: %v", err)
	}

	opName := complianceOperatorPolicyName("spoke1")
	placement, err := mgr.client.Get(context.Background(), client.GVRPlacement, DefaultNamespace, opName+"-placement")
	if err != nil {
		t.Fatalf("Placement not found: %v", err)
	}
	cs, _, _ := unstructured.NestedStringSlice(placement.Object, "spec", "clusterSets")
	if len(cs) != 1 || cs[0] != "team-gpu" {
		t.Errorf("clusterSets = %v, want [team-gpu]", cs)
	}
}

func TestDeployComplianceOperatorIdempotent(t *testing.T) {
	mgr := complianceManager()
	if err := mgr.DeployComplianceOperator(context.Background(), "spoke1", ""); err != nil {
		t.Fatalf("first deploy: %v", err)
	}
	if err := mgr.DeployComplianceOperator(context.Background(), "spoke1", ""); err != nil {
		t.Fatalf("second deploy should be idempotent: %v", err)
	}
}

func TestDeployComplianceOperatorError(t *testing.T) {
	c := complianceFakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "operatorpolicies", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("forbidden")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.DeployComplianceOperator(context.Background(), "spoke1", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "creating compliance operator policy") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCreateComplianceScan(t *testing.T) {
	mgr := complianceManager()
	opts := ComplianceScanOpts{
		Cluster: "spoke1",
		Profile: "ocp4-cis",
	}
	err := mgr.CreateComplianceScan(context.Background(), opts)
	if err != nil {
		t.Fatalf("CreateComplianceScan failed: %v", err)
	}

	scanName := complianceScanPolicyName("spoke1")
	policy, err := mgr.client.Get(context.Background(), client.GVRPolicy, DefaultNamespace, scanName)
	if err != nil {
		t.Fatalf("Scan policy not found: %v", err)
	}

	labels := policy.GetLabels()
	if labels["acmlab.redhat.com/compliance-profile"] != "ocp4-cis" {
		t.Errorf("profile label = %q, want ocp4-cis", labels["acmlab.redhat.com/compliance-profile"])
	}

	_, err = mgr.client.Get(context.Background(), client.GVRPlacement, DefaultNamespace, scanName+"-placement")
	if err != nil {
		t.Fatalf("Scan placement not found: %v", err)
	}

	_, err = mgr.client.Get(context.Background(), client.GVRPlacementBinding, DefaultNamespace, scanName+"-placement-binding")
	if err != nil {
		t.Fatalf("Scan placement binding not found: %v", err)
	}
}

func TestCreateComplianceScanDefaultProfile(t *testing.T) {
	mgr := complianceManager()
	opts := ComplianceScanOpts{
		Cluster: "spoke1",
	}
	err := mgr.CreateComplianceScan(context.Background(), opts)
	if err != nil {
		t.Fatalf("CreateComplianceScan failed: %v", err)
	}

	scanName := complianceScanPolicyName("spoke1")
	policy, err := mgr.client.Get(context.Background(), client.GVRPolicy, DefaultNamespace, scanName)
	if err != nil {
		t.Fatalf("Scan policy not found: %v", err)
	}
	labels := policy.GetLabels()
	if labels["acmlab.redhat.com/compliance-profile"] != "ocp4-cis" {
		t.Errorf("profile label = %q, want ocp4-cis (default)", labels["acmlab.redhat.com/compliance-profile"])
	}
}

func TestCreateComplianceScanError(t *testing.T) {
	c := complianceFakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "policies", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("forbidden")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.CreateComplianceScan(context.Background(), ComplianceScanOpts{Cluster: "spoke1", Profile: "ocp4-cis"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "creating compliance scan policy") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetComplianceStatusNotDeployed(t *testing.T) {
	mgr := complianceManager()
	status, err := mgr.GetComplianceStatus(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("GetComplianceStatus failed: %v", err)
	}
	if status.Phase != "NotDeployed" {
		t.Errorf("Phase = %q, want NotDeployed", status.Phase)
	}
}

func TestGetComplianceStatusOperatorOnly(t *testing.T) {
	mgr := complianceManager()
	if err := mgr.DeployComplianceOperator(context.Background(), "spoke1", ""); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	status, err := mgr.GetComplianceStatus(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("GetComplianceStatus failed: %v", err)
	}
	if status.Phase != "OperatorDeployed" {
		t.Errorf("Phase = %q, want OperatorDeployed", status.Phase)
	}
}

func TestGetComplianceStatusWithScan(t *testing.T) {
	mgr := complianceManager()
	if err := mgr.DeployComplianceOperator(context.Background(), "spoke1", ""); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	opts := ComplianceScanOpts{Cluster: "spoke1", Profile: "ocp4-cis"}
	if err := mgr.CreateComplianceScan(context.Background(), opts); err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	status, err := mgr.GetComplianceStatus(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("GetComplianceStatus failed: %v", err)
	}
	if status.Cluster != "spoke1" {
		t.Errorf("Cluster = %q", status.Cluster)
	}
}

func TestGetComplianceStatusWithResults(t *testing.T) {
	scanPolicy := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1",
			"kind":       "Policy",
			"metadata": map[string]interface{}{
				"name":      "compliance-scan-spoke1",
				"namespace": DefaultNamespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/compliance-profile": "ocp4-cis",
				},
			},
			"status": map[string]interface{}{
				"compliant": "NonCompliant",
				"details": []interface{}{
					map[string]interface{}{"rule": "rule-1", "status": "PASS", "severity": "high"},
					map[string]interface{}{"rule": "rule-2", "status": "FAIL", "severity": "medium"},
					map[string]interface{}{"rule": "rule-3", "status": "PASS", "severity": "low"},
				},
			},
		},
	}

	opPolicy := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1beta1",
			"kind":       "OperatorPolicy",
			"metadata": map[string]interface{}{
				"name":      "compliance-operator-spoke1",
				"namespace": DefaultNamespace,
			},
		},
	}

	mgr := complianceManager(opPolicy, scanPolicy)
	status, err := mgr.GetComplianceStatus(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("GetComplianceStatus failed: %v", err)
	}
	if status.Phase != "Done" {
		t.Errorf("Phase = %q, want Done", status.Phase)
	}
	if status.Compliant != 2 {
		t.Errorf("Compliant = %d, want 2", status.Compliant)
	}
	if status.NonCompliant != 1 {
		t.Errorf("NonCompliant = %d, want 1", status.NonCompliant)
	}
	if status.Profile != "ocp4-cis" {
		t.Errorf("Profile = %q, want ocp4-cis", status.Profile)
	}
}

func TestGetComplianceReport(t *testing.T) {
	scanPolicy := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1",
			"kind":       "Policy",
			"metadata": map[string]interface{}{
				"name":      "compliance-scan-spoke1",
				"namespace": DefaultNamespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/compliance-profile": "ocp4-cis",
				},
			},
			"status": map[string]interface{}{
				"compliant": "Compliant",
				"details": []interface{}{
					map[string]interface{}{"rule": "xccdf_rule_1", "status": "PASS", "severity": "high", "detail": "test passed"},
					map[string]interface{}{"rule": "xccdf_rule_2", "status": "FAIL", "severity": "medium", "detail": "etcd not encrypted"},
				},
			},
		},
	}

	opPolicy := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1beta1",
			"kind":       "OperatorPolicy",
			"metadata": map[string]interface{}{
				"name":      "compliance-operator-spoke1",
				"namespace": DefaultNamespace,
			},
		},
	}

	mgr := complianceManager(opPolicy, scanPolicy)
	report, err := mgr.GetComplianceReport(context.Background(), "spoke1", "ocp4-cis")
	if err != nil {
		t.Fatalf("GetComplianceReport failed: %v", err)
	}
	if report.Cluster != "spoke1" {
		t.Errorf("Cluster = %q", report.Cluster)
	}
	if len(report.Results) != 2 {
		t.Fatalf("Results count = %d, want 2", len(report.Results))
	}
	if report.Results[0].Rule != "xccdf_rule_1" {
		t.Errorf("Results[0].Rule = %q", report.Results[0].Rule)
	}
	if report.Results[1].Status != "FAIL" {
		t.Errorf("Results[1].Status = %q, want FAIL", report.Results[1].Status)
	}
}

func TestGetComplianceReportNotDeployed(t *testing.T) {
	mgr := complianceManager()
	report, err := mgr.GetComplianceReport(context.Background(), "spoke1", "ocp4-cis")
	if err != nil {
		t.Fatalf("GetComplianceReport failed: %v", err)
	}
	if len(report.Results) != 0 {
		t.Errorf("Results count = %d, want 0 when not deployed", len(report.Results))
	}
}

func TestRemoveComplianceScan(t *testing.T) {
	mgr := complianceManager()
	if err := mgr.DeployComplianceOperator(context.Background(), "spoke1", ""); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	opts := ComplianceScanOpts{Cluster: "spoke1", Profile: "ocp4-cis"}
	if err := mgr.CreateComplianceScan(context.Background(), opts); err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	err := mgr.RemoveComplianceScan(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("RemoveComplianceScan failed: %v", err)
	}

	scanName := complianceScanPolicyName("spoke1")
	_, err = mgr.client.Get(context.Background(), client.GVRPolicy, DefaultNamespace, scanName)
	if err == nil {
		t.Error("scan policy should not exist after removal")
	}

	opName := complianceOperatorPolicyName("spoke1")
	_, err = mgr.client.Get(context.Background(), client.GVROperatorPolicy, DefaultNamespace, opName)
	if err == nil {
		t.Error("operator policy should not exist after removal")
	}
}

func TestRemoveComplianceScanIdempotent(t *testing.T) {
	mgr := complianceManager()
	err := mgr.RemoveComplianceScan(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("RemoveComplianceScan should not error for nonexistent: %v", err)
	}
}

func TestBuildComplianceOperatorPolicy(t *testing.T) {
	pol := buildComplianceOperatorPolicy("spoke1")
	if pol.GetName() != "compliance-operator-spoke1" {
		t.Errorf("Name = %q", pol.GetName())
	}
	labels := pol.GetLabels()
	if labels["acmlab.redhat.com/compliance-operator"] != "true" {
		t.Error("missing compliance-operator label")
	}
	sub, _, _ := unstructured.NestedString(pol.Object, "spec", "subscription", "name")
	if sub != "compliance-operator" {
		t.Errorf("subscription name = %q", sub)
	}
}

func TestBuildComplianceScanPolicy(t *testing.T) {
	opts := ComplianceScanOpts{Cluster: "spoke1", Profile: "ocp4-moderate"}
	pol := buildComplianceScanPolicy(opts)
	if pol.GetName() != "compliance-scan-spoke1" {
		t.Errorf("Name = %q", pol.GetName())
	}
	labels := pol.GetLabels()
	if labels["acmlab.redhat.com/compliance-profile"] != "ocp4-moderate" {
		t.Errorf("profile label = %q", labels["acmlab.redhat.com/compliance-profile"])
	}
}

func TestBuildCompliancePlacementWithClusterSet(t *testing.T) {
	p := buildCompliancePlacement("spoke1", "team-gpu")
	cs, _, _ := unstructured.NestedStringSlice(p.Object, "spec", "clusterSets")
	if len(cs) != 1 || cs[0] != "team-gpu" {
		t.Errorf("clusterSets = %v, want [team-gpu]", cs)
	}
}

func TestBuildCompliancePlacementNoClusterSet(t *testing.T) {
	p := buildCompliancePlacement("spoke1", "")
	_, found, _ := unstructured.NestedStringSlice(p.Object, "spec", "clusterSets")
	if found {
		t.Error("clusterSets should not be set when empty")
	}
}

func TestParseComplianceScanStatusCompliant(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				"acmlab.redhat.com/compliance-profile": "ocp4-cis",
			},
		},
		"status": map[string]interface{}{
			"compliant": "Compliant",
			"details": []interface{}{
				map[string]interface{}{"rule": "r1", "status": "PASS"},
				map[string]interface{}{"rule": "r2", "status": "PASS"},
			},
		},
	}
	s := parseComplianceScanStatus("spoke1", obj)
	if s.Phase != "Done" {
		t.Errorf("Phase = %q, want Done", s.Phase)
	}
	if s.Compliant != 2 {
		t.Errorf("Compliant = %d, want 2", s.Compliant)
	}
	if s.Profile != "ocp4-cis" {
		t.Errorf("Profile = %q", s.Profile)
	}
}

func TestParseComplianceScanStatusPending(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{},
		"status": map[string]interface{}{
			"compliant": "Pending",
		},
	}
	s := parseComplianceScanStatus("spoke1", obj)
	if s.Phase != "Running" {
		t.Errorf("Phase = %q, want Running", s.Phase)
	}
}

func TestParseComplianceScanStatusNoStatus(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{},
	}
	s := parseComplianceScanStatus("spoke1", obj)
	if s.Phase != "Pending" {
		t.Errorf("Phase = %q, want Pending", s.Phase)
	}
}

func TestExtractComplianceResultsEmpty(t *testing.T) {
	obj := map[string]interface{}{}
	results := extractComplianceResults(obj)
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestExtractComplianceResultsWithData(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{
			"details": []interface{}{
				map[string]interface{}{"rule": "r1", "status": "PASS", "severity": "high"},
				map[string]interface{}{"rule": "r2", "status": "FAIL", "severity": "medium", "detail": "not encrypted"},
				map[string]interface{}{"rule": "", "status": "PASS"},
			},
		},
	}
	results := extractComplianceResults(obj)
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2 (empty rule is skipped)", len(results))
	}
	if results[1].Detail != "not encrypted" {
		t.Errorf("results[1].Detail = %q", results[1].Detail)
	}
}

func TestComplianceOperatorPolicyName(t *testing.T) {
	if complianceOperatorPolicyName("spoke1") != "compliance-operator-spoke1" {
		t.Error("unexpected name")
	}
}

func TestComplianceScanPolicyName(t *testing.T) {
	if complianceScanPolicyName("spoke1") != "compliance-scan-spoke1" {
		t.Error("unexpected name")
	}
}

func TestExtractPolicyConditions(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Compliant", "status": "True"},
				map[string]interface{}{"type": "Available", "status": "True"},
			},
		},
	}
	conds := extractPolicyConditions(obj)
	if len(conds) != 2 {
		t.Errorf("got %d conditions, want 2", len(conds))
	}
	if conds[0] != "Compliant=True" {
		t.Errorf("conds[0] = %q", conds[0])
	}
}

func TestExtractPolicyConditionsEmpty(t *testing.T) {
	obj := map[string]interface{}{}
	conds := extractPolicyConditions(obj)
	if len(conds) != 0 {
		t.Errorf("got %d conditions, want 0", len(conds))
	}
}

func TestStringVal(t *testing.T) {
	m := map[string]interface{}{"key": "val", "num": 42}
	if stringVal(m, "key") != "val" {
		t.Error("expected val")
	}
	if stringVal(m, "num") != "" {
		t.Error("expected empty for non-string")
	}
	if stringVal(m, "missing") != "" {
		t.Error("expected empty for missing")
	}
}
