package cost

import (
	"context"
	"encoding/json"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type CostCenterAttribution struct {
	Cluster    string `json:"cluster"`
	CostCenter string `json:"costCenter"`
}

type CenterCost struct {
	CostCenter     string        `json:"costCenter"`
	Clusters       []ClusterCost `json:"clusters"`
	TotalEstimate  float64       `json:"totalEstimate"`
	BudgetLimit    float64       `json:"budgetLimit,omitempty"`
	OverBudget     bool          `json:"overBudget"`
	Days           int           `json:"days"`
}

type BudgetAlert struct {
	CostCenter    string  `json:"costCenter"`
	TotalEstimate float64 `json:"totalEstimate"`
	BudgetLimit   float64 `json:"budgetLimit"`
	Overage       float64 `json:"overage"`
}

func (m *Manager) StampCostCenter(ctx context.Context, cluster, costCenter string) error {
	m.logger.Info("cost.StampCostCenter", "cluster", cluster, "costCenter", costCenter)
	if costCenter == "" {
		return fmt.Errorf("cost center must not be empty")
	}

	patch := buildCostCenterPatch(costCenter)
	data, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshaling cost center patch: %w", err)
	}

	_, err = m.client.Patch(ctx, client.GVRManagedCluster, "", cluster, types.MergePatchType, data)
	if err != nil {
		return fmt.Errorf("patching ManagedCluster %s cost center: %w", cluster, err)
	}
	return nil
}

func (m *Manager) GetCostByCenter(ctx context.Context, days int) ([]CenterCost, error) {
	m.logger.Info("cost.GetCostByCenter", "days", days)
	if days <= 0 {
		return nil, fmt.Errorf("days must be positive, got %d", days)
	}

	clusters, err := m.client.List(ctx, client.GVRManagedCluster, "", "")
	if err != nil {
		return nil, fmt.Errorf("listing ManagedClusters: %w", err)
	}

	attributions := make([]CostCenterAttribution, 0)
	for _, item := range clusters.Items {
		labels := item.GetLabels()
		cc := labels["caas/cost-center"]
		if cc == "" {
			continue
		}
		attributions = append(attributions, CostCenterAttribution{
			Cluster:    item.GetName(),
			CostCenter: cc,
		})
	}

	report, err := m.GenerateReport(ctx, days)
	if err != nil {
		return nil, fmt.Errorf("generating cost report: %w", err)
	}

	costByCluster := make(map[string]ClusterCost)
	for _, c := range report.Clusters {
		costByCluster[c.Name] = c
	}

	return groupByCostCenter(attributions, costByCluster, days), nil
}

func (m *Manager) CheckBudgets(ctx context.Context, days int, budgets map[string]float64) ([]BudgetAlert, error) {
	m.logger.Info("cost.CheckBudgets", "days", days)
	centers, err := m.GetCostByCenter(ctx, days)
	if err != nil {
		return nil, err
	}

	for i := range centers {
		if limit, ok := budgets[centers[i].CostCenter]; ok {
			centers[i].BudgetLimit = limit
			centers[i].OverBudget = centers[i].TotalEstimate > limit
		}
	}

	return findOverBudget(centers, budgets), nil
}

func (m *Manager) CreateBudgetPolicy(ctx context.Context, costCenter string, budgetLimit float64) error {
	m.logger.Info("cost.CreateBudgetPolicy", "costCenter", costCenter, "budget", budgetLimit)
	if costCenter == "" {
		return fmt.Errorf("cost center must not be empty")
	}
	if budgetLimit <= 0 {
		return fmt.Errorf("budget limit must be positive, got %.2f", budgetLimit)
	}

	ns := "open-cluster-management-global-set"
	policyName := fmt.Sprintf("budget-%s", costCenter)

	policy := &unstructured.Unstructured{Object: buildBudgetPolicy(policyName, ns, costCenter, budgetLimit)}
	if err := m.client.CreateIfNotExists(ctx, client.GVRPolicy, ns, policy); err != nil {
		return fmt.Errorf("creating budget policy %s: %w", policyName, err)
	}

	placement := &unstructured.Unstructured{Object: buildBudgetPlacement(policyName, ns, costCenter)}
	if err := m.client.CreateIfNotExists(ctx, client.GVRPlacement, ns, placement); err != nil {
		return fmt.Errorf("creating budget placement: %w", err)
	}

	binding := &unstructured.Unstructured{Object: buildBudgetPlacementBinding(policyName, ns)}
	if err := m.client.CreateIfNotExists(ctx, client.GVRPlacementBinding, ns, binding); err != nil {
		return fmt.Errorf("creating budget placement binding: %w", err)
	}

	return nil
}

func (m *Manager) RemoveBudgetPolicy(ctx context.Context, costCenter string) (bool, error) {
	m.logger.Info("cost.RemoveBudgetPolicy", "costCenter", costCenter)
	ns := "open-cluster-management-global-set"
	policyName := fmt.Sprintf("budget-%s", costCenter)

	_, err := m.client.Get(ctx, client.GVRPolicy, ns, policyName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking budget policy %s: %w", policyName, err)
	}

	if err := m.client.DeleteIfExists(ctx, client.GVRPlacementBinding, ns, policyName+"-placement-binding"); err != nil {
		return false, fmt.Errorf("removing budget placement binding: %w", err)
	}
	if err := m.client.DeleteIfExists(ctx, client.GVRPolicy, ns, policyName); err != nil {
		return false, fmt.Errorf("removing budget policy: %w", err)
	}
	if err := m.client.DeleteIfExists(ctx, client.GVRPlacement, ns, policyName+"-placement"); err != nil {
		return false, fmt.Errorf("removing budget placement: %w", err)
	}

	return true, nil
}
