package security

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var defaultAllowedRepos = []string{
	"registry.redhat.io",
	"quay.io",
	"registry.access.redhat.com",
}

func buildGatekeeperManifestWork(cluster, level string) *unstructured.Unstructured {
	manifests := []interface{}{
		map[string]interface{}{
			"apiVersion": "templates.gatekeeper.sh/v1",
			"kind":       "ConstraintTemplate",
			"metadata": map[string]interface{}{
				"name": "k8spspprivilegedcontainer",
			},
			"spec": map[string]interface{}{
				"crd": map[string]interface{}{
					"spec": map[string]interface{}{
						"names": map[string]interface{}{
							"kind": "K8sPSPPrivilegedContainer",
						},
					},
				},
				"targets": []interface{}{
					map[string]interface{}{
						"target": "admission.k8s.gatekeeper.sh",
						"rego":   "package k8spspprivilegedcontainer\nviolation[{\"msg\": msg}] {\n  c := input.review.object.spec.containers[_]\n  c.securityContext.privileged\n  msg := sprintf(\"Privileged container not allowed: %v\", [c.name])\n}",
					},
				},
			},
		},
		map[string]interface{}{
			"apiVersion": "templates.gatekeeper.sh/v1",
			"kind":       "ConstraintTemplate",
			"metadata": map[string]interface{}{
				"name": "k8spsphostnamespace",
			},
			"spec": map[string]interface{}{
				"crd": map[string]interface{}{
					"spec": map[string]interface{}{
						"names": map[string]interface{}{
							"kind": "K8sPSPHostNamespace",
						},
					},
				},
				"targets": []interface{}{
					map[string]interface{}{
						"target": "admission.k8s.gatekeeper.sh",
						"rego":   "package k8spsphostnamespace\nviolation[{\"msg\": msg}] {\n  input.review.object.spec.hostPID\n  msg := \"hostPID not allowed\"\n}\nviolation[{\"msg\": msg}] {\n  input.review.object.spec.hostNetwork\n  msg := \"hostNetwork not allowed\"\n}",
					},
				},
			},
		},
		map[string]interface{}{
			"apiVersion": "templates.gatekeeper.sh/v1",
			"kind":       "ConstraintTemplate",
			"metadata": map[string]interface{}{
				"name": "k8srequiredresources",
			},
			"spec": map[string]interface{}{
				"crd": map[string]interface{}{
					"spec": map[string]interface{}{
						"names": map[string]interface{}{
							"kind": "K8sRequiredResources",
						},
					},
				},
				"targets": []interface{}{
					map[string]interface{}{
						"target": "admission.k8s.gatekeeper.sh",
						"rego":   "package k8srequiredresources\nviolation[{\"msg\": msg}] {\n  c := input.review.object.spec.containers[_]\n  not c.resources.limits\n  msg := sprintf(\"Container %v has no resource limits\", [c.name])\n}",
					},
				},
			},
		},
		map[string]interface{}{
			"apiVersion": "templates.gatekeeper.sh/v1",
			"kind":       "ConstraintTemplate",
			"metadata": map[string]interface{}{
				"name": "k8sallowedrepos",
			},
			"spec": map[string]interface{}{
				"crd": map[string]interface{}{
					"spec": map[string]interface{}{
						"names": map[string]interface{}{
							"kind": "K8sAllowedRepos",
						},
						"validation": map[string]interface{}{
							"openAPIV3Schema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"repos": map[string]interface{}{
										"type": "array",
										"items": map[string]interface{}{
											"type": "string",
										},
									},
								},
							},
						},
					},
				},
				"targets": []interface{}{
					map[string]interface{}{
						"target": "admission.k8s.gatekeeper.sh",
						"rego":   "package k8sallowedrepos\nviolation[{\"msg\": msg}] {\n  c := input.review.object.spec.containers[_]\n  image := c.image\n  not startswith_any(image, input.parameters.repos)\n  msg := sprintf(\"Image %v not from allowed repos\", [image])\n}\nstartswith_any(str, prefixes) {\n  prefix := prefixes[_]\n  startswith(str, prefix)\n}",
					},
				},
			},
		},
	}

	constraints := buildConstraints(defaultAllowedRepos)
	for _, c := range constraints {
		manifests = append(manifests, c)
	}

	wrappedManifests := make([]interface{}, len(manifests))
	for i, m := range manifests {
		wrappedManifests[i] = map[string]interface{}{
			"apiVersion": m.(map[string]interface{})["apiVersion"],
			"kind":       m.(map[string]interface{})["kind"],
			"metadata":   m.(map[string]interface{})["metadata"],
			"spec":       m.(map[string]interface{})["spec"],
		}
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      manifestWorkName(cluster),
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":        "true",
					"acmlab.redhat.com/security-baseline": "true",
					"acmlab.redhat.com/security-level":    level,
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": wrappedManifests,
				},
			},
		},
	}
}

func buildConstraints(allowedRepos []string) []map[string]interface{} {
	repos := make([]interface{}, len(allowedRepos))
	for i, r := range allowedRepos {
		repos[i] = r
	}

	return []map[string]interface{}{
		{
			"apiVersion": "constraints.gatekeeper.sh/v1beta1",
			"kind":       "K8sPSPPrivilegedContainer",
			"metadata": map[string]interface{}{
				"name": "deny-privileged",
			},
			"spec": map[string]interface{}{
				"match": map[string]interface{}{
					"kinds": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{""},
							"kinds":     []interface{}{"Pod"},
						},
					},
				},
			},
		},
		{
			"apiVersion": "constraints.gatekeeper.sh/v1beta1",
			"kind":       "K8sPSPHostNamespace",
			"metadata": map[string]interface{}{
				"name": "deny-host-namespace",
			},
			"spec": map[string]interface{}{
				"match": map[string]interface{}{
					"kinds": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{""},
							"kinds":     []interface{}{"Pod"},
						},
					},
				},
			},
		},
		{
			"apiVersion": "constraints.gatekeeper.sh/v1beta1",
			"kind":       "K8sRequiredResources",
			"metadata": map[string]interface{}{
				"name": "require-resource-limits",
			},
			"spec": map[string]interface{}{
				"match": map[string]interface{}{
					"kinds": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{""},
							"kinds":     []interface{}{"Pod"},
						},
					},
				},
			},
		},
		{
			"apiVersion": "constraints.gatekeeper.sh/v1beta1",
			"kind":       "K8sAllowedRepos",
			"metadata": map[string]interface{}{
				"name": "allowed-repos",
			},
			"spec": map[string]interface{}{
				"match": map[string]interface{}{
					"kinds": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{""},
							"kinds":     []interface{}{"Pod"},
						},
					},
				},
				"parameters": map[string]interface{}{
					"repos": repos,
				},
			},
		},
	}
}

func buildCustomRegoManifestWork(cluster, name, pkg, rego string, matchKinds []string) *unstructured.Unstructured {
	kinds := make([]interface{}, len(matchKinds))
	for i, k := range matchKinds {
		kinds[i] = k
	}

	constraintKind := toCamelCase(pkg)
	templateName := toKebab(pkg)
	mwName := "custom-rego-" + name + "-" + cluster

	template := map[string]interface{}{
		"apiVersion": "templates.gatekeeper.sh/v1",
		"kind":       "ConstraintTemplate",
		"metadata": map[string]interface{}{
			"name": templateName,
		},
		"spec": map[string]interface{}{
			"crd": map[string]interface{}{
				"spec": map[string]interface{}{
					"names": map[string]interface{}{
						"kind": constraintKind,
					},
				},
			},
			"targets": []interface{}{
				map[string]interface{}{
					"target": "admission.k8s.gatekeeper.sh",
					"rego":   rego,
				},
			},
		},
	}

	constraint := map[string]interface{}{
		"apiVersion": "constraints.gatekeeper.sh/v1beta1",
		"kind":       constraintKind,
		"metadata": map[string]interface{}{
			"name": templateName + "-constraint",
		},
		"spec": map[string]interface{}{
			"match": map[string]interface{}{
				"kinds": []interface{}{
					map[string]interface{}{
						"apiGroups": []interface{}{""},
						"kinds":     kinds,
					},
				},
			},
		},
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      mwName,
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":     "true",
					"acmlab.redhat.com/custom-rego": "true",
					"acmlab.redhat.com/rego-policy": name,
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{template, constraint},
				},
			},
		},
	}
}

func toCamelCase(pkg string) string {
	parts := splitPkgName(pkg)
	result := ""
	for _, p := range parts {
		if len(p) > 0 {
			result += strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return result
}

func toKebab(pkg string) string {
	parts := splitPkgName(pkg)
	return strings.Join(parts, "-")
}

func splitPkgName(pkg string) []string {
	var parts []string
	current := ""
	for i, r := range pkg {
		if r == '_' || r == '.' {
			if current != "" {
				parts = append(parts, current)
			}
			current = ""
			continue
		}
		if i > 0 && r >= 'A' && r <= 'Z' && pkg[i-1] >= 'a' && pkg[i-1] <= 'z' {
			if current != "" {
				parts = append(parts, current)
			}
			current = string(r)
			continue
		}
		current += string(r)
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

func buildKyvernoManifestWork(cluster, name, policyYAML string) *unstructured.Unstructured {
	mwName := "kyverno-policy-" + name + "-" + cluster

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      mwName,
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":        "true",
					"acmlab.redhat.com/kyverno-policy": "true",
					"acmlab.redhat.com/policy-name":    name,
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{
						map[string]interface{}{
							"apiVersion": "kyverno.io/v1",
							"kind":       "ClusterPolicy",
							"metadata": map[string]interface{}{
								"name": name,
								"annotations": map[string]interface{}{
									"acmlab.redhat.com/policy-source": policyYAML,
								},
							},
						},
					},
				},
			},
		},
	}
}

func buildGatekeeperHealthPolicy(cluster, clusterSet string) *unstructured.Unstructured {
	policyName := healthPolicyName(cluster)

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1",
			"kind":       "Policy",
			"metadata": map[string]interface{}{
				"name":      policyName,
				"namespace": DefaultNamespace,
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
								"name": policyName + "-config",
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
												"name":      "gatekeeper-controller-manager",
												"namespace": "gatekeeper-system",
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

func buildHealthPlacement(cluster, clusterSet string) *unstructured.Unstructured {
	policyName := healthPolicyName(cluster)

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
				"name":      policyName + "-placement",
				"namespace": DefaultNamespace,
			},
			"spec": spec,
		},
	}
}

func buildHealthPlacementBinding(cluster string) *unstructured.Unstructured {
	policyName := healthPolicyName(cluster)

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1",
			"kind":       "PlacementBinding",
			"metadata": map[string]interface{}{
				"name":      policyName + "-placement-binding",
				"namespace": DefaultNamespace,
			},
			"placementRef": map[string]interface{}{
				"apiGroup": "cluster.open-cluster-management.io",
				"kind":     "Placement",
				"name":     policyName + "-placement",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"apiGroup": "policy.open-cluster-management.io",
					"kind":     "Policy",
					"name":     policyName,
				},
			},
		},
	}
}
