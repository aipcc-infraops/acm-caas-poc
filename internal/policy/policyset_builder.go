package policy

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

func buildPolicySet(name, namespace, description string, policies []string) *unstructured.Unstructured {
	policyList := make([]interface{}, len(policies))
	for i, p := range policies {
		policyList[i] = p
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1beta1",
			"kind":       "PolicySet",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"description": description,
				"policies":    policyList,
			},
		},
	}
}

func buildPolicySetPlacement(name, namespace, clusterSet string) *unstructured.Unstructured {
	spec := map[string]interface{}{
		"tolerations": []interface{}{
			map[string]interface{}{
				"key":      "cluster.open-cluster-management.io/unreachable",
				"operator": "Exists",
			},
		},
	}
	if clusterSet != "" {
		spec["clusterSets"] = []interface{}{clusterSet}
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta1",
			"kind":       "Placement",
			"metadata": map[string]interface{}{
				"name":      name + "-policyset-placement",
				"namespace": namespace,
			},
			"spec": spec,
		},
	}
}

func buildPolicySetBinding(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1",
			"kind":       "PlacementBinding",
			"metadata": map[string]interface{}{
				"name":      name + "-policyset-binding",
				"namespace": namespace,
			},
			"placementRef": map[string]interface{}{
				"apiGroup": "cluster.open-cluster-management.io",
				"kind":     "Placement",
				"name":     name + "-policyset-placement",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"apiGroup": "policy.open-cluster-management.io",
					"kind":     "PolicySet",
					"name":     name,
				},
			},
		},
	}
}
