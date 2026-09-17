package gpu

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func buildElasticLabels(gpuType, costTier string) map[string]interface{} {
	return map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				"gpu-type":      gpuType,
				"gpu-cost-tier": costTier,
				"gpu-available": "true",
				"gpu-elastic":   "true",
			},
		},
	}
}

func buildHibernateLabels() map[string]interface{} {
	return map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				"gpu-available":     "false",
				"gpu-elastic-state": "hibernated",
			},
		},
	}
}

func buildElasticManifestWork(cluster, gpuType string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      "gpu-elastic-stack-" + cluster,
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/gpu-elastic": "true",
					"gpu-type":                      gpuType,
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{
						map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "ConfigMap",
							"metadata": map[string]interface{}{
								"name":      "gpu-elastic-marker",
								"namespace": "kube-system",
							},
							"data": map[string]interface{}{
								"gpu-type":  gpuType,
								"cost-tier": "on-demand",
								"elastic":   "true",
							},
						},
					},
				},
			},
		},
	}
}

func parseElasticCluster(obj map[string]interface{}) ElasticCluster {
	metadata, _ := obj["metadata"].(map[string]interface{})
	name, _ := metadata["name"].(string)

	labels := make(map[string]string)
	if rawLabels, ok := metadata["labels"].(map[string]interface{}); ok {
		for k, v := range rawLabels {
			labels[k], _ = v.(string)
		}
	}

	return ElasticCluster{
		Name:      name,
		GPUType:   labels["gpu-type"],
		CostTier:  labels["gpu-cost-tier"],
		Available: labels["gpu-available"] == "true",
	}
}
