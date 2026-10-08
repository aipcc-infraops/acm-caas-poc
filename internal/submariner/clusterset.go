package submariner

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type CreateTestSetOpts struct {
	Name     string
	Clusters []string
	Confirm  bool
}

func (m *Manager) CreateTestSet(ctx context.Context, opts CreateTestSetOpts) error {
	m.logger.Info("submariner.CreateTestSet", "name", opts.Name, "clusters", opts.Clusters)

	if len(opts.Clusters) < 2 {
		return fmt.Errorf("at least 2 clusters required for a Submariner test set, got %d", len(opts.Clusters))
	}

	for _, name := range opts.Clusters {
		_, err := m.client.Get(ctx, client.GVRManagedCluster, "", name)
		if err != nil {
			return fmt.Errorf("cluster %q not found: %w", name, err)
		}
	}

	if !opts.Confirm {
		return fmt.Errorf("relabelling clusters changes their ClusterSet membership, which can affect existing placements; pass --confirm to proceed")
	}

	cs := buildManagedClusterSet(opts.Name)
	if err := m.client.CreateIfNotExists(ctx, client.GVRManagedClusterSet, "", cs); err != nil {
		return fmt.Errorf("creating ManagedClusterSet %q: %w", opts.Name, err)
	}

	for _, name := range opts.Clusters {
		if err := m.labelClusterForSet(ctx, name, opts.Name); err != nil {
			return fmt.Errorf("labelling cluster %q for set %q: %w", name, opts.Name, err)
		}
	}

	if err := m.ensureBrokerCR(ctx, opts.Name); err != nil {
		m.logger.Info("submariner.CreateTestSet", "broker_cr", err.Error())
	}

	return nil
}

func (m *Manager) labelClusterForSet(ctx context.Context, cluster, clusterSet string) error {
	patch := fmt.Sprintf(`{"metadata":{"labels":{"cluster.open-cluster-management.io/clusterset":"%s"}}}`, clusterSet)
	_, err := m.client.Patch(ctx, client.GVRManagedCluster, "", cluster, types.MergePatchType, []byte(patch))
	return err
}

func buildManagedClusterSet(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta2",
			"kind":       "ManagedClusterSet",
			"metadata": map[string]interface{}{
				"name": name,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
				},
			},
			"spec": map[string]interface{}{},
		},
	}
}
