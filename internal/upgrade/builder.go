package upgrade

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

func buildChannelManifestWork(clusterName, mwName, channel string) *unstructured.Unstructured {
	cvPatch := map[string]interface{}{
		"apiVersion": "config.openshift.io/v1",
		"kind":       "ClusterVersion",
		"metadata": map[string]interface{}{
			"name": "version",
		},
		"spec": map[string]interface{}{
			"channel": channel,
		},
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      mwName,
				"namespace": clusterName,
				"labels": map[string]interface{}{
					"caas-poc/operation": "upgrade",
					"caas-poc/cluster":   clusterName,
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{cvPatch},
				},
				"manifestConfigs": []interface{}{
					map[string]interface{}{
						"resourceIdentifier": map[string]interface{}{
							"group":     "config.openshift.io",
							"resource":  "clusterversions",
							"name":      "version",
							"namespace": "",
						},
						"updateStrategy": map[string]interface{}{
							"type": "ServerSideApply",
						},
					},
				},
			},
		},
	}
}

func buildUpgradeManifestWork(clusterName, mwName, version string) *unstructured.Unstructured {
	cvPatch := map[string]interface{}{
		"apiVersion": "config.openshift.io/v1",
		"kind":       "ClusterVersion",
		"metadata": map[string]interface{}{
			"name": "version",
		},
		"spec": map[string]interface{}{
			"desiredUpdate": map[string]interface{}{
				"version": version,
			},
		},
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      mwName,
				"namespace": clusterName,
				"labels": map[string]interface{}{
					"caas-poc/operation": "upgrade",
					"caas-poc/cluster":   clusterName,
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{cvPatch},
				},
				"manifestConfigs": []interface{}{
					map[string]interface{}{
						"resourceIdentifier": map[string]interface{}{
							"group":     "config.openshift.io",
							"resource":  "clusterversions",
							"name":      "version",
							"namespace": "",
						},
						"updateStrategy": map[string]interface{}{
							"type": "ServerSideApply",
						},
					},
				},
			},
		},
	}
}
