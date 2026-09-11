package client

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ClusterType represents the distribution type of a managed cluster.
type ClusterType string

const (
	// ClusterTypeOCP is an OpenShift cluster (Hive-provisioned or imported).
	ClusterTypeOCP ClusterType = "OCP"
	// ClusterTypeKubernetes is a vanilla Kubernetes cluster (CAPI, EKS, AKS, GKE, IKS).
	ClusterTypeKubernetes ClusterType = "kubernetes"
	// ClusterTypeUnknown means the cluster has not yet reported its distribution.
	ClusterTypeUnknown ClusterType = ""
)

// GetClusterType reads the cluster distribution type from ManagedClusterInfo.
// ACM sets distributionInfo.type automatically when a cluster joins the hub.
// Returns ClusterTypeUnknown if the cluster has not yet reported its distribution.
func (c *Client) GetClusterType(ctx context.Context, clusterName string) (ClusterType, error) {
	info, err := c.Get(ctx, GVRManagedClusterInfo, clusterName, clusterName)
	if err != nil {
		return ClusterTypeUnknown, fmt.Errorf("getting ManagedClusterInfo for %s: %w", clusterName, err)
	}
	t, _, _ := unstructured.NestedString(info.Object, "status", "distributionInfo", "type")
	return ClusterType(t), nil
}
