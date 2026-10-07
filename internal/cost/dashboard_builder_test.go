package cost

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestCostMetricsAllowlist(t *testing.T) {
	metrics := CostMetricsAllowlist()
	if len(metrics) == 0 {
		t.Fatal("allowlist should not be empty")
	}
	expected := map[string]bool{
		"kube_node_status_capacity": true,
		"kube_node_info":            true,
		"kube_node_labels":          true,
	}
	for _, m := range metrics {
		if !expected[m] {
			t.Errorf("unexpected metric %q in allowlist", m)
		}
	}
}

func TestCostRecordingRulesYAML(t *testing.T) {
	rules := CostRecordingRulesYAML(0.048)
	if !strings.Contains(rules, "groups:") {
		t.Error("rules YAML missing groups key")
	}
	if !strings.Contains(rules, CostRulesGroup) {
		t.Errorf("rules YAML missing group name %q", CostRulesGroup)
	}
	expectedRecords := []string{
		"acmlab_cost:cluster_cpu_cores:sum",
		"acmlab_cost:cluster_memory_gib:sum",
		"acmlab_cost:cluster_worker_nodes:sum",
		"acmlab_cost:node_hourly_price",
		"acmlab_cost:cluster_hourly_rate:sum",
		"acmlab_cost:cluster_daily_estimate:sum",
		"acmlab_cost:cluster_monthly_estimate:sum",
		"acmlab_cost:cluster_monthly_actual:sum",
	}
	for _, rec := range expectedRecords {
		if !strings.Contains(rules, rec) {
			t.Errorf("rules YAML missing recording rule %q", rec)
		}
	}
	if !strings.Contains(rules, "0.048") {
		t.Error("rules YAML should contain the fallback price per CPU hour")
	}
}

func TestCostRecordingRulesContainInstancePricing(t *testing.T) {
	rules := CostRecordingRulesYAML(0.048)
	for instanceType, pricing := range DefaultPricing {
		if !strings.Contains(rules, instanceType) {
			t.Errorf("rules YAML missing instance type %q", instanceType)
		}
		priceStr := fmt.Sprintf("%.3f", pricing.PricePerHr)
		if !strings.Contains(rules, priceStr) {
			t.Errorf("rules YAML missing price %s for %s", priceStr, instanceType)
		}
	}
}

func TestCostRecordingRulesActualUsesAvgOverTime(t *testing.T) {
	rules := CostRecordingRulesYAML(0.048)
	if !strings.Contains(rules, "avg_over_time") {
		t.Error("actual cost rule should use avg_over_time for real usage tracking")
	}
	if !strings.Contains(rules, "[30d]") {
		t.Error("actual cost rule should look back 30 days")
	}
}

func TestCostRecordingRulesCustomPrice(t *testing.T) {
	rules := CostRecordingRulesYAML(0.096)
	if !strings.Contains(rules, "0.096") {
		t.Error("rules YAML should reflect custom price per CPU hour")
	}
}

func TestCostDashboardJSON(t *testing.T) {
	dashJSON := CostDashboardJSON()
	if !json.Valid([]byte(dashJSON)) {
		t.Fatal("dashboard JSON is not valid")
	}
	var dash map[string]interface{}
	if err := json.Unmarshal([]byte(dashJSON), &dash); err != nil {
		t.Fatalf("dashboard JSON unmarshal: %v", err)
	}
	if dash["title"] != "ACMlab Fleet Cost Tracking" {
		t.Errorf("title = %q, want ACMlab Fleet Cost Tracking", dash["title"])
	}
	if dash["uid"] != "acmlab-cost-tracking" {
		t.Errorf("uid = %q, want acmlab-cost-tracking", dash["uid"])
	}
	panels, ok := dash["panels"].([]interface{})
	if !ok || len(panels) == 0 {
		t.Fatal("dashboard should have panels")
	}
	if len(panels) != 7 {
		t.Errorf("panel count = %d, want 7", len(panels))
	}
}

func TestCostDashboardPanelTypes(t *testing.T) {
	dashJSON := CostDashboardJSON()
	var dash map[string]interface{}
	json.Unmarshal([]byte(dashJSON), &dash)
	panels := dash["panels"].([]interface{})

	expectedTypes := map[string]bool{
		"stat":       false,
		"table":      false,
		"timeseries": false,
		"bargauge":   false,
	}
	for _, p := range panels {
		panel := p.(map[string]interface{})
		pt := panel["type"].(string)
		expectedTypes[pt] = true
	}
	for panelType, found := range expectedTypes {
		if !found {
			t.Errorf("dashboard missing panel type %q", panelType)
		}
	}
}

func TestCostDashboardPanelsContainCostMetrics(t *testing.T) {
	dashJSON := CostDashboardJSON()
	requiredExprs := []string{
		"acmlab_cost:cluster_monthly_estimate:sum",
		"acmlab_cost:cluster_monthly_actual:sum",
		"acmlab_cost:cluster_daily_estimate:sum",
		"acmlab_cost:cluster_cpu_cores:sum",
		"acmlab_cost:cluster_memory_gib:sum",
	}
	for _, expr := range requiredExprs {
		if !strings.Contains(dashJSON, expr) {
			t.Errorf("dashboard JSON missing metric expression %q", expr)
		}
	}
}

func TestDashboardNameAndConstants(t *testing.T) {
	if DashboardName == "" {
		t.Error("DashboardName should not be empty")
	}
	if CostRulesGroup == "" {
		t.Error("CostRulesGroup should not be empty")
	}
}
