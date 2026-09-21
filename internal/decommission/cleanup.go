package decommission

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

// Delete removes the cluster from ACM. For Hive-provisioned clusters it deletes
// the ClusterDeployment (which triggers infrastructure destruction). For imported
// clusters it only detaches from ACM — the external infrastructure is not managed.
// Returns true if it was a Hive cluster (infrastructure destroyed), false if imported.
func (m *Manager) Delete(ctx context.Context, clusterName string) (bool, error) {
	m.logger.Info("decommission.Delete", "cluster", clusterName)
	_, err := m.client.Get(ctx, client.GVRClusterDeployment, clusterName, clusterName)
	if err == nil {
		if err := m.client.Delete(ctx, client.GVRClusterDeployment, clusterName, clusterName); err != nil && !apierrors.IsNotFound(err) {
			return true, err
		}
		return true, nil
	}

	if err := m.client.Delete(ctx, client.GVRManagedCluster, "", clusterName); err != nil && !apierrors.IsNotFound(err) {
		return false, err
	}
	return false, nil
}

func (m *Manager) Cleanup(ctx context.Context, clusterName string) error {
	m.logger.Info("decommission.Cleanup", "cluster", clusterName)
	mwList, err := m.client.List(ctx, client.GVRManifestWork, clusterName, "")
	if err == nil {
		for _, mw := range mwList.Items {
			if err := m.client.Delete(ctx, client.GVRManifestWork, clusterName, mw.GetName()); err != nil && !apierrors.IsNotFound(err) {
				m.logger.Warn("cleanup: failed to delete ManifestWork", "name", mw.GetName(), "error", err)
			}
		}
	}

	if err := m.client.Delete(ctx, client.GVRManagedCluster, "", clusterName); err != nil && !apierrors.IsNotFound(err) {
		m.logger.Warn("cleanup: failed to delete ManagedCluster", "name", clusterName, "error", err)
	}

	if err := m.client.Delete(ctx, client.GVRNamespace, "", clusterName); err != nil && !apierrors.IsNotFound(err) {
		m.logger.Warn("cleanup: failed to delete namespace", "name", clusterName, "error", err)
	}

	return nil
}
