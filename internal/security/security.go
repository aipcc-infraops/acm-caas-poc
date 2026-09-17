package security

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

const DefaultNamespace = "open-cluster-management-policies"

type BaselineStatus struct {
	Cluster    string `json:"cluster"`
	Level      string `json:"level"`
	Applied    bool   `json:"applied"`
	Conditions []string `json:"conditions,omitempty"`
}

type BaselineInfo struct {
	Cluster string `json:"cluster"`
	Level   string `json:"level"`
	Status  string `json:"status"`
}

type Manager struct {
	client *client.Client
	cfg    config.Config
	logger *slog.Logger
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger}
}

var validLevels = map[string]bool{
	"privileged": true, "baseline": true, "restricted": true,
	"cis-level1": true, "cis-level2": true, "cis-level3": true,
}

func (m *Manager) ApplyBaseline(ctx context.Context, cluster, level, clusterSet string) error {
	m.logger.Info("security.ApplyBaseline", "cluster", cluster, "level", level)
	if level == "" {
		level = "cis-level1"
	}
	if !validLevels[level] {
		return fmt.Errorf("invalid security level %q (valid: privileged, baseline, restricted, cis-level1, cis-level2, cis-level3)", level)
	}

	mw := buildGatekeeperManifestWork(cluster, level)
	if err := m.client.CreateIfNotExists(ctx, client.GVRManifestWork, cluster, mw); err != nil {
		return fmt.Errorf("creating security baseline ManifestWork: %w", err)
	}

	healthPolicy := buildGatekeeperHealthPolicy(cluster, clusterSet)
	if err := m.client.CreateIfNotExists(ctx, client.GVRPolicy, DefaultNamespace, healthPolicy); err != nil {
		return fmt.Errorf("creating Gatekeeper health policy: %w", err)
	}

	placement := buildHealthPlacement(cluster, clusterSet)
	if err := m.client.CreateIfNotExists(ctx, client.GVRPlacement, DefaultNamespace, placement); err != nil {
		return fmt.Errorf("creating health placement: %w", err)
	}

	binding := buildHealthPlacementBinding(cluster)
	if err := m.client.CreateIfNotExists(ctx, client.GVRPlacementBinding, DefaultNamespace, binding); err != nil {
		return fmt.Errorf("creating health placement binding: %w", err)
	}

	return nil
}

func (m *Manager) GetStatus(ctx context.Context, cluster string) (*BaselineStatus, error) {
	m.logger.Info("security.GetStatus", "cluster", cluster)
	name := manifestWorkName(cluster)
	obj, err := m.client.Get(ctx, client.GVRManifestWork, cluster, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return &BaselineStatus{Cluster: cluster, Level: "none", Applied: false}, nil
		}
		return nil, fmt.Errorf("getting security baseline for %s: %w", cluster, err)
	}
	return parseBaselineStatus(cluster, obj.Object), nil
}

func (m *Manager) ListBaselines(ctx context.Context) ([]BaselineInfo, error) {
	m.logger.Info("security.ListBaselines")
	list, err := m.client.List(ctx, client.GVRManifestWork, "", "acmlab.redhat.com/security-baseline")
	if err != nil {
		return nil, fmt.Errorf("listing security baselines: %w", err)
	}
	infos := make([]BaselineInfo, 0, len(list.Items))
	for _, item := range list.Items {
		infos = append(infos, parseBaselineInfo(item.Object))
	}
	return infos, nil
}

func (m *Manager) RemoveBaseline(ctx context.Context, cluster string) (bool, error) {
	m.logger.Info("security.RemoveBaseline", "cluster", cluster)
	name := manifestWorkName(cluster)
	_, err := m.client.Get(ctx, client.GVRManifestWork, cluster, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking baseline %s: %w", name, err)
	}

	if err := m.client.DeleteIfExists(ctx, client.GVRManifestWork, cluster, name); err != nil {
		return false, fmt.Errorf("removing baseline ManifestWork: %w", err)
	}

	policyName := healthPolicyName(cluster)
	_ = m.client.DeleteIfExists(ctx, client.GVRPlacementBinding, DefaultNamespace, policyName+"-placement-binding")
	_ = m.client.DeleteIfExists(ctx, client.GVRPlacement, DefaultNamespace, policyName+"-placement")
	_ = m.client.DeleteIfExists(ctx, client.GVRPolicy, DefaultNamespace, policyName)

	return true, nil
}

type CustomPolicyOpts struct {
	Name       string
	Cluster    string
	RegoFile   string
	RegoInline string
	Match      []string
}

func (m *Manager) ApplyCustomPolicy(ctx context.Context, opts CustomPolicyOpts) error {
	m.logger.Info("security.ApplyCustomPolicy", "name", opts.Name, "cluster", opts.Cluster)

	rego, err := resolveRego(opts)
	if err != nil {
		return err
	}

	pkg := extractPackageName(rego)
	if pkg == "" {
		return fmt.Errorf("could not extract package name from Rego source")
	}

	if opts.Name == "" {
		opts.Name = pkg
	}

	matchKinds := opts.Match
	if len(matchKinds) == 0 {
		matchKinds = []string{"Pod"}
	}

	mw := buildCustomRegoManifestWork(opts.Cluster, opts.Name, pkg, rego, matchKinds)
	if err := m.client.CreateIfNotExists(ctx, client.GVRManifestWork, opts.Cluster, mw); err != nil {
		return fmt.Errorf("creating custom policy ManifestWork: %w", err)
	}
	return nil
}

func (m *Manager) RemoveCustomPolicy(ctx context.Context, name, cluster string) error {
	m.logger.Info("security.RemoveCustomPolicy", "name", name, "cluster", cluster)
	mwName := "custom-rego-" + name + "-" + cluster
	_, err := m.client.Get(ctx, client.GVRManifestWork, cluster, mwName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("custom policy %s not found on cluster %s", name, cluster)
		}
		return fmt.Errorf("checking custom policy: %w", err)
	}
	return m.client.DeleteIfExists(ctx, client.GVRManifestWork, cluster, mwName)
}

func (m *Manager) ListCustomPolicies(ctx context.Context) ([]BaselineInfo, error) {
	m.logger.Info("security.ListCustomPolicies")
	list, err := m.client.List(ctx, client.GVRManifestWork, "", "acmlab.redhat.com/custom-rego")
	if err != nil {
		return nil, fmt.Errorf("listing custom policies: %w", err)
	}
	infos := make([]BaselineInfo, 0, len(list.Items))
	for _, item := range list.Items {
		infos = append(infos, parseBaselineInfo(item.Object))
	}
	return infos, nil
}

func resolveRego(opts CustomPolicyOpts) (string, error) {
	if opts.RegoFile != "" {
		data, err := os.ReadFile(opts.RegoFile)
		if err != nil {
			return "", fmt.Errorf("reading rego file %s: %w", opts.RegoFile, err)
		}
		return string(data), nil
	}
	if opts.RegoInline != "" {
		return opts.RegoInline, nil
	}
	return "", fmt.Errorf("either RegoFile or RegoInline must be provided")
}

func extractPackageName(rego string) string {
	for _, line := range strings.Split(rego, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "package ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "package "))
		}
	}
	return ""
}

func manifestWorkName(cluster string) string {
	return "security-baseline-" + cluster
}

func healthPolicyName(cluster string) string {
	return "gatekeeper-health-" + cluster
}

func parseBaselineStatus(cluster string, obj map[string]interface{}) *BaselineStatus {
	bs := &BaselineStatus{Cluster: cluster}

	if labels, ok := nestedMap(obj, "metadata", "labels"); ok {
		bs.Level, _ = labels["acmlab.redhat.com/security-level"].(string)
	}

	status, _ := obj["status"].(map[string]interface{})
	if status == nil {
		return bs
	}

	conditions, _ := status["conditions"].([]interface{})
	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := cond["type"].(string)
		condStatus, _ := cond["status"].(string)
		bs.Conditions = append(bs.Conditions, fmt.Sprintf("%s=%s", condType, condStatus))
		if condType == "Applied" && condStatus == "True" {
			bs.Applied = true
		}
	}

	return bs
}

func parseBaselineInfo(obj map[string]interface{}) BaselineInfo {
	info := BaselineInfo{}
	if meta, ok := obj["metadata"].(map[string]interface{}); ok {
		info.Cluster, _ = meta["namespace"].(string)
		if labels, ok := meta["labels"].(map[string]interface{}); ok {
			info.Level, _ = labels["acmlab.redhat.com/security-level"].(string)
		}
	}

	info.Status = "Pending"
	status, _ := obj["status"].(map[string]interface{})
	if status == nil {
		return info
	}
	conditions, _ := status["conditions"].([]interface{})
	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] == "Applied" && cond["status"] == "True" {
			info.Status = "Applied"
		}
	}
	return info
}

func nestedMap(obj map[string]interface{}, keys ...string) (map[string]interface{}, bool) {
	current := obj
	for _, k := range keys {
		next, ok := current[k].(map[string]interface{})
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}
