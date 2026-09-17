package cost

import "fmt"

func buildCostCenterPatch(costCenter string) map[string]interface{} {
	return map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				"caas/cost-center": costCenter,
			},
		},
	}
}

func buildBudgetPolicy(name, ns, costCenter string, limit float64) map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "policy.open-cluster-management.io/v1",
		"kind":       "Policy",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": ns,
			"labels": map[string]interface{}{
				"caas/cost-center":  costCenter,
				"caas/budget-limit": fmt.Sprintf("%.0f", limit),
			},
			"annotations": map[string]interface{}{
				"policy.open-cluster-management.io/categories": "CM Configuration Management",
				"policy.open-cluster-management.io/standards":  "CaaS Budget Control",
			},
		},
		"spec": map[string]interface{}{
			"remediationAction": "inform",
			"disabled":          false,
			"policy-templates": []interface{}{
				map[string]interface{}{
					"objectDefinition": map[string]interface{}{
						"apiVersion": "policy.open-cluster-management.io/v1",
						"kind":       "ConfigurationPolicy",
						"metadata":   map[string]interface{}{"name": name + "-config"},
						"spec": map[string]interface{}{
							"remediationAction": "inform",
							"severity":          "high",
							"namespaceSelector": map[string]interface{}{
								"include": []interface{}{"*"},
							},
							"object-templates": []interface{}{
								map[string]interface{}{
									"complianceType": "musthave",
									"objectDefinition": map[string]interface{}{
										"apiVersion": "v1",
										"kind":       "ConfigMap",
										"metadata": map[string]interface{}{
											"name":      "budget-tracker",
											"namespace": "caas-system",
										},
										"data": map[string]interface{}{
											"cost-center":  costCenter,
											"budget-limit": fmt.Sprintf("%.2f", limit),
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

func buildBudgetPlacement(policyName, ns, costCenter string) map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "cluster.open-cluster-management.io/v1beta1",
		"kind":       "Placement",
		"metadata": map[string]interface{}{
			"name":      policyName + "-placement",
			"namespace": ns,
		},
		"spec": map[string]interface{}{
			"predicates": []interface{}{
				map[string]interface{}{
					"requiredClusterSelector": map[string]interface{}{
						"labelSelector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"caas/cost-center": costCenter,
							},
						},
					},
				},
			},
		},
	}
}

func buildBudgetPlacementBinding(policyName, ns string) map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "policy.open-cluster-management.io/v1",
		"kind":       "PlacementBinding",
		"metadata": map[string]interface{}{
			"name":      policyName + "-placement-binding",
			"namespace": ns,
		},
		"placementRef": map[string]interface{}{
			"name":     policyName + "-placement",
			"apiGroup": "cluster.open-cluster-management.io",
			"kind":     "Placement",
		},
		"subjects": []interface{}{
			map[string]interface{}{
				"name":     policyName,
				"apiGroup": "policy.open-cluster-management.io",
				"kind":     "Policy",
			},
		},
	}
}

func groupByCostCenter(attributions []CostCenterAttribution, costs map[string]ClusterCost, days int) []CenterCost {
	groups := map[string]*CenterCost{}
	for _, attr := range attributions {
		cc := attr.CostCenter
		if _, ok := groups[cc]; !ok {
			groups[cc] = &CenterCost{CostCenter: cc, Days: days}
		}
		if cost, ok := costs[attr.Cluster]; ok {
			groups[cc].Clusters = append(groups[cc].Clusters, cost)
			groups[cc].TotalEstimate += cost.PeriodEstimate
		}
	}

	result := make([]CenterCost, 0, len(groups))
	for _, g := range groups {
		result = append(result, *g)
	}
	return result
}

func findOverBudget(centers []CenterCost, budgets map[string]float64) []BudgetAlert {
	var alerts []BudgetAlert
	for _, c := range centers {
		limit, ok := budgets[c.CostCenter]
		if !ok {
			continue
		}
		if c.TotalEstimate > limit {
			alerts = append(alerts, BudgetAlert{
				CostCenter:    c.CostCenter,
				TotalEstimate: c.TotalEstimate,
				BudgetLimit:   limit,
				Overage:       c.TotalEstimate - limit,
			})
		}
	}
	return alerts
}

func parseCostCenterAttribution(labels map[string]string) string {
	return labels["caas/cost-center"]
}
