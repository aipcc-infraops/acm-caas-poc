package cost

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

func managedClusterWithCostCenter(name, costCenter string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	obj.SetName(name)
	obj.SetLabels(map[string]string{"caas/cost-center": costCenter})
	return obj
}

func managedClusterNoCostCenter(name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster",
	})
	obj.SetName(name)
	obj.SetLabels(map[string]string{"vendor": "OpenShift"})
	return obj
}

func TestStampCostCenterSetsLabel(t *testing.T) {
	mc := managedClusterNoCostCenter("spoke1")
	m := newTestManager(mc)

	if err := m.StampCostCenter(context.Background(), "spoke1", "engineering"); err != nil {
		t.Fatalf("StampCostCenter: %v", err)
	}

	obj, err := m.client.Get(context.Background(), client.GVRManagedCluster, "", "spoke1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if obj.GetLabels()["caas/cost-center"] != "engineering" {
		t.Errorf("cost-center = %s, want engineering", obj.GetLabels()["caas/cost-center"])
	}
}

func TestStampCostCenterRejectsEmpty(t *testing.T) {
	mc := managedClusterNoCostCenter("spoke1")
	m := newTestManager(mc)

	if err := m.StampCostCenter(context.Background(), "spoke1", ""); err == nil {
		t.Error("expected error for empty cost center")
	}
}

func TestGetCostByCenterGroupsClusters(t *testing.T) {
	mc1 := managedClusterWithCostCenter("spoke1", "engineering")
	mc2 := managedClusterWithCostCenter("spoke2", "engineering")
	mc3 := managedClusterWithCostCenter("spoke3", "research")
	info1 := clusterInfoWithNodes("spoke1", []map[string]interface{}{makeWorkerNode("m5.xlarge", "4", "16777216Ki")})
	info2 := clusterInfoWithNodes("spoke2", []map[string]interface{}{makeWorkerNode("m5.xlarge", "4", "16777216Ki")})
	info3 := clusterInfoWithNodes("spoke3", []map[string]interface{}{makeWorkerNode("m5.2xlarge", "8", "33554432Ki")})

	m := newTestManager(mc1, mc2, mc3, info1, info2, info3)

	centers, err := m.GetCostByCenter(context.Background(), 30)
	if err != nil {
		t.Fatalf("GetCostByCenter: %v", err)
	}
	if len(centers) != 2 {
		t.Fatalf("expected 2 cost centers, got %d", len(centers))
	}

	found := map[string]CenterCost{}
	for _, c := range centers {
		found[c.CostCenter] = c
	}

	if eng, ok := found["engineering"]; !ok {
		t.Error("engineering center not found")
	} else if len(eng.Clusters) != 2 {
		t.Errorf("engineering clusters = %d, want 2", len(eng.Clusters))
	}

	if res, ok := found["research"]; !ok {
		t.Error("research center not found")
	} else if len(res.Clusters) != 1 {
		t.Errorf("research clusters = %d, want 1", len(res.Clusters))
	}
}

func TestGetCostByCenterSkipsUnlabelled(t *testing.T) {
	mc1 := managedClusterWithCostCenter("spoke1", "engineering")
	mc2 := managedClusterNoCostCenter("spoke2")
	info1 := clusterInfoWithNodes("spoke1", []map[string]interface{}{makeWorkerNode("m5.xlarge", "4", "16777216Ki")})

	m := newTestManager(mc1, mc2, info1)

	centers, err := m.GetCostByCenter(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetCostByCenter: %v", err)
	}
	if len(centers) != 1 {
		t.Fatalf("expected 1 cost center, got %d", len(centers))
	}
	if centers[0].CostCenter != "engineering" {
		t.Errorf("CostCenter = %s, want engineering", centers[0].CostCenter)
	}
}

func TestGetCostByCenterRejectsBadDays(t *testing.T) {
	m := newTestManager()
	if _, err := m.GetCostByCenter(context.Background(), 0); err == nil {
		t.Error("expected error for days=0")
	}
}

func TestCheckBudgetsFindsOverages(t *testing.T) {
	mc1 := managedClusterWithCostCenter("spoke1", "engineering")
	mc2 := managedClusterWithCostCenter("spoke2", "research")
	info1 := clusterInfoWithNodes("spoke1", []map[string]interface{}{
		makeWorkerNode("m5.xlarge", "4", "16777216Ki"),
		makeWorkerNode("m5.xlarge", "4", "16777216Ki"),
		makeWorkerNode("m5.xlarge", "4", "16777216Ki"),
	})
	info2 := clusterInfoWithNodes("spoke2", []map[string]interface{}{
		makeWorkerNode("m5.xlarge", "4", "16777216Ki"),
	})
	m := newTestManager(mc1, mc2, info1, info2)

	budgets := map[string]float64{
		"engineering": 10.0,
		"research":    10000.0,
	}
	alerts, err := m.CheckBudgets(context.Background(), 30, budgets)
	if err != nil {
		t.Fatalf("CheckBudgets: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].CostCenter != "engineering" {
		t.Errorf("alert CostCenter = %s, want engineering", alerts[0].CostCenter)
	}
	if alerts[0].Overage <= 0 {
		t.Error("expected positive overage")
	}
}

func TestCheckBudgetsReturnsEmptyWhenUnderBudget(t *testing.T) {
	mc1 := managedClusterWithCostCenter("spoke1", "engineering")
	info1 := clusterInfoWithNodes("spoke1", []map[string]interface{}{
		makeWorkerNode("m5.xlarge", "4", "16777216Ki"),
	})
	m := newTestManager(mc1, info1)

	budgets := map[string]float64{"engineering": 100000.0}
	alerts, err := m.CheckBudgets(context.Background(), 30, budgets)
	if err != nil {
		t.Fatalf("CheckBudgets: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(alerts))
	}
}

func TestCreateBudgetPolicyCreatesResources(t *testing.T) {
	m := newTestManager()
	ns := "open-cluster-management-global-set"

	if err := m.CreateBudgetPolicy(context.Background(), "engineering", 5000.0); err != nil {
		t.Fatalf("CreateBudgetPolicy: %v", err)
	}

	policy, err := m.client.Get(context.Background(), client.GVRPolicy, ns, "budget-engineering")
	if err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if policy.GetName() != "budget-engineering" {
		t.Errorf("policy name = %s, want budget-engineering", policy.GetName())
	}

	_, err = m.client.Get(context.Background(), client.GVRPlacement, ns, "budget-engineering-placement")
	if err != nil {
		t.Fatalf("get placement: %v", err)
	}

	_, err = m.client.Get(context.Background(), client.GVRPlacementBinding, ns, "budget-engineering-placement-binding")
	if err != nil {
		t.Fatalf("get placement binding: %v", err)
	}
}

func TestCreateBudgetPolicyRejectsInvalidInput(t *testing.T) {
	m := newTestManager()

	if err := m.CreateBudgetPolicy(context.Background(), "", 5000.0); err == nil {
		t.Error("expected error for empty cost center")
	}
	if err := m.CreateBudgetPolicy(context.Background(), "engineering", 0); err == nil {
		t.Error("expected error for zero budget")
	}
	if err := m.CreateBudgetPolicy(context.Background(), "engineering", -100); err == nil {
		t.Error("expected error for negative budget")
	}
}

func TestRemoveBudgetPolicyDeletesResources(t *testing.T) {
	m := newTestManager()

	if err := m.CreateBudgetPolicy(context.Background(), "engineering", 5000.0); err != nil {
		t.Fatalf("setup: %v", err)
	}

	removed, err := m.RemoveBudgetPolicy(context.Background(), "engineering")
	if err != nil {
		t.Fatalf("RemoveBudgetPolicy: %v", err)
	}
	if !removed {
		t.Error("expected removed=true")
	}
}

func TestRemoveBudgetPolicyNotFoundReturnsFalse(t *testing.T) {
	m := newTestManager()

	removed, err := m.RemoveBudgetPolicy(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("RemoveBudgetPolicy: %v", err)
	}
	if removed {
		t.Error("expected removed=false for nonexistent")
	}
}

func TestGroupByCostCenter(t *testing.T) {
	attributions := []CostCenterAttribution{
		{Cluster: "a", CostCenter: "eng"},
		{Cluster: "b", CostCenter: "eng"},
		{Cluster: "c", CostCenter: "ops"},
	}
	costs := map[string]ClusterCost{
		"a": {Name: "a", PeriodEstimate: 100},
		"b": {Name: "b", PeriodEstimate: 200},
		"c": {Name: "c", PeriodEstimate: 50},
	}

	result := groupByCostCenter(attributions, costs, 30)
	if len(result) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(result))
	}

	found := map[string]CenterCost{}
	for _, r := range result {
		found[r.CostCenter] = r
	}
	if found["eng"].TotalEstimate != 300 {
		t.Errorf("eng total = %.2f, want 300", found["eng"].TotalEstimate)
	}
	if found["ops"].TotalEstimate != 50 {
		t.Errorf("ops total = %.2f, want 50", found["ops"].TotalEstimate)
	}
}

func TestFindOverBudget(t *testing.T) {
	centers := []CenterCost{
		{CostCenter: "eng", TotalEstimate: 500},
		{CostCenter: "ops", TotalEstimate: 100},
	}
	budgets := map[string]float64{"eng": 200, "ops": 500}

	alerts := findOverBudget(centers, budgets)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].CostCenter != "eng" {
		t.Errorf("alert center = %s, want eng", alerts[0].CostCenter)
	}
	if alerts[0].Overage != 300 {
		t.Errorf("overage = %.2f, want 300", alerts[0].Overage)
	}
}

func TestFindOverBudgetReturnsEmptyWhenNoneOver(t *testing.T) {
	centers := []CenterCost{{CostCenter: "eng", TotalEstimate: 100}}
	budgets := map[string]float64{"eng": 500}

	alerts := findOverBudget(centers, budgets)
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(alerts))
	}
}

func TestParseCostCenterAttribution(t *testing.T) {
	tests := []struct {
		labels map[string]string
		want   string
	}{
		{map[string]string{"caas/cost-center": "engineering"}, "engineering"},
		{map[string]string{"vendor": "OpenShift"}, ""},
		{map[string]string{}, ""},
	}
	for _, tt := range tests {
		got := parseCostCenterAttribution(tt.labels)
		if got != tt.want {
			t.Errorf("parseCostCenterAttribution(%v) = %s, want %s", tt.labels, got, tt.want)
		}
	}
}

func TestBuildCostCenterPatch(t *testing.T) {
	patch := buildCostCenterPatch("research")
	meta := patch["metadata"].(map[string]interface{})
	labels := meta["labels"].(map[string]interface{})
	if labels["caas/cost-center"] != "research" {
		t.Errorf("cost-center = %v, want research", labels["caas/cost-center"])
	}
}

func TestBuildBudgetPolicyStructure(t *testing.T) {
	policy := buildBudgetPolicy("budget-eng", "ns", "eng", 5000)
	if policy["kind"] != "Policy" {
		t.Errorf("kind = %v, want Policy", policy["kind"])
	}
	meta := policy["metadata"].(map[string]interface{})
	labels := meta["labels"].(map[string]interface{})
	if labels["caas/cost-center"] != "eng" {
		t.Errorf("cost-center label = %v, want eng", labels["caas/cost-center"])
	}
	if labels["caas/budget-limit"] != "5000" {
		t.Errorf("budget-limit = %v, want 5000", labels["caas/budget-limit"])
	}
}

func TestBuildBudgetPlacement(t *testing.T) {
	p := buildBudgetPlacement("budget-eng", "ns", "eng")
	if p["kind"] != "Placement" {
		t.Errorf("kind = %v, want Placement", p["kind"])
	}
}

func TestBuildBudgetPlacementBinding(t *testing.T) {
	b := buildBudgetPlacementBinding("budget-eng", "ns")
	if b["kind"] != "PlacementBinding" {
		t.Errorf("kind = %v, want PlacementBinding", b["kind"])
	}
	ref := b["placementRef"].(map[string]interface{})
	if ref["name"] != "budget-eng-placement" {
		t.Errorf("placementRef name = %v, want budget-eng-placement", ref["name"])
	}
}
