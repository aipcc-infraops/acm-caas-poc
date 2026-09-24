package pool

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/lifecycle"
	"github.com/pablofelix/acm-caas-poc/internal/provisioning"
)

const (
	labelPool        = "acmlab.redhat.com/pool"
	labelPoolClaimed = "acmlab.redhat.com/pool-claimed"
	labelPoolIndex   = "acmlab.redhat.com/pool-index"
)

var poolConfigs = make(map[string]provisioning.ClusterOpts)

type ManualPoolOpts struct {
	Name          string
	Size          int
	ProvisionOpts provisioning.ClusterOpts
}

func (m *Manager) CreateManualPool(ctx context.Context, opts ManualPoolOpts) error {
	m.logger.Info("pool.CreateManualPool", "name", opts.Name, "size", opts.Size)
	if m.provisioning == nil {
		return fmt.Errorf("provisioning manager required for manual pool (use NewWithManagers)")
	}
	if opts.Size <= 0 {
		opts.Size = 2
	}

	for i := 1; i <= opts.Size; i++ {
		clusterName := fmt.Sprintf("%s-%d", opts.Name, i)
		clusterOpts := opts.ProvisionOpts
		clusterOpts.Name = clusterName

		if err := m.provisioning.Create(ctx, clusterOpts); err != nil {
			return fmt.Errorf("provisioning pool cluster %s: %w", clusterName, err)
		}

		if err := m.labelPoolCluster(ctx, clusterName, opts.Name); err != nil {
			return fmt.Errorf("labeling pool cluster %s: %w", clusterName, err)
		}
	}

	poolConfigs[opts.Name] = opts.ProvisionOpts

	m.logger.Info("pool.CreateManualPool: all clusters created, waiting for install and hibernate separately",
		"pool", opts.Name, "count", opts.Size)
	return nil
}

func (m *Manager) WaitManualPoolReady(ctx context.Context, poolName string, timeout time.Duration) error {
	m.logger.Info("pool.WaitManualPoolReady", "pool", poolName, "timeout", timeout)
	if m.provisioning == nil || m.lifecycle == nil {
		return fmt.Errorf("provisioning and lifecycle managers required (use NewWithManagers)")
	}
	if timeout == 0 {
		timeout = 50 * time.Minute
	}

	clusters, err := m.listPoolClusters(ctx, poolName)
	if err != nil {
		return err
	}
	if len(clusters) == 0 {
		return fmt.Errorf("no clusters found for pool %s", poolName)
	}

	for _, name := range clusters {
		m.logger.Info("pool.WaitManualPoolReady: waiting for install", "cluster", name)
		if err := m.provisioning.WaitForProvision(ctx, name, timeout); err != nil {
			return fmt.Errorf("waiting for cluster %s: %w", name, err)
		}

		m.logger.Info("pool.WaitManualPoolReady: hibernating", "cluster", name)
		if err := m.lifecycle.Hibernate(ctx, name, name); err != nil {
			return fmt.Errorf("hibernating cluster %s: %w", name, err)
		}
	}

	return nil
}

func (m *Manager) ClaimManualPool(ctx context.Context, poolName, claimName string) (*ClaimInfo, error) {
	m.logger.Info("pool.ClaimManualPool", "pool", poolName, "claim", claimName)
	if m.lifecycle == nil {
		return nil, fmt.Errorf("lifecycle manager required for manual pool (use NewWithManagers)")
	}

	clusters, err := m.listPoolClusters(ctx, poolName)
	if err != nil {
		return nil, err
	}

	for _, name := range clusters {
		claimed, err := m.isClusterClaimed(ctx, name)
		if err != nil || claimed {
			continue
		}

		state, err := m.lifecycle.GetPowerState(ctx, name, name)
		if err != nil {
			continue
		}
		if state != lifecycle.PowerStateHibernating {
			continue
		}

		if err := m.lifecycle.Resume(ctx, name, name); err != nil {
			return nil, fmt.Errorf("resuming cluster %s: %w", name, err)
		}

		if err := m.setClaimLabel(ctx, name, "true"); err != nil {
			return nil, fmt.Errorf("marking cluster %s as claimed: %w", name, err)
		}

		if m.provisioning != nil {
			go m.replenishPool(poolName, len(clusters))
		}

		return &ClaimInfo{
			Name:    claimName,
			Pool:    poolName,
			Cluster: name,
			Status:  "Running",
		}, nil
	}

	return nil, fmt.Errorf("no available (hibernated, unclaimed) cluster in pool %s", poolName)
}

func (m *Manager) ReleaseManualClaim(ctx context.Context, clusterName string) error {
	m.logger.Info("pool.ReleaseManualClaim", "cluster", clusterName)
	if m.lifecycle == nil {
		return fmt.Errorf("lifecycle manager required for manual pool (use NewWithManagers)")
	}

	if err := m.lifecycle.Hibernate(ctx, clusterName, clusterName); err != nil {
		return fmt.Errorf("hibernating cluster %s: %w", clusterName, err)
	}

	return m.setClaimLabel(ctx, clusterName, "false")
}

func (m *Manager) ListManualPool(ctx context.Context, poolName string) (*PoolInfo, error) {
	m.logger.Info("pool.ListManualPool", "pool", poolName)

	list, err := m.client.List(ctx, client.GVRClusterDeployment, "", labelPool+"="+poolName)
	if err != nil {
		return nil, fmt.Errorf("listing pool clusters: %w", err)
	}

	info := &PoolInfo{
		Name: poolName,
		Size: len(list.Items),
	}

	for _, item := range list.Items {
		labels, _, _ := unstructured.NestedStringMap(item.Object, "metadata", "labels")
		installed, _, _ := unstructured.NestedBool(item.Object, "spec", "installed")
		powerState, _, _ := unstructured.NestedString(item.Object, "spec", "powerState")

		switch {
		case labels[labelPoolClaimed] == "true":
			info.Claimed++
		case installed && powerState == string(lifecycle.PowerStateHibernating):
			info.Standby++
			info.Ready++
		case installed:
			info.Ready++
		}
	}

	return info, nil
}

func (m *Manager) DeleteManualPool(ctx context.Context, poolName string) error {
	m.logger.Info("pool.DeleteManualPool", "pool", poolName)
	if m.provisioning == nil {
		return fmt.Errorf("provisioning manager required for manual pool (use NewWithManagers)")
	}

	clusters, err := m.listPoolClusters(ctx, poolName)
	if err != nil {
		return err
	}

	var lastErr error
	for _, name := range clusters {
		if err := m.provisioning.Destroy(ctx, name); err != nil {
			m.logger.Error("pool.DeleteManualPool: failed to destroy cluster", "cluster", name, "error", err)
			lastErr = err
		}
	}

	return lastErr
}

func (m *Manager) replenishPool(poolName string, currentSize int) {
	baseOpts, ok := poolConfigs[poolName]
	if !ok {
		m.logger.Error("pool.replenish: no config found for pool", "pool", poolName)
		return
	}

	clusterName := fmt.Sprintf("%s-%d", poolName, currentSize+1)
	m.logger.Info("pool.replenish: provisioning replacement cluster", "pool", poolName, "cluster", clusterName)

	ctx := context.Background()
	opts := baseOpts
	opts.Name = clusterName

	if err := m.provisioning.Create(ctx, opts); err != nil {
		m.logger.Error("pool.replenish: failed to provision", "cluster", clusterName, "error", err)
		return
	}

	if err := m.labelPoolCluster(ctx, clusterName, poolName); err != nil {
		m.logger.Error("pool.replenish: failed to label", "cluster", clusterName, "error", err)
		return
	}

	m.logger.Info("pool.replenish: replacement cluster created, will hibernate once installed", "cluster", clusterName)
}

func (m *Manager) listPoolClusters(ctx context.Context, poolName string) ([]string, error) {
	list, err := m.client.List(ctx, client.GVRClusterDeployment, "", labelPool+"="+poolName)
	if err != nil {
		return nil, fmt.Errorf("listing pool clusters: %w", err)
	}

	names := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		name, _, _ := unstructured.NestedString(item.Object, "metadata", "name")
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

func (m *Manager) isClusterClaimed(ctx context.Context, name string) (bool, error) {
	cd, err := m.client.Get(ctx, client.GVRClusterDeployment, name, name)
	if err != nil {
		return false, err
	}
	labels, _, _ := unstructured.NestedStringMap(cd.Object, "metadata", "labels")
	return labels[labelPoolClaimed] == "true", nil
}

func (m *Manager) labelPoolCluster(ctx context.Context, clusterName, poolName string) error {
	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				labelPool:        poolName,
				labelPoolClaimed: "false",
			},
		},
	}
	data, _ := json.Marshal(patch)

	_, err := m.client.Patch(ctx, client.GVRClusterDeployment, clusterName, clusterName,
		types.MergePatchType, data)
	if err != nil {
		return fmt.Errorf("patching ClusterDeployment labels: %w", err)
	}

	mcPatch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				labelPool: poolName,
			},
		},
	}
	mcData, _ := json.Marshal(mcPatch)

	_, err = m.client.Patch(ctx, client.GVRManagedCluster, "", clusterName,
		types.MergePatchType, mcData)
	return err
}

func (m *Manager) setClaimLabel(ctx context.Context, clusterName, value string) error {
	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				labelPoolClaimed: value,
			},
		},
	}
	data, _ := json.Marshal(patch)

	_, err := m.client.Patch(ctx, client.GVRClusterDeployment, clusterName, clusterName,
		types.MergePatchType, data)
	return err
}
