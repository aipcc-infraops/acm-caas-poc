package scaling

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

// ErrNoMachinePool is returned when no MachinePool exists for a cluster.
// IsHive=false means the cluster is imported — Hive scaling is not available.
type ErrNoMachinePool struct {
	ClusterName string
	IsHive      bool
	WorkerCount int
	WorkerType  string
}

func (e *ErrNoMachinePool) Error() string {
	if !e.IsHive {
		return fmt.Sprintf(
			"cluster %s was not provisioned by Hive — MachinePool scaling is not available.\n"+
				"Use your cloud provider's native scaling instead:\n"+
				"  IBM Cloud: ibmcloud ks worker-pool resize --cluster %s --size-per-zone N --worker-pool default\n"+
				"  AWS:       aws autoscaling set-desired-capacity\n"+
				"  GCP:       gcloud container node-pools update",
			e.ClusterName, e.ClusterName,
		)
	}
	return fmt.Sprintf(
		"no MachinePool found for cluster %s.\n"+
			"Current workers detected: %d × %s\n"+
			"To enable scaling management run:\n"+
			"  acmlab scaling init %s --worker-type %s --replicas %d",
		e.ClusterName, e.WorkerCount, e.WorkerType,
		e.ClusterName, e.WorkerType, e.WorkerCount,
	)
}

type Manager struct {
	client *client.Client
	cfg    config.Config
}

func New(c *client.Client, cfg config.Config) *Manager {
	return &Manager{client: c, cfg: cfg}
}

type MachinePoolInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Replicas  *int64 `json:"replicas,omitempty"`
	MinSize   *int64 `json:"minSize,omitempty"`
	MaxSize   *int64 `json:"maxSize,omitempty"`
	Platform  string `json:"platform,omitempty"`
}

// ClusterSupportsScaling returns true if the cluster was provisioned by Hive (ClusterDeployment exists).
func (m *Manager) ClusterSupportsScaling(ctx context.Context, clusterName string) (bool, error) {
	_, err := m.client.Get(ctx, client.GVRClusterDeployment, clusterName, clusterName)
	if errors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking ClusterDeployment for %s: %w", clusterName, err)
	}
	return true, nil
}

// GetCurrentWorkerInfo reads worker count and instance type from ManagedClusterInfo node labels.
func (m *Manager) GetCurrentWorkerInfo(ctx context.Context, clusterName string) (count int, workerType string, err error) {
	info, err := m.client.Get(ctx, client.GVRManagedClusterInfo, clusterName, clusterName)
	if err != nil {
		return 0, "bx2-4x16", nil
	}
	nodes, _, _ := unstructured.NestedSlice(info.Object, "status", "nodeList")
	for _, n := range nodes {
		node, ok := n.(map[string]interface{})
		if !ok {
			continue
		}
		labels, _, _ := unstructured.NestedStringMap(node, "labels")
		if _, isWorker := labels["node-role.kubernetes.io/worker"]; isWorker {
			count++
			if t, ok := labels["node.kubernetes.io/instance-type"]; ok && workerType == "" {
				workerType = t
			}
		}
	}
	if workerType == "" {
		workerType = "bx2-4x16"
	}
	return count, workerType, nil
}

// noMachinePoolErr builds the appropriate ErrNoMachinePool by checking cluster type.
func (m *Manager) noMachinePoolErr(ctx context.Context, clusterName string) error {
	isHive, _ := m.ClusterSupportsScaling(ctx, clusterName)
	if !isHive {
		return &ErrNoMachinePool{ClusterName: clusterName, IsHive: false}
	}
	count, workerType, _ := m.GetCurrentWorkerInfo(ctx, clusterName)
	return &ErrNoMachinePool{
		ClusterName: clusterName,
		IsHive:      true,
		WorkerCount: count,
		WorkerType:  workerType,
	}
}

// GetMachinePool returns the first MachinePool for a cluster. Namespace = cluster name (Hive convention).
// Returns *ErrNoMachinePool if no MachinePool exists, with context on whether this is a Hive or imported cluster.
func (m *Manager) GetMachinePool(ctx context.Context, clusterName string) (*MachinePoolInfo, error) {
	list, err := m.client.List(ctx, client.GVRMachinePool, clusterName, "")
	if err != nil {
		return nil, fmt.Errorf("listing MachinePools in namespace %s: %w", clusterName, err)
	}
	if len(list.Items) == 0 {
		return nil, m.noMachinePoolErr(ctx, clusterName)
	}
	info := machinePoolInfoFromUnstructured(&list.Items[0])
	return &info, nil
}

// InitMachinePool creates a MachinePool for an existing Hive cluster.
// If replicas matches detected worker count, no worker changes will be made.
func (m *Manager) InitMachinePool(ctx context.Context, clusterName, workerType string, replicas int) (*MachinePoolInfo, error) {
	isHive, err := m.ClusterSupportsScaling(ctx, clusterName)
	if err != nil {
		return nil, err
	}
	if !isHive {
		return nil, &ErrNoMachinePool{ClusterName: clusterName, IsHive: false}
	}

	mp := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "MachinePool",
			"metadata": map[string]interface{}{
				"name":      clusterName + "-worker",
				"namespace": clusterName,
			},
			"spec": map[string]interface{}{
				"clusterDeploymentRef": map[string]interface{}{
					"name": clusterName,
				},
				"name":     "worker",
				"replicas": int64(replicas),
				"platform": map[string]interface{}{
					"ibmcloud": map[string]interface{}{
						"type": workerType,
					},
				},
			},
		},
	}

	_, err = m.client.Create(ctx, client.GVRMachinePool, clusterName, mp)
	if err != nil {
		return nil, fmt.Errorf("creating MachinePool for %s: %w", clusterName, err)
	}

	return m.GetMachinePool(ctx, clusterName)
}

// SetReplicas patches the MachinePool replicas count and removes autoscaling if active.
func (m *Manager) SetReplicas(ctx context.Context, clusterName string, replicas int) error {
	mp, err := m.GetMachinePool(ctx, clusterName)
	if err != nil {
		return err
	}
	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas":    int64(replicas),
			"autoscaling": nil,
		},
	}
	data, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshaling patch: %w", err)
	}
	_, err = m.client.Patch(ctx, client.GVRMachinePool, mp.Namespace, mp.Name, types.MergePatchType, data)
	if err != nil {
		return fmt.Errorf("patching MachinePool %s/%s replicas: %w", mp.Namespace, mp.Name, err)
	}
	return nil
}

// EnableAutoscaling enables autoscaling on the MachinePool with min/max bounds.
func (m *Manager) EnableAutoscaling(ctx context.Context, clusterName string, min, max int) error {
	mp, err := m.GetMachinePool(ctx, clusterName)
	if err != nil {
		return err
	}
	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": nil,
			"autoscaling": map[string]interface{}{
				"minReplicas": int32(min),
				"maxReplicas": int32(max),
			},
		},
	}
	data, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshaling patch: %w", err)
	}
	_, err = m.client.Patch(ctx, client.GVRMachinePool, mp.Namespace, mp.Name, types.MergePatchType, data)
	if err != nil {
		return fmt.Errorf("patching MachinePool %s/%s autoscaling: %w", mp.Namespace, mp.Name, err)
	}
	return nil
}

// DisableAutoscaling removes autoscaling and sets a fixed replica count.
func (m *Manager) DisableAutoscaling(ctx context.Context, clusterName string, replicas int) error {
	return m.SetReplicas(ctx, clusterName, replicas)
}

// ListMachinePools returns all MachinePools across the fleet (all namespaces).
func (m *Manager) ListMachinePools(ctx context.Context) ([]MachinePoolInfo, error) {
	list, err := m.client.List(ctx, client.GVRMachinePool, "", "")
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing MachinePools: %w", err)
	}
	infos := make([]MachinePoolInfo, 0, len(list.Items))
	for i := range list.Items {
		infos = append(infos, machinePoolInfoFromUnstructured(&list.Items[i]))
	}
	return infos, nil
}

func machinePoolInfoFromUnstructured(mp *unstructured.Unstructured) MachinePoolInfo {
	info := MachinePoolInfo{
		Name:      mp.GetName(),
		Namespace: mp.GetNamespace(),
	}

	if replicas, found, err := unstructured.NestedInt64(mp.Object, "spec", "replicas"); err == nil && found {
		v := replicas
		info.Replicas = &v
	}

	if min, found, err := unstructured.NestedInt64(mp.Object, "spec", "autoscaling", "minReplicas"); err == nil && found {
		v := min
		info.MinSize = &v
	}
	if max, found, err := unstructured.NestedInt64(mp.Object, "spec", "autoscaling", "maxReplicas"); err == nil && found {
		v := max
		info.MaxSize = &v
	}

	for _, platform := range []string{"ibmvpc", "aws", "gcp", "azure"} {
		if _, found, _ := unstructured.NestedMap(mp.Object, "spec", "platform", platform); found {
			info.Platform = platform
			break
		}
	}

	return info
}
