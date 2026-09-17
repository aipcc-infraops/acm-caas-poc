package gpu

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func versionPolicyName(cluster string) string {
	return fmt.Sprintf("version-enforce-%s", cluster)
}

func buildVersionPlacement(cluster string) *unstructured.Unstructured {
	name := versionPolicyName(cluster)
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta1",
			"kind":       "Placement",
			"metadata": map[string]interface{}{
				"name":      name + "-placement",
				"namespace": DefaultNamespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":            "true",
					"acmlab.redhat.com/version-segregation": "true",
				},
			},
			"spec": map[string]interface{}{
				"predicates": []interface{}{
					map[string]interface{}{
						"requiredClusterSelector": map[string]interface{}{
							"labelSelector": map[string]interface{}{
								"matchLabels": map[string]interface{}{
									"name": cluster,
								},
							},
						},
					},
				},
			},
		},
	}
}

func buildVersionEnforcementPolicy(cluster, version string) *unstructured.Unstructured {
	name := versionPolicyName(cluster)
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1",
			"kind":       "ConfigurationPolicy",
			"metadata": map[string]interface{}{
				"name":      name + "-config",
				"namespace": DefaultNamespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":            "true",
					"acmlab.redhat.com/version-segregation": "true",
				},
			},
			"spec": map[string]interface{}{
				"remediationAction":   "inform",
				"severity":            "high",
				"pruneObjectBehavior": "None",
				"object-templates": []interface{}{
					map[string]interface{}{
						"complianceType": "musthave",
						"objectDefinition": map[string]interface{}{
							"apiVersion": "operators.coreos.com/v1alpha1",
							"kind":       "Subscription",
							"metadata": map[string]interface{}{
								"name":      "ai-platform-operator",
								"namespace": "ai-platform",
							},
							"spec": map[string]interface{}{
								"channel":        "stable",
								"startingCSV":    fmt.Sprintf("ai-platform-operator.v%s", version),
								"source":         "custom-operators",
								"sourceNamespace": "openshift-marketplace",
							},
						},
					},
				},
			},
		},
	}
}

func buildVersionPolicyWrapper(cluster string) *unstructured.Unstructured {
	name := versionPolicyName(cluster)
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1",
			"kind":       "Policy",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": DefaultNamespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":            "true",
					"acmlab.redhat.com/version-segregation": "true",
				},
			},
			"spec": map[string]interface{}{
				"disabled":          false,
				"remediationAction": "inform",
				"policy-templates": []interface{}{
					map[string]interface{}{
						"objectDefinition": map[string]interface{}{
							"apiVersion": "policy.open-cluster-management.io/v1",
							"kind":       "ConfigurationPolicy",
							"metadata": map[string]interface{}{
								"name": name + "-config",
							},
							"spec": map[string]interface{}{
								"remediationAction":   "inform",
								"severity":            "high",
								"pruneObjectBehavior": "None",
								"object-templates": []interface{}{
									map[string]interface{}{
										"complianceType": "musthave",
										"objectDefinition": map[string]interface{}{
											"apiVersion": "operators.coreos.com/v1alpha1",
											"kind":       "Subscription",
											"metadata": map[string]interface{}{
												"name":      "ai-platform-operator",
												"namespace": "ai-platform",
											},
											"spec": map[string]interface{}{
												"channel":        "stable",
												"startingCSV":    fmt.Sprintf("ai-platform-operator.v%s", cluster),
												"source":         "custom-operators",
												"sourceNamespace": "openshift-marketplace",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func buildVersionPlacementBinding(cluster string) *unstructured.Unstructured {
	name := versionPolicyName(cluster)
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1",
			"kind":       "PlacementBinding",
			"metadata": map[string]interface{}{
				"name":      name + "-binding",
				"namespace": DefaultNamespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":            "true",
					"acmlab.redhat.com/version-segregation": "true",
				},
			},
			"placementRef": map[string]interface{}{
				"name":     name + "-placement",
				"kind":     "Placement",
				"apiGroup": "cluster.open-cluster-management.io",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"name":     name,
					"kind":     "Policy",
					"apiGroup": "policy.open-cluster-management.io",
				},
			},
		},
	}
}

func buildOperatorManifestWork(cluster, version string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      fmt.Sprintf("ai-platform-operator-%s", version),
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":            "true",
					"acmlab.redhat.com/version-segregation": "true",
					"acmlab.redhat.com/operator-version":   version,
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{
						map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "Namespace",
							"metadata": map[string]interface{}{
								"name": "ai-platform",
							},
						},
						map[string]interface{}{
							"apiVersion": "operators.coreos.com/v1",
							"kind":       "OperatorGroup",
							"metadata": map[string]interface{}{
								"name":      "ai-platform-og",
								"namespace": "ai-platform",
							},
							"spec": map[string]interface{}{
								"targetNamespaces": []interface{}{
									"ai-platform",
								},
							},
						},
						map[string]interface{}{
							"apiVersion": "operators.coreos.com/v1alpha1",
							"kind":       "Subscription",
							"metadata": map[string]interface{}{
								"name":      "ai-platform-operator",
								"namespace": "ai-platform",
							},
							"spec": map[string]interface{}{
								"channel":         "stable",
								"name":            "ai-platform-operator",
								"source":          "custom-operators",
								"sourceNamespace": "openshift-marketplace",
								"startingCSV":     fmt.Sprintf("ai-platform-operator.v%s", version),
								"installPlanApproval": "Automatic",
							},
						},
					},
				},
			},
		},
	}
}

func parseVersionCluster(obj map[string]interface{}) VersionCluster {
	metadata, _ := obj["metadata"].(map[string]interface{})
	name, _ := metadata["name"].(string)
	labels, _ := metadata["labels"].(map[string]interface{})

	version, _ := labels["ai-platform-version"].(string)
	channel, _ := labels["ai-platform-channel"].(string)
	build, _ := labels["ai-platform-build"].(string)

	available := false
	if v, ok := labels["gpu-available"].(string); ok && v == "true" {
		available = true
	}

	return VersionCluster{
		Name:      name,
		Version:   version,
		Channel:   channel,
		Build:     build,
		Available: available,
	}
}
