package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

const (
	DefaultNamespace = "open-cluster-management-global-set"
)

type ComplianceInfo struct {
	ClusterName      string
	ComplianceState  string
}

type PolicyInfo struct {
	Name              string
	Namespace         string
	RemediationAction string
	Disabled          bool
	Compliant         string
	ClusterCompliance []ComplianceInfo
}

type Manager struct {
	client *client.Client
	cfg    config.Config
	logger *slog.Logger
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger}
}

func (m *Manager) Apply(ctx context.Context, opts PolicyOpts) error {
	m.logger.Info("policy.Apply", "policy", opts.Name)
	ns := opts.Namespace
	if ns == "" {
		ns = DefaultNamespace
	}

	steps := []struct {
		name string
		fn   func() error
	}{
		{"placement", func() error { return m.ensurePlacement(ctx, ns, opts) }},
		{"policy", func() error { return m.ensurePolicy(ctx, ns, opts) }},
		{"placement-binding", func() error { return m.ensurePlacementBinding(ctx, ns, opts) }},
	}
	for _, s := range steps {
		if err := s.fn(); err != nil {
			return fmt.Errorf("apply %s: %w", s.name, err)
		}
	}
	return nil
}

func (m *Manager) Remove(ctx context.Context, name, namespace string) (bool, error) {
	m.logger.Info("policy.Remove", "policy", name)
	if namespace == "" {
		namespace = DefaultNamespace
	}

	_, err := m.client.Get(ctx, client.GVRPolicy, namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking policy %s: %w", name, err)
	}

	steps := []struct {
		label string
		gvr   schema.GroupVersionResource
		name  string
	}{
		{"placement-binding", client.GVRPlacementBinding, name + "-placement-binding"},
		{"policy", client.GVRPolicy, name},
		{"placement", client.GVRPlacement, name + "-placement"},
	}
	for _, s := range steps {
		if err := m.client.DeleteIfExists(ctx, s.gvr, namespace, s.name); err != nil {
			return false, fmt.Errorf("remove %s: %w", s.label, err)
		}
	}
	return true, nil
}

func (m *Manager) List(ctx context.Context, namespace string) ([]PolicyInfo, error) {
	m.logger.Info("policy.List")
	if namespace == "" {
		namespace = DefaultNamespace
	}
	list, err := m.client.List(ctx, client.GVRPolicy, namespace, "")
	if err != nil {
		return nil, fmt.Errorf("listing policies: %w", err)
	}
	policies := make([]PolicyInfo, 0, len(list.Items))
	for _, item := range list.Items {
		policies = append(policies, parsePolicyInfo(item.Object))
	}
	return policies, nil
}

func (m *Manager) Get(ctx context.Context, name, namespace string) (*PolicyInfo, error) {
	m.logger.Info("policy.Get", "policy", name)
	if namespace == "" {
		namespace = DefaultNamespace
	}
	obj, err := m.client.Get(ctx, client.GVRPolicy, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("getting policy %s: %w", name, err)
	}
	info := parsePolicyInfo(obj.Object)
	return &info, nil
}

func (m *Manager) SetRemediation(ctx context.Context, name, namespace, action string) error {
	m.logger.Info("policy.SetRemediation", "policy", name, "action", action)
	if namespace == "" {
		namespace = DefaultNamespace
	}
	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"remediationAction": action,
		},
	}
	data, _ := json.Marshal(patch)
	_, err := m.client.Patch(ctx, client.GVRPolicy, namespace, name, types.MergePatchType, data)
	if err != nil {
		return fmt.Errorf("patching policy %s remediation to %s: %w", name, action, err)
	}
	return nil
}

func (m *Manager) SetDisabled(ctx context.Context, name, namespace string, disabled bool) error {
	m.logger.Info("policy.SetDisabled", "policy", name, "disabled", disabled)
	if namespace == "" {
		namespace = DefaultNamespace
	}
	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"disabled": disabled,
		},
	}
	data, _ := json.Marshal(patch)
	_, err := m.client.Patch(ctx, client.GVRPolicy, namespace, name, types.MergePatchType, data)
	if err != nil {
		return fmt.Errorf("patching policy %s disabled=%v: %w", name, disabled, err)
	}
	return nil
}

type QuotaStatus struct {
	Cluster    string `json:"cluster"`
	MaxWorkers int    `json:"maxWorkers"`
	MaxGPUs    int    `json:"maxGpus"`
	PolicyName string `json:"policyName"`
	Compliant  string `json:"compliant"`
}

func (m *Manager) ApplyQuotaPolicy(ctx context.Context, cluster string, maxWorkers, maxGPUs int) error {
	m.logger.Info("policy.ApplyQuotaPolicy", "cluster", cluster, "maxWorkers", maxWorkers, "maxGPUs", maxGPUs)

	if err := m.stampQuotaLabels(ctx, cluster, maxWorkers, maxGPUs); err != nil {
		return fmt.Errorf("stamping quota labels on %s: %w", cluster, err)
	}

	policyName := fmt.Sprintf("quota-%s", cluster)
	opts := PolicyOpts{
		Name:              policyName,
		RemediationAction: "inform",
		MaxWorkers:        maxWorkers,
		MaxGPUs:           maxGPUs,
		ClusterLabels:     map[string]string{"name": cluster},
	}
	return m.Apply(ctx, opts)
}

func (m *Manager) stampQuotaLabels(ctx context.Context, cluster string, maxWorkers, maxGPUs int) error {
	labels := map[string]interface{}{}
	if maxWorkers > 0 {
		labels["caas/max-workers"] = fmt.Sprintf("%d", maxWorkers)
	}
	if maxGPUs > 0 {
		labels["caas/max-gpus"] = fmt.Sprintf("%d", maxGPUs)
	}

	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": labels,
		},
	}
	data, _ := json.Marshal(patch)
	_, err := m.client.Patch(ctx, client.GVRManagedCluster, "", cluster, types.MergePatchType, data)
	return err
}

func (m *Manager) GetQuotaStatus(ctx context.Context, cluster string) (*QuotaStatus, error) {
	m.logger.Info("policy.GetQuotaStatus", "cluster", cluster)

	mc, err := m.client.Get(ctx, client.GVRManagedCluster, "", cluster)
	if err != nil {
		return nil, fmt.Errorf("getting ManagedCluster %s: %w", cluster, err)
	}

	labels := mc.GetLabels()
	status := &QuotaStatus{
		Cluster:    cluster,
		PolicyName: fmt.Sprintf("quota-%s", cluster),
	}

	if v, ok := labels["caas/max-workers"]; ok {
		fmt.Sscanf(v, "%d", &status.MaxWorkers)
	}
	if v, ok := labels["caas/max-gpus"]; ok {
		fmt.Sscanf(v, "%d", &status.MaxGPUs)
	}

	policyInfo, err := m.Get(ctx, status.PolicyName, "")
	if err != nil {
		status.Compliant = "NoPolicyFound"
		return status, nil
	}
	status.Compliant = policyInfo.Compliant
	if status.Compliant == "" {
		status.Compliant = "Pending"
	}

	return status, nil
}

type ClusterSetCompliance struct {
	ClusterSet   string `json:"clusterSet"`
	Total        int    `json:"total"`
	Compliant    int    `json:"compliant"`
	NonCompliant int    `json:"nonCompliant"`
	Pending      int    `json:"pending"`
}

func (m *Manager) ComplianceReport(ctx context.Context, namespace string) ([]ClusterSetCompliance, error) {
	m.logger.Info("policy.ComplianceReport")
	if namespace == "" {
		namespace = DefaultNamespace
	}

	policies, err := m.List(ctx, namespace)
	if err != nil {
		return nil, err
	}

	clusters, err := m.client.List(ctx, client.GVRManagedCluster, "", "")
	if err != nil {
		return nil, fmt.Errorf("listing ManagedClusters: %w", err)
	}

	clusterToSet := map[string]string{}
	for _, mc := range clusters.Items {
		labels := mc.GetLabels()
		if labels == nil {
			continue
		}
		if setName, ok := labels["cluster.open-cluster-management.io/clusterset"]; ok {
			clusterToSet[mc.GetName()] = setName
		}
	}

	setStats := map[string]*ClusterSetCompliance{}
	for _, pol := range policies {
		for _, cc := range pol.ClusterCompliance {
			setName := clusterToSet[cc.ClusterName]
			if setName == "" {
				setName = "default"
			}
			stats, ok := setStats[setName]
			if !ok {
				stats = &ClusterSetCompliance{ClusterSet: setName}
				setStats[setName] = stats
			}
			stats.Total++
			switch cc.ComplianceState {
			case "Compliant":
				stats.Compliant++
			case "NonCompliant":
				stats.NonCompliant++
			default:
				stats.Pending++
			}
		}
	}

	result := make([]ClusterSetCompliance, 0, len(setStats))
	for _, s := range setStats {
		result = append(result, *s)
	}
	return result, nil
}

func parsePolicyInfo(obj map[string]interface{}) PolicyInfo {
	info := PolicyInfo{}

	if meta, ok := obj["metadata"].(map[string]interface{}); ok {
		info.Name, _ = meta["name"].(string)
		info.Namespace, _ = meta["namespace"].(string)
	}

	spec, _ := obj["spec"].(map[string]interface{})
	if spec != nil {
		info.RemediationAction, _ = spec["remediationAction"].(string)
		info.Disabled, _ = spec["disabled"].(bool)
	}

	status, _ := obj["status"].(map[string]interface{})
	if status != nil {
		info.Compliant, _ = status["compliant"].(string)
		if cds, ok := status["status"].([]interface{}); ok {
			for _, raw := range cds {
				cs, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				ci := ComplianceInfo{}
				ci.ClusterName, _ = cs["clustername"].(string)
				ci.ComplianceState, _ = cs["compliant"].(string)
				info.ClusterCompliance = append(info.ClusterCompliance, ci)
			}
		}
		if info.Compliant == "" && len(info.ClusterCompliance) > 0 {
			info.Compliant = aggregateCompliance(info.ClusterCompliance)
		}
	}

	return info
}

func aggregateCompliance(clusters []ComplianceInfo) string {
	for _, c := range clusters {
		if c.ComplianceState == "NonCompliant" {
			return "NonCompliant"
		}
	}
	for _, c := range clusters {
		if c.ComplianceState == "Compliant" {
			return "Compliant"
		}
	}
	return "Pending"
}
