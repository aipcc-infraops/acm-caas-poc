package fleet

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

func buildScoringPlacement(name, namespace string, prioritizers []string, clusterSet string, labels map[string]string) *unstructured.Unstructured {
	prioriList := make([]interface{}, len(prioritizers))
	for i, p := range prioritizers {
		prioriList[i] = map[string]interface{}{
			"scoreCoordinate": map[string]interface{}{
				"builtIn": p,
				"type":    "BuiltIn",
			},
			"weight": int64(1),
		}
	}

	spec := map[string]interface{}{
		"prioritizerPolicy": map[string]interface{}{
			"mode":          "Exact",
			"configurations": prioriList,
		},
	}

	if clusterSet != "" {
		spec["clusterSets"] = []interface{}{clusterSet}
	}

	if len(labels) > 0 {
		matchExpressions := make([]interface{}, 0, len(labels))
		for k, v := range labels {
			matchExpressions = append(matchExpressions, map[string]interface{}{
				"key":      k,
				"operator": "In",
				"values":   []interface{}{v},
			})
		}
		spec["predicates"] = []interface{}{
			map[string]interface{}{
				"requiredClusterSelector": map[string]interface{}{
					"labelSelector": map[string]interface{}{
						"matchExpressions": matchExpressions,
					},
				},
			},
		}
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta1",
			"kind":       "Placement",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/scoring": "true",
				},
			},
			"spec": spec,
		},
	}
}
