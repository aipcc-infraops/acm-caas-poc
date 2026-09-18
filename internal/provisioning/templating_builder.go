package provisioning

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

func buildClusterDeploymentCustomization(name string, patches []TemplatePatch) *unstructured.Unstructured {
	patchList := make([]interface{}, len(patches))
	for i, p := range patches {
		patchList[i] = map[string]interface{}{
			"op":    p.Op,
			"path":  p.Path,
			"value": p.Value,
		}
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeploymentCustomization",
			"metadata": map[string]interface{}{
				"name": name,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/cluster-template": "true",
				},
			},
			"spec": map[string]interface{}{
				"installConfigPatches": patchList,
			},
		},
	}
}
