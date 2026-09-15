package decommission

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

func (m *Manager) Audit(ctx context.Context, clusterName string) (*AuditReport, error) {
	m.logger.Info("decommission.Audit", "cluster", clusterName)
	mc, err := m.client.Get(ctx, client.GVRManagedCluster, "", clusterName)
	if err != nil {
		return nil, fmt.Errorf("getting ManagedCluster %s: %w", clusterName, err)
	}

	report := &AuditReport{}

	labels, _, _ := unstructured.NestedStringMap(mc.Object, "metadata", "labels")
	report.Owner = labels["caas-poc/owner"]
	report.Platform = labels["cloud"]

	creationStr, _, _ := unstructured.NestedString(mc.Object, "metadata", "creationTimestamp")
	if created, err := time.Parse(time.RFC3339, creationStr); err == nil {
		report.ClusterAge = fmt.Sprintf("%dd", int(time.Since(created).Hours()/24))
	}

	mci, err := m.client.Get(ctx, client.GVRManagedClusterInfo, clusterName, clusterName)
	if err == nil {
		nodeList, _, _ := unstructured.NestedSlice(mci.Object, "status", "nodeList")
		report.NodeCount = len(nodeList)

		totalCPU := 0
		totalMemory := 0
		memUnit := ""
		for _, node := range nodeList {
			nodeMap, ok := node.(map[string]interface{})
			if !ok {
				continue
			}
			capacity, _, _ := unstructured.NestedStringMap(nodeMap, "capacity")
			if cpuStr, ok := capacity["cpu"]; ok {
				if cpu, err := strconv.Atoi(cpuStr); err == nil {
					totalCPU += cpu
				}
			}
			if memStr, ok := capacity["memory"]; ok {
				mem, unit := parseMemory(memStr)
				totalMemory += mem
				memUnit = unit
			}
		}
		report.CPUCapacity = strconv.Itoa(totalCPU)
		report.MemoryCapacity = fmt.Sprintf("%d%s", totalMemory, memUnit)
	}

	if report.Platform == "" {
		report.Platform = detectPlatformFromCD(ctx, m.client, clusterName)
	}

	return report, nil
}

func detectPlatformFromCD(ctx context.Context, c *client.Client, name string) string {
	cd, err := c.Get(ctx, client.GVRClusterDeployment, name, name)
	if err != nil {
		return "unknown"
	}
	platform, _, _ := unstructured.NestedMap(cd.Object, "spec", "platform")
	for key := range platform {
		return key
	}
	return "unknown"
}

func parseMemory(s string) (int, string) {
	for i, c := range s {
		if c < '0' || c > '9' {
			val, _ := strconv.Atoi(s[:i])
			return val, s[i:]
		}
	}
	val, _ := strconv.Atoi(s)
	return val, ""
}
