package client

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type ClusterType string

const (
	ClusterTypeOCP        ClusterType = "OCP"
	ClusterTypeKubernetes ClusterType = "kubernetes"
	ClusterTypeUnknown    ClusterType = ""
)

// GetClusterType reads distributionInfo.type from ManagedClusterInfo.
func (c *Client) GetClusterType(ctx context.Context, clusterName string) (ClusterType, error) {
	info, err := c.Get(ctx, GVRManagedClusterInfo, clusterName, clusterName)
	if err != nil {
		return ClusterTypeUnknown, fmt.Errorf("getting ManagedClusterInfo for %s: %w", clusterName, err)
	}
	t, _, _ := unstructured.NestedString(info.Object, "status", "distributionInfo", "type")
	return ClusterType(t), nil
}
