package cost

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
	fc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRManagedClusterInfo: "ManagedClusterInfoList",
			client.GVRManagedCluster:     "ManagedClusterList",
			client.GVRPolicy:             "PolicyList",
			client.GVRPlacement:          "PlacementList",
			client.GVRPlacementBinding:   "PlacementBindingList",
		}, objs...)
	return &client.Client{Dynamic: fc}
}

func newTestManager(objs ...runtime.Object) *Manager {
	return New(fakeClient(objs...), config.Config{}, discardLogger)
}

func clusterInfoWithNodes(name string, nodes []map[string]interface{}) *unstructured.Unstructured {
	nodeList := make([]interface{}, len(nodes))
	for i, n := range nodes {
		nodeList[i] = n
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "internal.open-cluster-management.io/v1beta1",
			"kind":       "ManagedClusterInfo",
			"metadata":   map[string]interface{}{"name": name, "namespace": name},
			"status": map[string]interface{}{
				"nodeList": nodeList,
			},
		},
	}
}

func makeWorkerNode(instanceType, cpu, memory string) map[string]interface{} {
	return map[string]interface{}{
		"name": "worker-1",
		"labels": map[string]interface{}{
			"node-role.kubernetes.io/worker": "",
			"node.kubernetes.io/instance-type": instanceType,
		},
		"capacity": map[string]interface{}{
			"cpu":    cpu,
			"memory": memory,
		},
	}
}

func TestGetClusterCostKnownInstance(t *testing.T) {
	nodes := []map[string]interface{}{
		makeWorkerNode("m5.xlarge", "4", "16777216Ki"),
		makeWorkerNode("m5.xlarge", "4", "16777216Ki"),
	}
	info := clusterInfoWithNodes("spoke1", nodes)
	m := New(fakeClient(info), config.Config{}, discardLogger)

	cost, err := m.GetClusterCost(context.Background(), "spoke1", 30)
	if err != nil {
		t.Fatalf("GetClusterCost: %v", err)
	}

	if cost.Name != "spoke1" {
		t.Errorf("name = %s, want spoke1", cost.Name)
	}
	if cost.Nodes != 2 {
		t.Errorf("nodes = %d, want 2", cost.Nodes)
	}
	if cost.PeriodEstimate <= 0 {
		t.Error("expected positive period estimate")
	}
	if cost.DailyEstimate <= 0 {
		t.Error("expected positive daily estimate")
	}
}

func TestGetClusterCostUnknownInstanceUsesFallback(t *testing.T) {
	nodes := []map[string]interface{}{
		makeWorkerNode("custom-type", "8", "33554432Ki"),
	}
	info := clusterInfoWithNodes("spoke2", nodes)
	m := New(fakeClient(info), config.Config{}, discardLogger)

	cost, err := m.GetClusterCost(context.Background(), "spoke2", 7)
	if err != nil {
		t.Fatalf("GetClusterCost: %v", err)
	}

	expectedDaily := float64(8) * FallbackPricePerCPUHr * 24
	if diff := cost.DailyEstimate - expectedDaily; diff > 0.01 || diff < -0.01 {
		t.Errorf("daily = %.4f, want %.4f", cost.DailyEstimate, expectedDaily)
	}
}

func TestGetClusterCostInvalidDays(t *testing.T) {
	m := New(fakeClient(), config.Config{}, discardLogger)
	_, err := m.GetClusterCost(context.Background(), "spoke1", 0)
	if err == nil {
		t.Error("expected error for days=0")
	}
	_, err = m.GetClusterCost(context.Background(), "spoke1", -1)
	if err == nil {
		t.Error("expected error for negative days")
	}
}

func TestGenerateReportMultipleClusters(t *testing.T) {
	c1 := clusterInfoWithNodes("spoke1", []map[string]interface{}{
		makeWorkerNode("m5.xlarge", "4", "16777216Ki"),
	})
	c2 := clusterInfoWithNodes("spoke2", []map[string]interface{}{
		makeWorkerNode("bx2-8x32", "8", "33554432Ki"),
	})
	m := New(fakeClient(c1, c2), config.Config{}, discardLogger)

	report, err := m.GenerateReport(context.Background(), 30)
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}

	if len(report.Clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(report.Clusters))
	}
	if report.TotalCost <= 0 {
		t.Error("expected positive total cost")
	}
	if report.Days != 30 {
		t.Errorf("days = %d, want 30", report.Days)
	}
}

func TestGenerateReportEmptyFleet(t *testing.T) {
	m := New(fakeClient(), config.Config{}, discardLogger)
	report, err := m.GenerateReport(context.Background(), 30)
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}
	if len(report.Clusters) != 0 {
		t.Errorf("expected 0 clusters, got %d", len(report.Clusters))
	}
}

func TestCalculateClusterCostPureFunctionKnownType(t *testing.T) {
	nodes := []nodeData{
		{InstanceType: "m5.xlarge", CPU: "4", Memory: "16777216Ki"},
		{InstanceType: "m5.xlarge", CPU: "4", Memory: "16777216Ki"},
	}
	cost := calculateClusterCost("test", nodes, 30)

	if cost.Name != "test" {
		t.Errorf("name = %s, want test", cost.Name)
	}
	if cost.Nodes != 2 {
		t.Errorf("nodes = %d, want 2", cost.Nodes)
	}
	if cost.CPUCores != 8 {
		t.Errorf("cpu = %d, want 8", cost.CPUCores)
	}
	expectedCost := 0.192 * 24 * 30 * 2
	if cost.PeriodEstimate != expectedCost {
		t.Errorf("periodEstimate = %.2f, want %.2f", cost.PeriodEstimate, expectedCost)
	}
}

func TestCalculateClusterCostFallbackPricing(t *testing.T) {
	nodes := []nodeData{
		{InstanceType: "unknown-type", CPU: "16", Memory: "67108864Ki"},
	}
	cost := calculateClusterCost("test", nodes, 1)

	expectedCost := 16 * FallbackPricePerCPUHr * 24
	if cost.PeriodEstimate != expectedCost {
		t.Errorf("periodEstimate = %.2f, want %.2f", cost.PeriodEstimate, expectedCost)
	}
	if cost.InstanceType != "" {
		t.Errorf("instanceType = %s, want empty for unknown", cost.InstanceType)
	}
}

func TestCalculateClusterCostNoNodes(t *testing.T) {
	cost := calculateClusterCost("empty", nil, 30)
	if cost.PeriodEstimate != 0 {
		t.Errorf("expected 0 cost, got %.2f", cost.PeriodEstimate)
	}
	if cost.Nodes != 0 {
		t.Errorf("expected 0 nodes, got %d", cost.Nodes)
	}
}

func TestCalculateTenantCost(t *testing.T) {
	tc := calculateTenantCost("spoke1", "team-alpha", 100.5, 200.0, 7)
	if tc.Cluster != "spoke1" {
		t.Errorf("cluster = %s, want spoke1", tc.Cluster)
	}
	if tc.Namespace != "team-alpha" {
		t.Errorf("namespace = %s, want team-alpha", tc.Namespace)
	}
	if tc.CPUHours != 100.5 {
		t.Errorf("cpuHours = %.1f, want 100.5", tc.CPUHours)
	}
}

func TestFormatCostCSV(t *testing.T) {
	report := CostReport{
		Clusters: []ClusterCost{
			{Name: "spoke1", Nodes: 3, CPUCores: 12, MemoryGiB: 48, DailyEstimate: 13.82, PeriodEstimate: 414.72, Days: 30},
		},
		TotalCost: 414.72,
		Days:      30,
	}
	csv := formatCostCSV(report)
	if !contains(csv, "spoke1") {
		t.Error("CSV missing cluster name")
	}
	if !contains(csv, "TOTAL") {
		t.Error("CSV missing TOTAL row")
	}
	lines := len(splitLines(csv))
	if lines != 3 {
		t.Errorf("expected 3 lines (header+data+total), got %d", lines)
	}
}

func TestFormatCostJSON(t *testing.T) {
	report := CostReport{
		Clusters:  []ClusterCost{{Name: "spoke1", Nodes: 2, PeriodEstimate: 100.0, Days: 7}},
		TotalCost: 100.0,
		Days:      7,
	}
	data, err := formatCostJSON(report)
	if err != nil {
		t.Fatalf("formatCostJSON: %v", err)
	}
	if !contains(string(data), "spoke1") {
		t.Error("JSON missing cluster name")
	}
}

func TestParseCPU(t *testing.T) {
	tests := []struct{ input string; want int }{
		{"4", 4},
		{"16", 16},
		{"", 0},
		{"abc", 0},
	}
	for _, tt := range tests {
		if got := parseCPU(tt.input); got != tt.want {
			t.Errorf("parseCPU(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseMemoryGiB(t *testing.T) {
	tests := []struct{ input string; want float64 }{
		{"16777216Ki", 16.0},
		{"33554432Ki", 32.0},
		{"", 0},
		{"invalid", 0},
	}
	for _, tt := range tests {
		got := parseMemoryGiB(tt.input)
		if got != tt.want {
			t.Errorf("parseMemoryGiB(%q) = %.1f, want %.1f", tt.input, got, tt.want)
		}
	}
}

func TestExtractNodeInfo(t *testing.T) {
	obj := map[string]interface{}{
		"status": map[string]interface{}{
			"nodeList": []interface{}{
				makeWorkerNode("m5.xlarge", "4", "16777216Ki"),
				map[string]interface{}{
					"name": "master-1",
					"labels": map[string]interface{}{
						"node-role.kubernetes.io/master": "",
					},
					"capacity": map[string]interface{}{"cpu": "8", "memory": "33554432Ki"},
				},
			},
		},
	}

	nodes := extractNodeInfo(obj)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 worker node, got %d", len(nodes))
	}
	if nodes[0].InstanceType != "m5.xlarge" {
		t.Errorf("instanceType = %s, want m5.xlarge", nodes[0].InstanceType)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if line != "" {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
