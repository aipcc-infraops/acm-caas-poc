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

// GetMachinePool returns the first MachinePool for a cluster. Namespace = cluster name (Hive convention).
func (m *Manager) GetMachinePool(ctx context.Context, clusterName string) (*MachinePoolInfo, error) {
	list, err := m.client.List(ctx, client.GVRMachinePool, clusterName, "")
	if err != nil {
		return nil, fmt.Errorf("listing MachinePools in namespace %s: %w", clusterName, err)
	}
	if len(list.Items) == 0 {
		return nil, fmt.Errorf("no MachinePool found for cluster %s", clusterName)
	}
	info := machinePoolInfoFromUnstructured(&list.Items[0])
	return &info, nil
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
