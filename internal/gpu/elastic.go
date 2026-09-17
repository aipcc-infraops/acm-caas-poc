package gpu

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/types"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type SaturationStatus struct {
	Cluster     string  `json:"cluster"`
	GPUType     string  `json:"gpuType"`
	Utilization float64 `json:"utilization"`
	Saturated   bool    `json:"saturated"`
	Threshold   float64 `json:"threshold"`
}

type ElasticCluster struct {
	Name      string `json:"name"`
	GPUType   string `json:"gpuType"`
	CostTier  string `json:"costTier"`
	Available bool   `json:"available"`
}

type OnDemandOpts struct {
	Cluster string
	GPUType string
}

func (m *Manager) DetectSaturation(ctx context.Context, cluster string, threshold float64) (*SaturationStatus, error) {
	m.logger.Info("gpu.DetectSaturation", "cluster", cluster, "threshold", threshold)

	mc, err := m.client.Get(ctx, client.GVRManagedCluster, "", cluster)
	if err != nil {
		return nil, fmt.Errorf("getting ManagedCluster %s: %w", cluster, err)
	}

	labels := mc.GetLabels()
	gpuType := labels["gpu-type"]

	var utilization float64
	if raw, ok := labels["gpu-utilization"]; ok {
		utilization, _ = strconv.ParseFloat(raw, 64)
	}

	saturated := utilization >= threshold
	if labels["gpu-available"] == "false" {
		saturated = true
	}

	return &SaturationStatus{
		Cluster:     cluster,
		GPUType:     gpuType,
		Utilization: utilization,
		Saturated:   saturated,
		Threshold:   threshold,
	}, nil
}

func (m *Manager) ProvisionOnDemand(ctx context.Context, opts OnDemandOpts) error {
	m.logger.Info("gpu.ProvisionOnDemand", "cluster", opts.Cluster, "gpuType", opts.GPUType)

	patch := buildElasticLabels(opts.GPUType, "on-demand")
	data, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshalling elastic labels: %w", err)
	}

	if _, err := m.client.Patch(ctx, client.GVRManagedCluster, "", opts.Cluster, types.MergePatchType, data); err != nil {
		return fmt.Errorf("patching ManagedCluster %s with elastic labels: %w", opts.Cluster, err)
	}

	mw := buildElasticManifestWork(opts.Cluster, opts.GPUType)
	if err := m.client.CreateIfNotExists(ctx, client.GVRManifestWork, opts.Cluster, mw); err != nil {
		return fmt.Errorf("creating elastic ManifestWork for %s: %w", opts.Cluster, err)
	}

	return nil
}

func (m *Manager) HibernateIdle(ctx context.Context, cluster string) error {
	m.logger.Info("gpu.HibernateIdle", "cluster", cluster)

	patch := buildHibernateLabels()
	data, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshalling hibernate labels: %w", err)
	}

	if _, err := m.client.Patch(ctx, client.GVRManagedCluster, "", cluster, types.MergePatchType, data); err != nil {
		return fmt.Errorf("patching ManagedCluster %s with hibernate labels: %w", cluster, err)
	}

	return nil
}

func (m *Manager) ListElasticClusters(ctx context.Context) ([]ElasticCluster, error) {
	m.logger.Info("gpu.ListElasticClusters")

	list, err := m.client.List(ctx, client.GVRManagedCluster, "", "gpu-elastic=true")
	if err != nil {
		return nil, fmt.Errorf("listing elastic GPU clusters: %w", err)
	}

	var clusters []ElasticCluster
	for _, item := range list.Items {
		clusters = append(clusters, parseElasticCluster(item.Object))
	}
	return clusters, nil
}
