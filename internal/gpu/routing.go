package gpu

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/types"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

var (
	ErrGPUPending = errors.New("placement decision pending reconciliation")
	ErrGPUNoMatch = errors.New("no GPU cluster matches the requested type")
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
	return m.BestClusterWithTimeout(ctx, gpuType, 30*time.Second)
}

func (m *Manager) BestClusterWithTimeout(ctx context.Context, gpuType string, timeout time.Duration) (string, error) {
	m.logger.Info("gpu.BestCluster", "gpuType", gpuType)

	suffix, err := randomSuffix()
	if err != nil {
		return "", fmt.Errorf("generating unique placement name: %w", err)
	}
	tempName := fmt.Sprintf("gpu-best-%s-%s", gpuType, suffix)

	opCtx, opCancel := context.WithTimeout(ctx, timeout)
	defer opCancel()

	if err := m.CreateGPUPlacement(opCtx, tempName, gpuType, ""); err != nil {
		return "", fmt.Errorf("creating temporary placement: %w", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = m.client.DeleteIfExists(cleanupCtx, client.GVRPlacement, DefaultNamespace, tempName)
	}()

	interval := 500 * time.Millisecond
	for {
		results, hasObj, err := m.queryPlacementDecision(opCtx, tempName)
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			if opCtx.Err() != nil {
				return "", ErrGPUPending
			}
			return "", fmt.Errorf("reading placement decision: %w", err)
		}
		if len(results) > 0 {
			return results[0].ClusterName, nil
		}
		if hasObj {
			return "", ErrGPUNoMatch
		}

		select {
		case <-opCtx.Done():
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "", ErrGPUPending
		case <-time.After(interval):
			if interval < 4*time.Second {
				interval *= 2
			}
		}
	}
}

func (m *Manager) queryPlacementDecision(ctx context.Context, placementName string) ([]PlacementResult, bool, error) {
	selector := fmt.Sprintf("cluster.open-cluster-management.io/placement=%s", placementName)
	list, err := m.client.List(ctx, client.GVRPlacementDecision, DefaultNamespace, selector)
	if err != nil {
		return nil, false, err
	}
	hasObj := len(list.Items) > 0
	results := parsePlacementDecisions(list)
	return results, hasObj, nil
}

func randomSuffix() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
