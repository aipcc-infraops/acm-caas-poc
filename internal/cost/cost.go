package cost

import (
	"context"
	"fmt"
	"log/slog"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

type ClusterCost struct {
	Name           string  `json:"name"`
	Nodes          int     `json:"nodes"`
	CPUCores       int     `json:"cpuCores"`
	MemoryGiB      float64 `json:"memoryGiB"`
	InstanceType   string  `json:"instanceType"`
	DailyEstimate  float64 `json:"dailyEstimate"`
	PeriodEstimate float64 `json:"periodEstimate"`
	Days           int     `json:"days"`
}

type TenantCost struct {
	Cluster       string  `json:"cluster"`
	Namespace     string  `json:"namespace"`
	CPUHours      float64 `json:"cpuHours"`
	MemoryGiBHours float64 `json:"memoryGiBHours"`
	Days          int     `json:"days"`
}

type CostReport struct {
	Clusters    []ClusterCost `json:"clusters"`
	TotalCost   float64       `json:"totalCost"`
	Days        int           `json:"days"`
}

type Manager struct {
	client *client.Client
	cfg    config.Config
	logger *slog.Logger
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger}
}

func (m *Manager) GetClusterCost(ctx context.Context, cluster string, days int) (*ClusterCost, error) {
	m.logger.Info("cost.GetClusterCost", "cluster", cluster, "days", days)
	if days <= 0 {
		return nil, fmt.Errorf("days must be positive, got %d", days)
	}

	info, err := m.client.Get(ctx, client.GVRManagedClusterInfo, cluster, cluster)
	if err != nil {
		return nil, fmt.Errorf("getting ManagedClusterInfo %s: %w", cluster, err)
	}

	nodes := extractNodeInfo(info.Object)
	cost := calculateClusterCost(cluster, nodes, days)
	return &cost, nil
}

func (m *Manager) GenerateReport(ctx context.Context, days int) (*CostReport, error) {
	m.logger.Info("cost.GenerateReport", "days", days)
	if days <= 0 {
		return nil, fmt.Errorf("days must be positive, got %d", days)
	}

	list, err := m.client.List(ctx, client.GVRManagedClusterInfo, "", "")
	if err != nil {
		return nil, fmt.Errorf("listing ManagedClusterInfo: %w", err)
	}

	report := &CostReport{Days: days}
	for _, item := range list.Items {
		nodes := extractNodeInfo(item.Object)
		cost := calculateClusterCost(item.GetName(), nodes, days)
		report.Clusters = append(report.Clusters, cost)
		report.TotalCost += cost.PeriodEstimate
	}
	return report, nil
}

func extractNodeInfo(obj map[string]interface{}) []nodeData {
	nodeList, _, _ := unstructured.NestedSlice(obj, "status", "nodeList")
	var nodes []nodeData
	for _, raw := range nodeList {
		node, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		labels, _, _ := unstructured.NestedStringMap(node, "labels")
		if _, isWorker := labels["node-role.kubernetes.io/worker"]; !isWorker {
			continue
		}
		nd := nodeData{
			InstanceType: labels["node.kubernetes.io/instance-type"],
		}
		if capacity, ok := node["capacity"].(map[string]interface{}); ok {
			nd.CPU, _ = capacity["cpu"].(string)
			nd.Memory, _ = capacity["memory"].(string)
		}
		nodes = append(nodes, nd)
	}
	return nodes
}
