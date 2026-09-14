package decommission

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

func (m *Manager) Delete(ctx context.Context, clusterName string) error {
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
	mwList, err := m.client.List(ctx, client.GVRManifestWork, clusterName, "")
	if err == nil {
		for _, mw := range mwList.Items {
			_ = m.client.Delete(ctx, client.GVRManifestWork, clusterName, mw.GetName())
		}
	}

	if err := m.client.Delete(ctx, client.GVRManagedCluster, "", clusterName); err != nil && !apierrors.IsNotFound(err) {
		// non-fatal
	}

	if err := m.client.Delete(ctx, client.GVRNamespace, "", clusterName); err != nil && !apierrors.IsNotFound(err) {
		// non-fatal
	}

	return nil
}
