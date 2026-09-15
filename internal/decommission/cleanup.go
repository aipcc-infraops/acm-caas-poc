package decommission

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

func (m *Manager) Delete(ctx context.Context, clusterName string) error {
	m.logger.Info("decommission.Delete", "cluster", clusterName)
	_, err := m.client.Get(ctx, client.GVRClusterDeployment, clusterName, clusterName)
	if err == nil {
		if err := m.client.Delete(ctx, client.GVRClusterDeployment, clusterName, clusterName); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		return nil
	}

	if err := m.client.Delete(ctx, client.GVRManagedCluster, "", clusterName); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
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
