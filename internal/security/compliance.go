package security

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type ComplianceScanOpts struct {
	Cluster    string
	Profile    string
	ClusterSet string
}

type ComplianceScanStatus struct {
	Cluster      string   `json:"cluster"`
	Profile      string   `json:"profile"`
	Phase        string   `json:"phase"`
	Compliant    int      `json:"compliant"`
	NonCompliant int      `json:"nonCompliant"`
	Conditions   []string `json:"conditions,omitempty"`
}

type ComplianceCheckResult struct {
	Rule     string `json:"rule"`
	Status   string `json:"status"`
	Severity string `json:"severity"`
	Detail   string `json:"detail,omitempty"`
}

type ComplianceReport struct {
	Cluster string                  `json:"cluster"`
	Profile string                  `json:"profile"`
	Results []ComplianceCheckResult `json:"results"`
	Summary ComplianceScanStatus    `json:"summary"`
}

func (m *Manager) DeployComplianceOperator(ctx context.Context, cluster, clusterSet string) error {
	m.logger.Info("security.DeployComplianceOperator", "cluster", cluster)

	opPolicy := buildComplianceOperatorPolicy(cluster)
	if err := m.client.CreateIfNotExists(ctx, client.GVROperatorPolicy, DefaultNamespace, opPolicy); err != nil {
		return fmt.Errorf("creating compliance operator policy: %w", err)
	}

	policy := buildComplianceOperatorHealthPolicy(cluster)
	if err := m.client.CreateIfNotExists(ctx, client.GVRPolicy, DefaultNamespace, policy); err != nil {
		return fmt.Errorf("creating compliance operator health policy: %w", err)
	}

	placement := buildCompliancePlacement(cluster, clusterSet)
	if err := m.client.CreateIfNotExists(ctx, client.GVRPlacement, DefaultNamespace, placement); err != nil {
		return fmt.Errorf("creating compliance placement: %w", err)
	}

	binding := buildCompliancePlacementBinding(cluster)
	if err := m.client.CreateIfNotExists(ctx, client.GVRPlacementBinding, DefaultNamespace, binding); err != nil {
		return fmt.Errorf("creating compliance placement binding: %w", err)
	}

	return nil
}

func (m *Manager) CreateComplianceScan(ctx context.Context, opts ComplianceScanOpts) error {
	m.logger.Info("security.CreateComplianceScan", "cluster", opts.Cluster, "profile", opts.Profile)
	if opts.Profile == "" {
		opts.Profile = "ocp4-cis"
	}

	scanPolicy := buildComplianceScanPolicy(opts)
	if err := m.client.CreateIfNotExists(ctx, client.GVRPolicy, DefaultNamespace, scanPolicy); err != nil {
		return fmt.Errorf("creating compliance scan policy: %w", err)
	}

	placement := buildComplianceScanPlacement(opts.Cluster, opts.ClusterSet)
	if err := m.client.CreateIfNotExists(ctx, client.GVRPlacement, DefaultNamespace, placement); err != nil {
		return fmt.Errorf("creating compliance scan placement: %w", err)
	}

	binding := buildComplianceScanPlacementBinding(opts.Cluster)
	if err := m.client.CreateIfNotExists(ctx, client.GVRPlacementBinding, DefaultNamespace, binding); err != nil {
		return fmt.Errorf("creating compliance scan placement binding: %w", err)
	}

	return nil
}

func (m *Manager) GetComplianceStatus(ctx context.Context, cluster string) (*ComplianceScanStatus, error) {
	m.logger.Info("security.GetComplianceStatus", "cluster", cluster)

	status := &ComplianceScanStatus{
		Cluster: cluster,
		Phase:   "Pending",
	}

	opName := complianceOperatorPolicyName(cluster)
	opObj, err := m.client.Get(ctx, client.GVROperatorPolicy, DefaultNamespace, opName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			status.Phase = "NotDeployed"
			return status, nil
		}
		return nil, fmt.Errorf("getting compliance operator policy: %w", err)
	}

	opConditions := extractPolicyConditions(opObj.Object)
	status.Conditions = append(status.Conditions, opConditions...)

	scanName := complianceScanPolicyName(cluster)
	scanObj, err := m.client.Get(ctx, client.GVRPolicy, DefaultNamespace, scanName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			status.Phase = "OperatorDeployed"
			return status, nil
		}
		return nil, fmt.Errorf("getting compliance scan policy: %w", err)
	}

	scanStatus := parseComplianceScanStatus(cluster, scanObj.Object)
	status.Phase = scanStatus.Phase
	status.Profile = scanStatus.Profile
	status.Compliant = scanStatus.Compliant
	status.NonCompliant = scanStatus.NonCompliant
	status.Conditions = append(status.Conditions, scanStatus.Conditions...)

	return status, nil
}

func (m *Manager) GetComplianceReport(ctx context.Context, cluster, profile string) (*ComplianceReport, error) {
	m.logger.Info("security.GetComplianceReport", "cluster", cluster, "profile", profile)

	status, err := m.GetComplianceStatus(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("getting compliance status: %w", err)
	}

	report := &ComplianceReport{
		Cluster: cluster,
		Profile: profile,
		Summary: *status,
		Results: make([]ComplianceCheckResult, 0),
	}

	scanName := complianceScanPolicyName(cluster)
	scanObj, err := m.client.Get(ctx, client.GVRPolicy, DefaultNamespace, scanName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return report, nil
		}
		return nil, fmt.Errorf("getting compliance scan policy: %w", err)
	}

	report.Results = extractComplianceResults(scanObj.Object)
	return report, nil
}

func (m *Manager) RemoveComplianceScan(ctx context.Context, cluster string) error {
	m.logger.Info("security.RemoveComplianceScan", "cluster", cluster)

	scanName := complianceScanPolicyName(cluster)
	_ = m.client.DeleteIfExists(ctx, client.GVRPlacementBinding, DefaultNamespace, scanName+"-placement-binding")
	_ = m.client.DeleteIfExists(ctx, client.GVRPlacement, DefaultNamespace, scanName+"-placement")
	_ = m.client.DeleteIfExists(ctx, client.GVRPolicy, DefaultNamespace, scanName)

	opName := complianceOperatorPolicyName(cluster)
	_ = m.client.DeleteIfExists(ctx, client.GVRPlacementBinding, DefaultNamespace, opName+"-placement-binding")
	_ = m.client.DeleteIfExists(ctx, client.GVRPlacement, DefaultNamespace, opName+"-placement")
	_ = m.client.DeleteIfExists(ctx, client.GVRPolicy, DefaultNamespace, opName+"-health")
	_ = m.client.DeleteIfExists(ctx, client.GVROperatorPolicy, DefaultNamespace, opName)

	return nil
}

func complianceOperatorPolicyName(cluster string) string {
	return "compliance-operator-" + cluster
}

func complianceScanPolicyName(cluster string) string {
	return "compliance-scan-" + cluster
}

func extractPolicyConditions(obj map[string]interface{}) []string {
	var conditions []string
	status, _ := obj["status"].(map[string]interface{})
	if status == nil {
		return conditions
	}
	condList, _ := status["conditions"].([]interface{})
	for _, raw := range condList {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := cond["type"].(string)
		condStatus, _ := cond["status"].(string)
		conditions = append(conditions, fmt.Sprintf("%s=%s", condType, condStatus))
	}
	return conditions
}

func extractComplianceResults(obj map[string]interface{}) []ComplianceCheckResult {
	var results []ComplianceCheckResult
	status, _ := obj["status"].(map[string]interface{})
	if status == nil {
		return results
	}

	details, _ := status["details"].([]interface{})
	for _, raw := range details {
		detail, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		result := ComplianceCheckResult{
			Rule:     stringVal(detail, "rule"),
			Status:   stringVal(detail, "status"),
			Severity: stringVal(detail, "severity"),
			Detail:   stringVal(detail, "detail"),
		}
		if result.Rule != "" {
			results = append(results, result)
		}
	}
	return results
}

func stringVal(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}
