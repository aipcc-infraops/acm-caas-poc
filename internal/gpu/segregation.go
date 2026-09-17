package gpu

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/types"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type VersionCluster struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Channel   string `json:"channel"`
	Build     string `json:"build,omitempty"`
	Available bool   `json:"available"`
}

func (m *Manager) RouteByVersion(ctx context.Context, version string) (string, error) {
	m.logger.Info("gpu.RouteByVersion", "version", version)

	selector := fmt.Sprintf("ai-platform-version=%s,gpu-available=true", version)
	list, err := m.client.List(ctx, client.GVRManagedCluster, "", selector)
	if err != nil {
		return "", fmt.Errorf("listing clusters for version %s: %w", version, err)
	}

	if len(list.Items) == 0 {
		return "", fmt.Errorf("no available cluster with ai-platform-version=%s", version)
	}

	return list.Items[0].GetName(), nil
}

func (m *Manager) EnforceVersionPolicy(ctx context.Context, cluster, version string) error {
	m.logger.Info("gpu.EnforceVersionPolicy", "cluster", cluster, "version", version)

	versionLabel := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				"ai-platform-version": version,
			},
		},
	}
	patch, err := json.Marshal(versionLabel)
	if err != nil {
		return fmt.Errorf("marshalling version label: %w", err)
	}
	if _, err := m.client.Patch(ctx, client.GVRManagedCluster, "", cluster, types.MergePatchType, patch); err != nil {
		return fmt.Errorf("labelling cluster %s with version %s: %w", cluster, version, err)
	}

	configPolicy := buildVersionEnforcementPolicy(cluster, version)
	if err := m.client.CreateIfNotExists(ctx, client.GVRConfigurationPolicy, DefaultNamespace, configPolicy); err != nil {
		return fmt.Errorf("creating version enforcement config policy: %w", err)
	}

	policy := buildVersionPolicyWrapper(cluster)
	if err := m.client.CreateIfNotExists(ctx, client.GVRPolicy, DefaultNamespace, policy); err != nil {
		return fmt.Errorf("creating version enforcement policy: %w", err)
	}

	placement := buildVersionPlacement(cluster)
	if err := m.client.CreateIfNotExists(ctx, client.GVRPlacement, DefaultNamespace, placement); err != nil {
		return fmt.Errorf("creating version enforcement placement: %w", err)
	}

	binding := buildVersionPlacementBinding(cluster)
	if err := m.client.CreateIfNotExists(ctx, client.GVRPlacementBinding, DefaultNamespace, binding); err != nil {
		return fmt.Errorf("creating version enforcement placement binding: %w", err)
	}

	return nil
}

func (m *Manager) RemoveVersionPolicy(ctx context.Context, cluster string) error {
	m.logger.Info("gpu.RemoveVersionPolicy", "cluster", cluster)

	prefix := versionPolicyName(cluster)
	_ = m.client.DeleteIfExists(ctx, client.GVRPlacementBinding, DefaultNamespace, prefix+"-binding")
	_ = m.client.DeleteIfExists(ctx, client.GVRPlacement, DefaultNamespace, prefix+"-placement")
	_ = m.client.DeleteIfExists(ctx, client.GVRPolicy, DefaultNamespace, prefix)
	_ = m.client.DeleteIfExists(ctx, client.GVRConfigurationPolicy, DefaultNamespace, prefix+"-config")

	return nil
}

func (m *Manager) ProvisionForVersion(ctx context.Context, cluster, version string) error {
	m.logger.Info("gpu.ProvisionForVersion", "cluster", cluster, "version", version)

	labels := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				"ai-platform-version": version,
				"ai-platform-channel": "stable",
				"gpu-available":       "true",
			},
		},
	}
	patch, err := json.Marshal(labels)
	if err != nil {
		return fmt.Errorf("marshalling label patch: %w", err)
	}
	if _, err := m.client.Patch(ctx, client.GVRManagedCluster, "", cluster, types.MergePatchType, patch); err != nil {
		return fmt.Errorf("labelling cluster %s with version %s: %w", cluster, version, err)
	}

	mw := buildOperatorManifestWork(cluster, version)
	if err := m.client.CreateIfNotExists(ctx, client.GVRManifestWork, cluster, mw); err != nil {
		return fmt.Errorf("creating operator manifest work: %w", err)
	}

	return nil
}

func (m *Manager) ListVersionClusters(ctx context.Context) ([]VersionCluster, error) {
	m.logger.Info("gpu.ListVersionClusters")

	list, err := m.client.List(ctx, client.GVRManagedCluster, "", "ai-platform-version")
	if err != nil {
		return nil, fmt.Errorf("listing version clusters: %w", err)
	}

	var clusters []VersionCluster
	for _, item := range list.Items {
		labels := item.GetLabels()
		if labels == nil {
			continue
		}
		if _, ok := labels["ai-platform-version"]; !ok {
			continue
		}
		clusters = append(clusters, parseVersionCluster(item.Object))
	}

	return clusters, nil
}
