package gpu

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/types"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type PlacementResult struct {
	ClusterName string `json:"clusterName"`
	Score       int    `json:"score,omitempty"`
}

func (m *Manager) CreateGPUPlacement(ctx context.Context, name, gpuType, region string) error {
	m.logger.Info("gpu.CreateGPUPlacement", "name", name, "gpuType", gpuType, "region", region)

	placement := buildGPUPlacement(name, gpuType, region)
	return m.client.CreateIfNotExists(ctx, client.GVRPlacement, DefaultNamespace, placement)
}

func (m *Manager) GetPlacementDecision(ctx context.Context, placementName string) ([]PlacementResult, error) {
	m.logger.Info("gpu.GetPlacementDecision", "placement", placementName)

	selector := fmt.Sprintf("cluster.open-cluster-management.io/placement=%s", placementName)
	list, err := m.client.List(ctx, client.GVRPlacementDecision, DefaultNamespace, selector)
	if err != nil {
		return nil, fmt.Errorf("listing placement decisions for %s: %w", placementName, err)
	}

	return parsePlacementDecisions(list), nil
}

func (m *Manager) MarkSaturated(ctx context.Context, cluster string, saturated bool) error {
	m.logger.Info("gpu.MarkSaturated", "cluster", cluster, "saturated", saturated)

	value := "true"
	if saturated {
		value = "false"
	}

	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				"gpu-available": value,
			},
		},
	}
	data, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshalling saturated patch: %w", err)
	}

	_, err = m.client.Patch(ctx, client.GVRManagedCluster, "", cluster, types.MergePatchType, data)
	if err != nil {
		return fmt.Errorf("patching cluster %s gpu-available label: %w", cluster, err)
	}
	return nil
}

func (m *Manager) BestCluster(ctx context.Context, gpuType string) (string, error) {
	m.logger.Info("gpu.BestCluster", "gpuType", gpuType)

	tempName := fmt.Sprintf("gpu-best-%s-tmp", gpuType)

	if err := m.CreateGPUPlacement(ctx, tempName, gpuType, ""); err != nil {
		return "", fmt.Errorf("creating temporary placement: %w", err)
	}
	defer func() {
		_ = m.client.DeleteIfExists(ctx, client.GVRPlacement, DefaultNamespace, tempName)
	}()

	results, err := m.GetPlacementDecision(ctx, tempName)
	if err != nil {
		return "", fmt.Errorf("reading placement decision: %w", err)
	}
	if len(results) == 0 {
		return "", fmt.Errorf("no GPU cluster available for type %s", gpuType)
	}

	return results[0].ClusterName, nil
}
