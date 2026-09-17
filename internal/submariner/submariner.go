package submariner

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"k8s.io/apimachinery/pkg/types"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

type SubmarinerStatus struct {
	ClusterSet string          `json:"clusterSet"`
	Clusters   []ClusterStatus `json:"clusters"`
	Connected  bool            `json:"connected"`
}

type ClusterStatus struct {
	Name         string `json:"name"`
	GatewayReady bool   `json:"gatewayReady"`
	AgentReady   bool   `json:"agentReady"`
	Connections  int    `json:"connections"`
}

type SubmarinerInfo struct {
	ClusterSet string `json:"clusterSet"`
	Clusters   int    `json:"clusters"`
	Enabled    bool   `json:"enabled"`
}

type Manager struct {
	client *client.Client
	cfg    config.Config
	logger *slog.Logger
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger}
}

func (m *Manager) Enable(ctx context.Context, clusterSet string) error {
	m.logger.Info("submariner.Enable", "clusterSet", clusterSet)

	clusters, err := m.clustersInSet(ctx, clusterSet)
	if err != nil {
		return err
	}
	if len(clusters) == 0 {
		return fmt.Errorf("no clusters found in ClusterSet %q", clusterSet)
	}

	for _, name := range clusters {
		addOn := buildSubmarinerAddOn(name)
		if err := m.client.CreateIfNotExists(ctx, client.GVRManagedClusterAddOn, name, addOn); err != nil {
			return fmt.Errorf("creating submariner addon for %s: %w", name, err)
		}

		cfg := buildSubmarinerConfig(name)
		if err := m.client.CreateIfNotExists(ctx, client.GVRSubmarinerConfig, name, cfg); err != nil {
			return fmt.Errorf("creating submariner config for %s: %w", name, err)
		}

		patch, _ := json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{"submariner": "enabled"},
			},
		})
		if _, err := m.client.Patch(ctx, client.GVRManagedCluster, "", name, types.MergePatchType, patch); err != nil {
			return fmt.Errorf("labelling cluster %s: %w", name, err)
		}
	}
	return nil
}

func (m *Manager) Disable(ctx context.Context, clusterSet string) error {
	m.logger.Info("submariner.Disable", "clusterSet", clusterSet)

	clusters, err := m.clustersInSet(ctx, clusterSet)
	if err != nil {
		return err
	}

	for _, name := range clusters {
		_ = m.client.DeleteIfExists(ctx, client.GVRManagedClusterAddOn, name, "submariner")
		_ = m.client.DeleteIfExists(ctx, client.GVRSubmarinerConfig, name, "submariner")

		patch, _ := json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{"submariner": nil},
			},
		})
		_, _ = m.client.Patch(ctx, client.GVRManagedCluster, "", name, types.MergePatchType, patch)
	}
	return nil
}

func (m *Manager) Status(ctx context.Context, clusterSet string) (*SubmarinerStatus, error) {
	m.logger.Info("submariner.Status", "clusterSet", clusterSet)

	clusters, err := m.clustersInSet(ctx, clusterSet)
	if err != nil {
		return nil, err
	}

	status := &SubmarinerStatus{ClusterSet: clusterSet, Connected: true}
	for _, name := range clusters {
		addOn, err := m.client.Get(ctx, client.GVRManagedClusterAddOn, name, "submariner")
		if err != nil {
			status.Clusters = append(status.Clusters, ClusterStatus{Name: name})
			status.Connected = false
			continue
		}
		cs := parseClusterStatus(name, addOn.Object)
		if !cs.GatewayReady || !cs.AgentReady {
			status.Connected = false
		}
		status.Clusters = append(status.Clusters, cs)
	}
	return status, nil
}

func (m *Manager) List(ctx context.Context) ([]SubmarinerInfo, error) {
	m.logger.Info("submariner.List")

	list, err := m.client.List(ctx, client.GVRManagedCluster, "", "submariner=enabled")
	if err != nil {
		return nil, fmt.Errorf("listing submariner clusters: %w", err)
	}

	objs := make([]map[string]interface{}, len(list.Items))
	for i, item := range list.Items {
		objs[i] = item.Object
	}
	return groupByClusterSet(objs), nil
}

func (m *Manager) clustersInSet(ctx context.Context, clusterSet string) ([]string, error) {
	sel := fmt.Sprintf("cluster.open-cluster-management.io/clusterset=%s", clusterSet)
	list, err := m.client.List(ctx, client.GVRManagedCluster, "", sel)
	if err != nil {
		return nil, fmt.Errorf("listing clusters in set %q: %w", clusterSet, err)
	}
	names := make([]string, len(list.Items))
	for i, item := range list.Items {
		names[i] = item.GetName()
	}
	return names, nil
}
