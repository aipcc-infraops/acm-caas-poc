package gpu

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func buildGPUPlacement(name, gpuType, region string) *unstructured.Unstructured {
	matchExpressions := []interface{}{
		map[string]interface{}{
			"key":      "gpu-type",
			"operator": "In",
			"values":   []interface{}{gpuType},
		},
		map[string]interface{}{
			"key":      "gpu-available",
			"operator": "In",
			"values":   []interface{}{"true"},
		},
	}

	if region != "" {
		matchExpressions = append(matchExpressions, map[string]interface{}{
			"key":      "region",
			"operator": "In",
			"values":   []interface{}{region},
		})
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta1",
			"kind":       "Placement",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": DefaultNamespace,
			},
			"spec": map[string]interface{}{
				"predicates": []interface{}{
					map[string]interface{}{
						"requiredClusterSelector": map[string]interface{}{
							"labelSelector": map[string]interface{}{
								"matchExpressions": matchExpressions,
							},
						},
					},
				},
			},
		},
	}
}

func parsePlacementDecisions(list *unstructured.UnstructuredList) []PlacementResult {
	var results []PlacementResult
	for _, item := range list.Items {
		decisions, found, _ := unstructured.NestedSlice(item.Object, "status", "decisions")
		if !found {
			continue
		}
		for _, d := range decisions {
			dm, ok := d.(map[string]interface{})
			if !ok {
				continue
			}
			name, _, _ := unstructured.NestedString(dm, "clusterName")
			if name == "" {
				continue
			}
			results = append(results, PlacementResult{ClusterName: name})
		}
	}
	return results
}
