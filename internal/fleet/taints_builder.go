package fleet

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

func buildTolerantPlacement(name, namespace string, tolerations []Taint, clusterSets []string) *unstructured.Unstructured {
	tolList := make([]interface{}, len(tolerations))
	for i, t := range tolerations {
		tol := map[string]interface{}{
			"key":      t.Key,
			"operator": "Equal",
			"value":    t.Value,
		}
		if t.Effect != "" {
			tol["effect"] = t.Effect
		}
		tolList[i] = tol
	}

	spec := map[string]interface{}{
		"tolerations": tolList,
	}

	if len(clusterSets) > 0 {
		sets := make([]interface{}, len(clusterSets))
		for i, s := range clusterSets {
			sets[i] = s
		}
		spec["clusterSets"] = sets
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta1",
			"kind":       "Placement",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/tolerant-placement": "true",
				},
			},
			"spec": spec,
		},
	}
}
