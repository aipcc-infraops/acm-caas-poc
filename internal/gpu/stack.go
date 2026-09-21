package gpu

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type DriftStatus struct {
	Cluster    string   `json:"cluster"`
	Compliant  bool     `json:"compliant"`
	Degraded   []string `json:"degraded,omitempty"`
	Conditions []string `json:"conditions,omitempty"`
}

func (m *Manager) PreflightStack(ctx context.Context, cluster string) error {
	m.logger.Info("gpu.PreflightStack", "cluster", cluster)

	mc, err := m.client.Get(ctx, client.GVRManagedCluster, "", cluster)
	if err != nil {
		return fmt.Errorf("cluster %s not found: %w", cluster, err)
	}

	labels, _, _ := unstructured.NestedStringMap(mc.Object, "metadata", "labels")
	if labels["gpu-type"] == "" {
		return fmt.Errorf("cluster %s has no gpu-type label", cluster)
	}

	conditions, _, _ := unstructured.NestedSlice(mc.Object, "status", "conditions")
	available := false
	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := cond["type"].(string)
		condStatus, _ := cond["status"].(string)
		if condType == "ManagedClusterConditionAvailable" && condStatus == "True" {
			available = true
			break
		}
	}
	if !available {
		return fmt.Errorf("cluster %s is not available", cluster)
	}

	return nil
}

func (m *Manager) DeployStack(ctx context.Context, cluster, clusterSet string) error {
	m.logger.Info("gpu.DeployStack", "cluster", cluster)

	if err := m.PreflightStack(ctx, cluster); err != nil {
		return fmt.Errorf("preflight failed: %w", err)
	}

	kueueMW := buildKueueManifestWork(cluster)
	if err := m.client.CreateIfNotExists(ctx, client.GVRManifestWork, cluster, kueueMW); err != nil {
		return fmt.Errorf("creating kueue manifest work: %w", err)
	}

	kyvernoMW := buildKyvernoManifestWork(cluster)
	if err := m.client.CreateIfNotExists(ctx, client.GVRManifestWork, cluster, kyvernoMW); err != nil {
		return fmt.Errorf("creating kyverno manifest work: %w", err)
	}

	healthPolicy := buildStackHealthPolicy(cluster)
	if err := m.client.CreateIfNotExists(ctx, client.GVRPolicy, DefaultNamespace, healthPolicy); err != nil {
		return fmt.Errorf("creating stack health policy: %w", err)
	}

	placement := buildStackPlacement(cluster, clusterSet)
	if err := m.client.CreateIfNotExists(ctx, client.GVRPlacement, DefaultNamespace, placement); err != nil {
		return fmt.Errorf("creating stack placement: %w", err)
	}

	binding := buildStackPlacementBinding(cluster)
	if err := m.client.CreateIfNotExists(ctx, client.GVRPlacementBinding, DefaultNamespace, binding); err != nil {
		return fmt.Errorf("creating stack placement binding: %w", err)
	}

	patch, _ := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				"gpu-sharing": "enabled",
			},
		},
	})
	if _, err := m.client.Patch(ctx, client.GVRManagedCluster, "", cluster, types.MergePatchType, patch); err != nil {
		return fmt.Errorf("labelling cluster gpu-sharing=enabled: %w", err)
	}

	return nil
}

func (m *Manager) RemoveStack(ctx context.Context, cluster string) error {
	m.logger.Info("gpu.RemoveStack", "cluster", cluster)

	_, err := m.client.Get(ctx, client.GVRManagedCluster, "", cluster)
	if err != nil {
		return fmt.Errorf("cluster %s not found: %w", cluster, err)
	}

	policyName := stackPolicyName(cluster)
	_ = m.client.DeleteIfExists(ctx, client.GVRManifestWork, cluster, cluster+"-kueue-stack")
	_ = m.client.DeleteIfExists(ctx, client.GVRManifestWork, cluster, cluster+"-kyverno-gpu")
	_ = m.client.DeleteIfExists(ctx, client.GVRManifestWork, cluster, cluster+"-gpu-queues")
	_ = m.client.DeleteIfExists(ctx, client.GVRPlacementBinding, DefaultNamespace, policyName+"-placement-binding")
	_ = m.client.DeleteIfExists(ctx, client.GVRPlacement, DefaultNamespace, policyName+"-placement")
	_ = m.client.DeleteIfExists(ctx, client.GVRPolicy, DefaultNamespace, policyName)

	patch, _ := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				"gpu-sharing": nil,
			},
		},
	})
	_, _ = m.client.Patch(ctx, client.GVRManagedCluster, "", cluster, types.MergePatchType, patch)

	return nil
}

func (m *Manager) DetectDrift(ctx context.Context, cluster string) (*DriftStatus, error) {
	m.logger.Info("gpu.DetectDrift", "cluster", cluster)

	policyName := stackPolicyName(cluster)
	policy, err := m.client.Get(ctx, client.GVRPolicy, DefaultNamespace, policyName)
	if err != nil {
		return nil, fmt.Errorf("getting stack health policy: %w", err)
	}

	return parseDriftStatus(cluster, policy), nil
}

func (m *Manager) CreateClusterQueues(ctx context.Context, cluster string, gpuTypes []string) error {
	if len(gpuTypes) == 0 {
		return fmt.Errorf("at least one GPU type is required")
	}
	m.logger.Info("gpu.CreateClusterQueues", "cluster", cluster, "gpuTypes", gpuTypes)

	mw := buildClusterQueueManifestWork(cluster, gpuTypes)
	if err := m.client.CreateIfNotExists(ctx, client.GVRManifestWork, cluster, mw); err != nil {
		return fmt.Errorf("creating cluster queue manifest work: %w", err)
	}

	return nil
}

func stackPolicyName(cluster string) string {
	return cluster + "-gpu-stack-health"
}

func parseDriftStatus(cluster string, policy *unstructured.Unstructured) *DriftStatus {
	status := &DriftStatus{Cluster: cluster}

	compliant, _, _ := unstructured.NestedString(policy.Object, "status", "compliant")
	status.Compliant = compliant == "Compliant"

	details, _, _ := unstructured.NestedSlice(policy.Object, "status", "details")
	for _, d := range details {
		dm, ok := d.(map[string]interface{})
		if !ok {
			continue
		}
		c, _, _ := unstructured.NestedString(dm, "compliant")
		name, _, _ := unstructured.NestedString(dm, "templateMeta", "name")
		if c != "Compliant" && name != "" {
			status.Degraded = append(status.Degraded, name)
		}
	}

	conditions, _, _ := unstructured.NestedSlice(policy.Object, "status", "conditions")
	for _, c := range conditions {
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		msg, _, _ := unstructured.NestedString(cm, "message")
		if msg != "" {
			status.Conditions = append(status.Conditions, msg)
		}
	}

	return status
}
