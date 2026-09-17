package rightsizing

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func buildRightsizingManifestWork(cluster string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      rightsizingMWName(cluster),
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":      "true",
					"acmlab.redhat.com/rightsizing":   "true",
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{
						buildMCOAPrometheusRule(),
						buildResourceRecommendationConfigMap(cluster),
					},
				},
			},
		},
	}
}

func buildMCOAPrometheusRule() map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "monitoring.coreos.com/v1",
		"kind":       "PrometheusRule",
		"metadata": map[string]interface{}{
			"name":      "acmlab-rightsizing-rules",
			"namespace": "openshift-monitoring",
			"labels": map[string]interface{}{
				"acmlab.redhat.com/managed": "true",
			},
		},
		"spec": map[string]interface{}{
			"groups": []interface{}{
				map[string]interface{}{
					"name":     "acmlab.rightsizing",
					"interval": "5m",
					"rules": []interface{}{
						map[string]interface{}{
							"record": "acmlab:container_cpu_usage_p95",
							"expr":   "quantile_over_time(0.95, rate(container_cpu_usage_seconds_total{container!=\"\"}[5m])[24h:5m])",
						},
						map[string]interface{}{
							"record": "acmlab:container_memory_usage_p95",
							"expr":   "quantile_over_time(0.95, container_memory_working_set_bytes{container!=\"\"}[24h:5m])",
						},
						map[string]interface{}{
							"record": "acmlab:container_cpu_overprovisioned",
							"expr":   "kube_pod_container_resource_requests{resource=\"cpu\"} - acmlab:container_cpu_usage_p95 > 0.1",
						},
						map[string]interface{}{
							"record": "acmlab:container_memory_overprovisioned",
							"expr":   "kube_pod_container_resource_requests{resource=\"memory\"} - acmlab:container_memory_usage_p95 > 67108864",
						},
					},
				},
			},
		},
	}
}

func buildResourceRecommendationConfigMap(cluster string) map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      "acmlab-rightsizing-config",
			"namespace": "openshift-monitoring",
			"labels": map[string]interface{}{
				"acmlab.redhat.com/managed": "true",
			},
		},
		"data": map[string]interface{}{
			"cpu-headroom-percent":    "20",
			"memory-headroom-percent": "25",
			"min-cpu":                 "50m",
			"min-memory":             "64Mi",
			"evaluation-window":      "24h",
		},
	}
}

func buildAdjustManifestWork(cluster string, rec Recommendation) *unstructured.Unstructured {
	mwName := "rightsizing-adjust-" + rec.Workload + "-" + cluster

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      mwName,
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":           "true",
					"acmlab.redhat.com/rightsizing-adjust": "true",
					"acmlab.redhat.com/workload":          rec.Workload,
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{
						map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "ConfigMap",
							"metadata": map[string]interface{}{
								"name":      "rightsizing-patch-" + rec.Workload,
								"namespace": rec.Namespace,
							},
							"data": map[string]interface{}{
								"container":      rec.Container,
								"cpu-request":    rec.RecommendedCPU,
								"memory-request": rec.RecommendedMem,
							},
						},
					},
				},
			},
		},
	}
}
