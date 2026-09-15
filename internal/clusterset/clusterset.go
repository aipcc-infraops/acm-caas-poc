package clusterset

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

const clusterSetLabel = "cluster.open-cluster-management.io/clusterset"

type ClusterSetInfo struct {
	Name     string   `json:"name"`
	Members  []string `json:"members,omitempty"`
	Count    int      `json:"count"`
}

type Manager struct {
	client *client.Client
	cfg    config.Config
	logger *slog.Logger
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger}
}

func (m *Manager) Create(ctx context.Context, name, namespace string) error {
	m.logger.Info("clusterset.Create", "name", name, "namespace", namespace)

	cs := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta2",
			"kind":       "ManagedClusterSet",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": map[string]interface{}{
				"clusterSelector": map[string]interface{}{
					"selectorType": "ExclusiveClusterSetLabel",
				},
			},
		},
	}
	if err := m.client.CreateIfNotExists(ctx, client.GVRManagedClusterSet, "", cs); err != nil {
		return fmt.Errorf("creating ManagedClusterSet %s: %w", name, err)
	}

	binding := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta2",
			"kind":       "ManagedClusterSetBinding",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"clusterSet": name,
			},
		},
	}
	if err := m.client.CreateIfNotExists(ctx, client.GVRManagedClusterSetBinding, namespace, binding); err != nil {
		return fmt.Errorf("creating ManagedClusterSetBinding %s in %s: %w", name, namespace, err)
	}

	return nil
}

func (m *Manager) Remove(ctx context.Context, name, namespace string) error {
	m.logger.Info("clusterset.Remove", "name", name, "namespace", namespace)

	if err := m.client.DeleteIfExists(ctx, client.GVRManagedClusterSetBinding, namespace, name); err != nil {
		return fmt.Errorf("deleting ManagedClusterSetBinding %s: %w", name, err)
	}
	if err := m.client.DeleteIfExists(ctx, client.GVRManagedClusterSet, "", name); err != nil {
		return fmt.Errorf("deleting ManagedClusterSet %s: %w", name, err)
	}
	return nil
}

func (m *Manager) List(ctx context.Context) ([]ClusterSetInfo, error) {
	m.logger.Info("clusterset.List")

	sets, err := m.client.List(ctx, client.GVRManagedClusterSet, "", "")
	if err != nil {
		return nil, fmt.Errorf("listing ManagedClusterSets: %w", err)
	}

	clusters, err := m.client.List(ctx, client.GVRManagedCluster, "", "")
	if err != nil {
		return nil, fmt.Errorf("listing ManagedClusters: %w", err)
	}

	membership := map[string][]string{}
	for _, mc := range clusters.Items {
		labels := mc.GetLabels()
		if labels == nil {
			continue
		}
		if setName, ok := labels[clusterSetLabel]; ok {
			membership[setName] = append(membership[setName], mc.GetName())
		}
	}

	result := make([]ClusterSetInfo, 0, len(sets.Items))
	for _, s := range sets.Items {
		name := s.GetName()
		members := membership[name]
		result = append(result, ClusterSetInfo{
			Name:    name,
			Members: members,
			Count:   len(members),
		})
	}
	return result, nil
}

func (m *Manager) Assign(ctx context.Context, clusterName, setName string) error {
	m.logger.Info("clusterset.Assign", "cluster", clusterName, "set", setName)

	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				clusterSetLabel: setName,
			},
		},
	}
	data, _ := json.Marshal(patch)
	_, err := m.client.Patch(ctx, client.GVRManagedCluster, "", clusterName, types.MergePatchType, data)
	if err != nil {
		return fmt.Errorf("assigning cluster %s to set %s: %w", clusterName, setName, err)
	}
	return nil
}
