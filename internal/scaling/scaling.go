package scaling

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

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
	VendorType  string // distributionInfo.type — provider-independent detection
	WorkerCount int
	WorkerType  string
}

func (e *ErrNoMachinePool) Error() string {
	if !e.IsHive {
		// Provider-independent message: use VendorType to give the best hint,
		// but don't hardcode provider-specific commands as the cluster may run
		// on any Kubernetes distribution.
		return fmt.Sprintf(
			"cluster %s (%s) was not provisioned by Hive — MachinePool scaling is not available.\n"+
				"Use acmlab scaling init to adopt existing workers, or your cloud provider's\n"+
				"native scaling tools (e.g., ibmcloud ks, aws eks, gcloud container).",
			e.ClusterName, coalesce(e.VendorType, "unknown type"),
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
	logger *slog.Logger
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger}
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
	m.logger.Info("scaling.ClusterSupportsScaling", "cluster", clusterName)
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
	m.logger.Info("scaling.GetCurrentWorkerInfo", "cluster", clusterName)
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
		// Use distributionInfo.type for a provider-independent error message.
		clusterType, _ := m.client.GetClusterType(ctx, clusterName)
		return &ErrNoMachinePool{
			ClusterName: clusterName,
			IsHive:      false,
			VendorType:  string(clusterType),
		}
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
	m.logger.Info("scaling.GetMachinePool", "cluster", clusterName)
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
// Platform is auto-detected from the ClusterDeployment spec.platform key.
func (m *Manager) InitMachinePool(ctx context.Context, clusterName, workerType string, replicas int) (*MachinePoolInfo, error) {
	m.logger.Info("scaling.InitMachinePool", "cluster", clusterName)
	cd, err := m.client.Get(ctx, client.GVRClusterDeployment, clusterName, clusterName)
	if err != nil {
		return nil, &ErrNoMachinePool{ClusterName: clusterName, IsHive: false}
	}

	platform := detectPlatform(cd)
	if platform == "" {
		return nil, fmt.Errorf("cannot detect platform from ClusterDeployment %s — spec.platform is empty", clusterName)
	}

	platformBlock := map[string]interface{}{
		platform: map[string]interface{}{
			"type": workerType,
		},
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
				"platform": platformBlock,
			},
		},
	}

	_, err = m.client.Create(ctx, client.GVRMachinePool, clusterName, mp)
	if err != nil {
		return nil, fmt.Errorf("creating MachinePool for %s: %w", clusterName, err)
	}

	return m.GetMachinePool(ctx, clusterName)
}

var knownPlatforms = []string{"ibmcloud", "aws", "gcp", "azure", "openstack", "vsphere", "ovirt"}

func detectPlatform(cd *unstructured.Unstructured) string {
	platformMap, _, _ := unstructured.NestedMap(cd.Object, "spec", "platform")
	for _, p := range knownPlatforms {
		if _, ok := platformMap[p]; ok {
			return p
		}
	}
	return ""
}

// SetFlavor patches the MachinePool worker instance type via the platform-specific spec path.
func (m *Manager) SetFlavor(ctx context.Context, clusterName, workerType string) error {
	m.logger.Info("scaling.SetFlavor", "cluster", clusterName, "workerType", workerType)
	mp, err := m.GetMachinePool(ctx, clusterName)
	if err != nil {
		return err
	}

	cd, err := m.client.Get(ctx, client.GVRClusterDeployment, clusterName, clusterName)
	if err != nil {
		return fmt.Errorf("getting ClusterDeployment for %s: %w", clusterName, err)
	}
	platform := detectPlatform(cd)
	if platform == "" {
		return fmt.Errorf("cannot detect platform from ClusterDeployment %s — spec.platform is empty", clusterName)
	}

	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"platform": map[string]interface{}{
				platform: map[string]interface{}{
					"type": workerType,
				},
			},
		},
	}
	data, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshaling patch: %w", err)
	}
	_, err = m.client.Patch(ctx, client.GVRMachinePool, mp.Namespace, mp.Name, types.MergePatchType, data)
	if err != nil {
		return fmt.Errorf("patching MachinePool %s/%s flavor: %w", mp.Namespace, mp.Name, err)
	}
	return nil
}

// SetReplicas patches the MachinePool replicas count and removes autoscaling if active.
func (m *Manager) SetReplicas(ctx context.Context, clusterName string, replicas int) error {
	m.logger.Info("scaling.SetReplicas", "cluster", clusterName, "replicas", replicas)
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
	m.logger.Info("scaling.EnableAutoscaling", "cluster", clusterName, "min", min, "max", max)
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
	m.logger.Info("scaling.DisableAutoscaling", "cluster", clusterName, "replicas", replicas)
	return m.SetReplicas(ctx, clusterName, replicas)
}

// ListMachinePools returns all MachinePools across the fleet (all namespaces).
func (m *Manager) ListMachinePools(ctx context.Context) ([]MachinePoolInfo, error) {
	m.logger.Info("scaling.ListMachinePools")
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

type NodePoolInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Replicas  *int64 `json:"replicas,omitempty"`
	Platform  string `json:"platform,omitempty"`
}

type MachineDeploymentInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Replicas  *int64 `json:"replicas,omitempty"`
}

func (m *Manager) GetNodePool(ctx context.Context, clusterName string) (*NodePoolInfo, error) {
	m.logger.Info("scaling.GetNodePool", "cluster", clusterName)
	for _, ns := range []string{"clusters", clusterName} {
		list, err := m.client.List(ctx, client.GVRNodePool, ns, "")
		if err != nil {
			continue
		}
		for i := range list.Items {
			np := &list.Items[i]
			spec, _, _ := unstructured.NestedString(np.Object, "spec", "clusterName")
			if spec == clusterName || np.GetNamespace() == clusterName {
				return nodePoolInfoFromUnstructured(np), nil
			}
		}
	}
	return nil, fmt.Errorf("no NodePool found for cluster %s", clusterName)
}

func (m *Manager) GetMachineDeployment(ctx context.Context, clusterName string) (*MachineDeploymentInfo, error) {
	m.logger.Info("scaling.GetMachineDeployment", "cluster", clusterName)
	for _, ns := range []string{clusterName, "default"} {
		list, err := m.client.List(ctx, client.GVRCAPIMachineDeployment, ns, "")
		if err != nil {
			continue
		}
		for i := range list.Items {
			md := &list.Items[i]
			clusterLabel, _, _ := unstructured.NestedString(md.Object, "metadata", "labels", "cluster.x-k8s.io/cluster-name")
			if clusterLabel == clusterName || md.GetNamespace() == clusterName {
				return machineDeploymentInfoFromUnstructured(md), nil
			}
		}
	}
	return nil, fmt.Errorf("no CAPI MachineDeployment found for cluster %s", clusterName)
}

func (m *Manager) SetNodePoolReplicas(ctx context.Context, clusterName string, replicas int) error {
	m.logger.Info("scaling.SetNodePoolReplicas", "cluster", clusterName, "replicas", replicas)
	np, err := m.GetNodePool(ctx, clusterName)
	if err != nil {
		return err
	}
	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": int64(replicas),
		},
	}
	data, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshaling patch: %w", err)
	}
	_, err = m.client.Patch(ctx, client.GVRNodePool, np.Namespace, np.Name, types.MergePatchType, data)
	if err != nil {
		return fmt.Errorf("patching NodePool %s/%s replicas: %w", np.Namespace, np.Name, err)
	}
	return nil
}

func (m *Manager) SetMachineDeploymentReplicas(ctx context.Context, clusterName string, replicas int) error {
	m.logger.Info("scaling.SetMachineDeploymentReplicas", "cluster", clusterName, "replicas", replicas)
	md, err := m.GetMachineDeployment(ctx, clusterName)
	if err != nil {
		return err
	}
	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": int64(replicas),
		},
	}
	data, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshaling patch: %w", err)
	}
	_, err = m.client.Patch(ctx, client.GVRCAPIMachineDeployment, md.Namespace, md.Name, types.MergePatchType, data)
	if err != nil {
		return fmt.Errorf("patching MachineDeployment %s/%s replicas: %w", md.Namespace, md.Name, err)
	}
	return nil
}

// SetReplicasAuto tries MachinePool, then NodePool, then CAPI MachineDeployment.
func (m *Manager) SetReplicasAuto(ctx context.Context, clusterName string, replicas int) error {
	m.logger.Info("scaling.SetReplicasAuto", "cluster", clusterName, "replicas", replicas)
	err := m.SetReplicas(ctx, clusterName, replicas)
	if err == nil {
		return nil
	}
	var noMP *ErrNoMachinePool
	if !isNoMachinePool(err, &noMP) {
		return err
	}

	if npErr := m.SetNodePoolReplicas(ctx, clusterName, replicas); npErr == nil {
		return nil
	}

	if mdErr := m.SetMachineDeploymentReplicas(ctx, clusterName, replicas); mdErr == nil {
		return nil
	}

	return err
}

func isNoMachinePool(err error, target **ErrNoMachinePool) bool {
	e, ok := err.(*ErrNoMachinePool)
	if ok && target != nil {
		*target = e
	}
	return ok
}

func nodePoolInfoFromUnstructured(np *unstructured.Unstructured) *NodePoolInfo {
	info := &NodePoolInfo{
		Name:      np.GetName(),
		Namespace: np.GetNamespace(),
	}
	if replicas, found, err := unstructured.NestedInt64(np.Object, "spec", "replicas"); err == nil && found {
		info.Replicas = &replicas
	}
	if pt, found, err := unstructured.NestedString(np.Object, "spec", "platform", "type"); err == nil && found {
		info.Platform = pt
	}
	return info
}

func machineDeploymentInfoFromUnstructured(md *unstructured.Unstructured) *MachineDeploymentInfo {
	info := &MachineDeploymentInfo{
		Name:      md.GetName(),
		Namespace: md.GetNamespace(),
	}
	if replicas, found, err := unstructured.NestedInt64(md.Object, "spec", "replicas"); err == nil && found {
		info.Replicas = &replicas
	}
	return info
}

func coalesce(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
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

	for _, platform := range []string{"ibmcloud", "ibmvpc", "aws", "gcp", "azure"} {
		if _, found, _ := unstructured.NestedMap(mp.Object, "spec", "platform", platform); found {
			info.Platform = platform
			break
		}
	}

	return info
}
