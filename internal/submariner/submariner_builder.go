package submariner

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func buildSubmarinerAddOn(cluster string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "addon.open-cluster-management.io/v1alpha1",
			"kind":       "ManagedClusterAddOn",
			"metadata": map[string]interface{}{
				"name":      "submariner",
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"installNamespace": "submariner-operator",
			},
		},
	}
}

func buildSubmarinerConfig(cluster string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "submarineraddon.open-cluster-management.io/v1alpha1",
			"kind":       "SubmarinerConfig",
			"metadata": map[string]interface{}{
				"name":      "submariner",
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"IPSecNATTPort":    int64(4500),
				"NATTEnable":       true,
				"cableDriver":     "libreswan",
				"gatewayConfig":   map[string]interface{}{"gateways": int64(1)},
				"credentialsSecret": map[string]interface{}{"name": cluster + "-submariner-creds"},
			},
		},
	}
}

func parseClusterStatus(cluster string, obj map[string]interface{}) ClusterStatus {
	cs := ClusterStatus{Name: cluster}

	status, ok := obj["status"].(map[string]interface{})
	if !ok {
		return cs
	}

	conditions, ok := status["conditions"].([]interface{})
	if !ok {
		return cs
	}

	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := cond["type"].(string)
		condStatus, _ := cond["status"].(string)

		switch condType {
		case "SubmarinerGatewayNodesLabeled":
			cs.GatewayReady = condStatus == "True"
		case "SubmarinerAgentDegraded":
			cs.AgentReady = condStatus != "True"
		case "SubmarinerConnectionsEstablished":
			cs.AgentReady = true
			if condStatus == "True" {
				cs.Connections = 1
			}
		}
	}
	return cs
}

func groupByClusterSet(clusters []map[string]interface{}) []SubmarinerInfo {
	sets := map[string]*SubmarinerInfo{}
	var order []string

	for _, obj := range clusters {
		meta, _ := obj["metadata"].(map[string]interface{})
		labels, _ := meta["labels"].(map[string]interface{})
		setName, _ := labels["cluster.open-cluster-management.io/clusterset"].(string)
		if setName == "" {
			setName = "unassigned"
		}

		info, exists := sets[setName]
		if !exists {
			info = &SubmarinerInfo{ClusterSet: setName, Enabled: true}
			sets[setName] = info
			order = append(order, setName)
		}
		info.Clusters++
	}

	result := make([]SubmarinerInfo, len(order))
	for i, name := range order {
		result[i] = *sets[name]
	}
	return result
}
