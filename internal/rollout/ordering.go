package rollout

import (
	"context"
	"fmt"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type OrderedManifest struct {
	Ordinal  int                    `json:"ordinal"`
	Object   map[string]interface{} `json:"object"`
}

type OrderedWorkInfo struct {
	Name     string `json:"name"`
	Cluster  string `json:"cluster"`
	Manifests int   `json:"manifests"`
	Status   string `json:"status"`
}

func (m *Manager) CreateOrderedWork(ctx context.Context, cluster, name string, manifests []OrderedManifest) error {
	m.logger.Info("rollout.CreateOrderedWork", "cluster", cluster, "name", name)

	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i].Ordinal < manifests[j].Ordinal
	})

	mw := buildOrderedManifestWork(cluster, name, manifests)
	if err := m.client.CreateIfNotExists(ctx, client.GVRManifestWork, cluster, mw); err != nil {
		return fmt.Errorf("creating ordered ManifestWork %s on %s: %w", name, cluster, err)
	}
	return nil
}

func (m *Manager) GetOrderedWork(ctx context.Context, cluster, name string) (*OrderedWorkInfo, error) {
	m.logger.Info("rollout.GetOrderedWork", "cluster", cluster, "name", name)

	obj, err := m.client.Get(ctx, client.GVRManifestWork, cluster, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("ordered work %s not found on %s", name, cluster)
		}
		return nil, fmt.Errorf("getting ordered work %s: %w", name, err)
	}

	manifestCount := 0
	spec, _ := obj.Object["spec"].(map[string]interface{})
	if spec != nil {
		if wl, ok := spec["workload"].(map[string]interface{}); ok {
			if ms, ok := wl["manifests"].([]interface{}); ok {
				manifestCount = len(ms)
			}
		}
	}

	status := "Unknown"
	statusMap, _ := obj.Object["status"].(map[string]interface{})
	if statusMap != nil {
		conditions, _ := statusMap["conditions"].([]interface{})
		for _, c := range conditions {
			cm, _ := c.(map[string]interface{})
			if cm != nil && cm["type"] == "Applied" {
				s, _ := cm["status"].(string)
				if s == "True" {
					status = "Applied"
				} else {
					status = "Pending"
				}
			}
		}
	}

	return &OrderedWorkInfo{
		Name:      name,
		Cluster:   cluster,
		Manifests: manifestCount,
		Status:    status,
	}, nil
}

func (m *Manager) ListOrderedWork(ctx context.Context, cluster string) ([]OrderedWorkInfo, error) {
	m.logger.Info("rollout.ListOrderedWork", "cluster", cluster)

	list, err := m.client.List(ctx, client.GVRManifestWork, cluster, "acmlab.redhat.com/ordered-work=true")
	if err != nil {
		return nil, fmt.Errorf("listing ordered work on %s: %w", cluster, err)
	}

	result := make([]OrderedWorkInfo, 0, len(list.Items))
	for _, item := range list.Items {
		manifestCount := 0
		spec, _ := item.Object["spec"].(map[string]interface{})
		if spec != nil {
			if wl, ok := spec["workload"].(map[string]interface{}); ok {
				if ms, ok := wl["manifests"].([]interface{}); ok {
					manifestCount = len(ms)
				}
			}
		}
		result = append(result, OrderedWorkInfo{
			Name:      item.GetName(),
			Cluster:   cluster,
			Manifests: manifestCount,
		})
	}
	return result, nil
}

func (m *Manager) RemoveOrderedWork(ctx context.Context, cluster, name string) error {
	m.logger.Info("rollout.RemoveOrderedWork", "cluster", cluster, "name", name)

	if err := m.client.DeleteIfExists(ctx, client.GVRManifestWork, cluster, name); err != nil {
		return fmt.Errorf("deleting ordered work %s from %s: %w", name, cluster, err)
	}
	return nil
}
