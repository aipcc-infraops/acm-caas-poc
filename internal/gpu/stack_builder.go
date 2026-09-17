package gpu

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func buildKueueManifestWork(cluster string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      cluster + "-kueue-stack",
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":   "true",
					"acmlab.redhat.com/gpu-stack": "kueue",
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{
						map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "Namespace",
							"metadata": map[string]interface{}{
								"name": "kueue-system",
							},
						},
						map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Deployment",
							"metadata": map[string]interface{}{
								"name":      "kueue-controller-manager",
								"namespace": "kueue-system",
							},
							"spec": map[string]interface{}{
								"replicas": int64(1),
								"selector": map[string]interface{}{
									"matchLabels": map[string]interface{}{
										"control-plane": "controller-manager",
									},
								},
								"template": map[string]interface{}{
									"metadata": map[string]interface{}{
										"labels": map[string]interface{}{
											"control-plane": "controller-manager",
										},
									},
									"spec": map[string]interface{}{
										"containers": []interface{}{
											map[string]interface{}{
												"name":  "manager",
												"image": "registry.k8s.io/kueue/kueue:v0.9.1",
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

func buildKyvernoManifestWork(cluster string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      cluster + "-kyverno-gpu",
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":   "true",
					"acmlab.redhat.com/gpu-stack": "kyverno",
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{
						map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "Namespace",
							"metadata": map[string]interface{}{
								"name": "kyverno",
							},
						},
						map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Deployment",
							"metadata": map[string]interface{}{
								"name":      "kyverno-admission-controller",
								"namespace": "kyverno",
							},
							"spec": map[string]interface{}{
								"replicas": int64(1),
								"selector": map[string]interface{}{
									"matchLabels": map[string]interface{}{
										"app.kubernetes.io/name": "kyverno",
									},
								},
								"template": map[string]interface{}{
									"metadata": map[string]interface{}{
										"labels": map[string]interface{}{
											"app.kubernetes.io/name": "kyverno",
										},
									},
									"spec": map[string]interface{}{
										"containers": []interface{}{
											map[string]interface{}{
												"name":  "kyverno",
												"image": "ghcr.io/kyverno/kyverno:v1.12.0",
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

func buildStackHealthPolicy(cluster string) *unstructured.Unstructured {
	policyName := stackPolicyName(cluster)
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1",
			"kind":       "Policy",
			"metadata": map[string]interface{}{
				"name":      policyName,
				"namespace": DefaultNamespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":   "true",
					"acmlab.redhat.com/gpu-stack": "health",
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
								"name": policyName + "-kueue",
							},
							"spec": map[string]interface{}{
								"remediationAction":   "inform",
								"severity":            "high",
								"pruneObjectBehavior": "None",
								"object-templates": []interface{}{
									map[string]interface{}{
										"complianceType": "musthave",
										"objectDefinition": map[string]interface{}{
											"apiVersion": "apps/v1",
											"kind":       "Deployment",
											"metadata": map[string]interface{}{
												"name":      "kueue-controller-manager",
												"namespace": "kueue-system",
											},
											"status": map[string]interface{}{
												"readyReplicas": int64(1),
											},
										},
									},
								},
							},
						},
					},
					map[string]interface{}{
						"objectDefinition": map[string]interface{}{
							"apiVersion": "policy.open-cluster-management.io/v1",
							"kind":       "ConfigurationPolicy",
							"metadata": map[string]interface{}{
								"name": policyName + "-kyverno",
							},
							"spec": map[string]interface{}{
								"remediationAction":   "inform",
								"severity":            "high",
								"pruneObjectBehavior": "None",
								"object-templates": []interface{}{
									map[string]interface{}{
										"complianceType": "musthave",
										"objectDefinition": map[string]interface{}{
											"apiVersion": "apps/v1",
											"kind":       "Deployment",
											"metadata": map[string]interface{}{
												"name":      "kyverno-admission-controller",
												"namespace": "kyverno",
											},
											"status": map[string]interface{}{
												"readyReplicas": int64(1),
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

func buildStackPlacement(cluster, clusterSet string) *unstructured.Unstructured {
	policyName := stackPolicyName(cluster)
	spec := map[string]interface{}{
		"predicates": []interface{}{
			map[string]interface{}{
				"requiredClusterSelector": map[string]interface{}{
					"labelSelector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"gpu-sharing": "enabled",
						},
					},
				},
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
				"name":      policyName + "-placement",
				"namespace": DefaultNamespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":   "true",
					"acmlab.redhat.com/gpu-stack": "placement",
				},
			},
			"spec": spec,
		},
	}
}

func buildStackPlacementBinding(cluster string) *unstructured.Unstructured {
	policyName := stackPolicyName(cluster)
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1",
			"kind":       "PlacementBinding",
			"metadata": map[string]interface{}{
				"name":      policyName + "-placement-binding",
				"namespace": DefaultNamespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":   "true",
					"acmlab.redhat.com/gpu-stack": "binding",
				},
			},
			"placementRef": map[string]interface{}{
				"name":     policyName + "-placement",
				"kind":     "Placement",
				"apiGroup": "cluster.open-cluster-management.io",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"name":     policyName,
					"kind":     "Policy",
					"apiGroup": "policy.open-cluster-management.io",
				},
			},
		},
	}
}

func buildClusterQueueManifestWork(cluster string, gpuTypes []string) *unstructured.Unstructured {
	manifests := make([]interface{}, 0, len(gpuTypes)*2)
	for _, gpuType := range gpuTypes {
		manifests = append(manifests,
			map[string]interface{}{
				"apiVersion": "kueue.x-k8s.io/v1beta1",
				"kind":       "ResourceFlavor",
				"metadata": map[string]interface{}{
					"name": fmt.Sprintf("gpu-%s", gpuType),
				},
				"spec": map[string]interface{}{
					"nodeLabels": map[string]interface{}{
						"nvidia.com/gpu.product": gpuType,
					},
				},
			},
			map[string]interface{}{
				"apiVersion": "kueue.x-k8s.io/v1beta1",
				"kind":       "ClusterQueue",
				"metadata": map[string]interface{}{
					"name": fmt.Sprintf("gpu-%s-queue", gpuType),
				},
				"spec": map[string]interface{}{
					"namespaceSelector": map[string]interface{}{},
					"resourceGroups": []interface{}{
						map[string]interface{}{
							"coveredResources": []interface{}{"nvidia.com/gpu"},
							"flavors": []interface{}{
								map[string]interface{}{
									"name": fmt.Sprintf("gpu-%s", gpuType),
									"resources": []interface{}{
										map[string]interface{}{
											"name":         "nvidia.com/gpu",
											"nominalQuota": int64(8),
										},
									},
								},
							},
						},
					},
				},
			},
		)
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      cluster + "-gpu-queues",
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":   "true",
					"acmlab.redhat.com/gpu-stack": "queues",
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": manifests,
				},
			},
		},
	}
}
