package security

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func buildComplianceOperatorPolicy(cluster string) *unstructured.Unstructured {
	name := complianceOperatorPolicyName(cluster)
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1beta1",
			"kind":       "OperatorPolicy",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": DefaultNamespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":             "true",
					"acmlab.redhat.com/compliance-operator": "true",
				},
			},
			"spec": map[string]interface{}{
				"remediationAction": "enforce",
				"severity":          "high",
				"complianceType":    "musthave",
				"subscription": map[string]interface{}{
					"channel":         "stable",
					"name":            "compliance-operator",
					"namespace":       "openshift-compliance",
					"source":          "redhat-operators",
					"sourceNamespace": "openshift-marketplace",
				},
			},
		},
	}
}

func buildComplianceOperatorHealthPolicy(cluster string) *unstructured.Unstructured {
	name := complianceOperatorPolicyName(cluster)
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1",
			"kind":       "Policy",
			"metadata": map[string]interface{}{
				"name":      name + "-health",
				"namespace": DefaultNamespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":             "true",
					"acmlab.redhat.com/compliance-operator": "true",
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
								"name": name + "-health-config",
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
												"name":      "compliance-operator",
												"namespace": "openshift-compliance",
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

func buildCompliancePlacement(cluster, clusterSet string) *unstructured.Unstructured {
	name := complianceOperatorPolicyName(cluster)
	spec := map[string]interface{}{
		"predicates": []interface{}{
			map[string]interface{}{
				"requiredClusterSelector": map[string]interface{}{
					"labelSelector": map[string]interface{}{
						"matchExpressions": []interface{}{
							map[string]interface{}{
								"key":      "name",
								"operator": "In",
								"values":   []interface{}{cluster},
							},
						},
					},
				},
			},
		},
		"tolerations": []interface{}{
			map[string]interface{}{
				"key":      "cluster.open-cluster-management.io/unreachable",
				"operator": "Exists",
			},
			map[string]interface{}{
				"key":      "cluster.open-cluster-management.io/unavailable",
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
				"name":      name + "-placement",
				"namespace": DefaultNamespace,
			},
			"spec": spec,
		},
	}
}

func buildCompliancePlacementBinding(cluster string) *unstructured.Unstructured {
	name := complianceOperatorPolicyName(cluster)
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1",
			"kind":       "PlacementBinding",
			"metadata": map[string]interface{}{
				"name":      name + "-placement-binding",
				"namespace": DefaultNamespace,
			},
			"placementRef": map[string]interface{}{
				"apiGroup": "cluster.open-cluster-management.io",
				"kind":     "Placement",
				"name":     name + "-placement",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"apiGroup": "policy.open-cluster-management.io",
					"kind":     "Policy",
					"name":     name + "-health",
				},
			},
		},
	}
}

func buildComplianceScanPolicy(opts ComplianceScanOpts) *unstructured.Unstructured {
	name := complianceScanPolicyName(opts.Cluster)
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1",
			"kind":       "Policy",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": DefaultNamespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":          "true",
					"acmlab.redhat.com/compliance-scan":  "true",
					"acmlab.redhat.com/compliance-profile": opts.Profile,
				},
			},
			"spec": map[string]interface{}{
				"disabled":          false,
				"remediationAction": "enforce",
				"policy-templates": []interface{}{
					map[string]interface{}{
						"objectDefinition": map[string]interface{}{
							"apiVersion": "policy.open-cluster-management.io/v1",
							"kind":       "ConfigurationPolicy",
							"metadata": map[string]interface{}{
								"name": name + "-config",
							},
							"spec": map[string]interface{}{
								"remediationAction":   "enforce",
								"severity":            "high",
								"pruneObjectBehavior": "DeleteIfCreated",
								"object-templates": []interface{}{
									map[string]interface{}{
										"complianceType": "musthave",
										"objectDefinition": map[string]interface{}{
											"apiVersion": "compliance.openshift.io/v1alpha1",
											"kind":       "ScanSettingBinding",
											"metadata": map[string]interface{}{
												"name":      fmt.Sprintf("%s-scan", opts.Profile),
												"namespace": "openshift-compliance",
											},
											"profiles": []interface{}{
												map[string]interface{}{
													"apiGroup": "compliance.openshift.io/v1alpha1",
													"kind":     "Profile",
													"name":     opts.Profile,
												},
											},
											"settingsRef": map[string]interface{}{
												"apiGroup": "compliance.openshift.io/v1alpha1",
												"kind":     "ScanSetting",
												"name":     "default",
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

func buildComplianceScanPlacement(cluster, clusterSet string) *unstructured.Unstructured {
	name := complianceScanPolicyName(cluster)
	spec := map[string]interface{}{
		"predicates": []interface{}{
			map[string]interface{}{
				"requiredClusterSelector": map[string]interface{}{
					"labelSelector": map[string]interface{}{
						"matchExpressions": []interface{}{
							map[string]interface{}{
								"key":      "name",
								"operator": "In",
								"values":   []interface{}{cluster},
							},
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
				"name":      name + "-placement",
				"namespace": DefaultNamespace,
			},
			"spec": spec,
		},
	}
}

func buildComplianceScanPlacementBinding(cluster string) *unstructured.Unstructured {
	name := complianceScanPolicyName(cluster)
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1",
			"kind":       "PlacementBinding",
			"metadata": map[string]interface{}{
				"name":      name + "-placement-binding",
				"namespace": DefaultNamespace,
			},
			"placementRef": map[string]interface{}{
				"apiGroup": "cluster.open-cluster-management.io",
				"kind":     "Placement",
				"name":     name + "-placement",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"apiGroup": "policy.open-cluster-management.io",
					"kind":     "Policy",
					"name":     name,
				},
			},
		},
	}
}

func parseComplianceScanStatus(cluster string, obj map[string]interface{}) *ComplianceScanStatus {
	status := &ComplianceScanStatus{
		Cluster: cluster,
		Phase:   "Pending",
	}

	if labels, ok := nestedMap(obj, "metadata", "labels"); ok {
		status.Profile, _ = labels["acmlab.redhat.com/compliance-profile"].(string)
	}

	statusMap, _ := obj["status"].(map[string]interface{})
	if statusMap == nil {
		return status
	}

	compliant, _ := statusMap["compliant"].(string)
	switch compliant {
	case "Compliant":
		status.Phase = "Done"
	case "NonCompliant":
		status.Phase = "Done"
	case "Pending":
		status.Phase = "Running"
	}

	details, _ := statusMap["details"].([]interface{})
	for _, raw := range details {
		detail, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		s, _ := detail["status"].(string)
		switch s {
		case "PASS":
			status.Compliant++
		case "FAIL":
			status.NonCompliant++
		}
	}

	conditions, _ := statusMap["conditions"].([]interface{})
	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := cond["type"].(string)
		condStatus, _ := cond["status"].(string)
		status.Conditions = append(status.Conditions, fmt.Sprintf("%s=%s", condType, condStatus))
	}

	return status
}
