package policy

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type PolicyOpts struct {
	Name              string
	Namespace         string
	RemediationAction string
	ClusterLabels     map[string]string
	AllowedRegistries []string
	OperatorName      string
	OperatorVersion   string
	OperatorChannel   string
	CertExpiryDays    int
	CertNamespaces    []string
	ClusterSet        string
	MaxWorkers        int
	MaxGPUs           int
}

func (m *Manager) ensurePolicy(ctx context.Context, namespace string, opts PolicyOpts) error {
	remediation := opts.RemediationAction
	if remediation == "" {
		remediation = "inform"
	}

	policyTemplates := buildPolicyTemplates(opts, remediation)

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1",
			"kind":       "Policy",
			"metadata": map[string]interface{}{
				"name":      opts.Name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"disabled":          false,
				"remediationAction": remediation,
				"policy-templates":  policyTemplates,
			},
		},
	}
	return m.client.CreateIfNotExists(ctx, client.GVRPolicy, namespace, obj)
}

func (m *Manager) ensurePlacement(ctx context.Context, namespace string, opts PolicyOpts) error {
	predicates := []interface{}{}
	if len(opts.ClusterLabels) > 0 {
		matchExpressions := []interface{}{}
		for k, v := range opts.ClusterLabels {
			matchExpressions = append(matchExpressions, map[string]interface{}{
				"key":      k,
				"operator": "In",
				"values":   []interface{}{v},
			})
		}
		predicates = append(predicates, map[string]interface{}{
			"requiredClusterSelector": map[string]interface{}{
				"labelSelector": map[string]interface{}{
					"matchExpressions": matchExpressions,
				},
			},
		})
	}

	spec := map[string]interface{}{
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
	if opts.ClusterSet != "" {
		spec["clusterSets"] = []interface{}{opts.ClusterSet}
	}
	if len(predicates) > 0 {
		spec["predicates"] = predicates
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta1",
			"kind":       "Placement",
			"metadata": map[string]interface{}{
				"name":      opts.Name + "-placement",
				"namespace": namespace,
			},
			"spec": spec,
		},
	}
	return m.client.CreateIfNotExists(ctx, client.GVRPlacement, namespace, obj)
}

func (m *Manager) ensurePlacementBinding(ctx context.Context, namespace string, opts PolicyOpts) error {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1",
			"kind":       "PlacementBinding",
			"metadata": map[string]interface{}{
				"name":      opts.Name + "-placement-binding",
				"namespace": namespace,
			},
			"placementRef": map[string]interface{}{
				"apiGroup": "cluster.open-cluster-management.io",
				"kind":     "Placement",
				"name":     opts.Name + "-placement",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"apiGroup": "policy.open-cluster-management.io",
					"kind":     "Policy",
					"name":     opts.Name,
				},
			},
		},
	}
	return m.client.CreateIfNotExists(ctx, client.GVRPlacementBinding, namespace, obj)
}

func buildPolicyTemplates(opts PolicyOpts, remediation string) []interface{} {
	if opts.OperatorName != "" {
		return buildOperatorPolicyTemplate(opts)
	}
	if opts.CertExpiryDays > 0 {
		return buildCertificatePolicyTemplate(opts, remediation)
	}
	if opts.MaxWorkers > 0 || opts.MaxGPUs > 0 {
		return buildQuotaPolicyTemplate(opts, remediation)
	}
	return buildConfigurationPolicyTemplate(opts, remediation)
}

func buildConfigurationPolicyTemplate(opts PolicyOpts, remediation string) []interface{} {
	var objectTemplates []interface{}
	if len(opts.AllowedRegistries) > 0 {
		objectTemplates = buildRegistryRestrictionTemplates(opts.AllowedRegistries)
	} else {
		objectTemplates = buildNamespaceTemplate(opts.Name)
	}
	return []interface{}{
		map[string]interface{}{
			"objectDefinition": map[string]interface{}{
				"apiVersion": "policy.open-cluster-management.io/v1",
				"kind":       "ConfigurationPolicy",
				"metadata": map[string]interface{}{
					"name": opts.Name + "-config",
				},
				"spec": map[string]interface{}{
					"remediationAction":   remediation,
					"severity":            "medium",
					"object-templates":    objectTemplates,
					"pruneObjectBehavior": "None",
				},
			},
		},
	}
}

func buildOperatorPolicyTemplate(opts PolicyOpts) []interface{} {
	spec := map[string]interface{}{
		"remediationAction": "inform",
		"severity":          "medium",
		"complianceType":    "musthave",
		"subscription": map[string]interface{}{
			"name": opts.OperatorName,
		},
	}
	if opts.OperatorVersion != "" {
		spec["versions"] = []interface{}{opts.OperatorVersion}
	}
	if opts.OperatorChannel != "" {
		sub := spec["subscription"].(map[string]interface{})
		sub["channel"] = opts.OperatorChannel
	}
	return []interface{}{
		map[string]interface{}{
			"objectDefinition": map[string]interface{}{
				"apiVersion": "policy.open-cluster-management.io/v1beta1",
				"kind":       "OperatorPolicy",
				"metadata": map[string]interface{}{
					"name": opts.Name + "-operator",
				},
				"spec": spec,
			},
		},
	}
}

func buildCertificatePolicyTemplate(opts PolicyOpts, remediation string) []interface{} {
	namespaces := opts.CertNamespaces
	if len(namespaces) == 0 {
		namespaces = []string{"openshift-config", "openshift-ingress"}
	}
	nsSelector := make([]interface{}, len(namespaces))
	for i, ns := range namespaces {
		nsSelector[i] = ns
	}

	return []interface{}{
		map[string]interface{}{
			"objectDefinition": map[string]interface{}{
				"apiVersion": "policy.open-cluster-management.io/v1",
				"kind":       "CertificatePolicy",
				"metadata": map[string]interface{}{
					"name": opts.Name + "-cert",
				},
				"spec": map[string]interface{}{
					"remediationAction": remediation,
					"severity":          "high",
					"minimumDuration":   fmt.Sprintf("%dh", opts.CertExpiryDays*24),
					"namespaceSelector": map[string]interface{}{
						"include": nsSelector,
					},
				},
			},
		},
	}
}

func buildNamespaceTemplate(name string) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"complianceType": "musthave",
			"objectDefinition": map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name": name,
				},
			},
		},
	}
}

func buildQuotaPolicyTemplate(opts PolicyOpts, remediation string) []interface{} {
	var objectTemplates []interface{}

	if opts.MaxWorkers > 0 {
		objectTemplates = append(objectTemplates, buildWorkerQuotaTemplate(opts.MaxWorkers))
	}
	if opts.MaxGPUs > 0 {
		objectTemplates = append(objectTemplates, buildGPUQuotaTemplate(opts.MaxGPUs))
	}

	return []interface{}{
		map[string]interface{}{
			"objectDefinition": map[string]interface{}{
				"apiVersion": "policy.open-cluster-management.io/v1",
				"kind":       "ConfigurationPolicy",
				"metadata": map[string]interface{}{
					"name": opts.Name + "-quota",
				},
				"spec": map[string]interface{}{
					"remediationAction":   remediation,
					"severity":            "high",
					"object-templates":    objectTemplates,
					"pruneObjectBehavior": "None",
				},
			},
		},
	}
}

func buildWorkerQuotaTemplate(maxWorkers int) map[string]interface{} {
	return map[string]interface{}{
		"complianceType": "musthave",
		"objectDefinition": map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "caas-worker-quota",
				"namespace": "open-cluster-management-agent",
				"annotations": map[string]interface{}{
					"caas/max-workers":   fmt.Sprintf("%d", maxWorkers),
					"caas/quota-enforced": "true",
				},
			},
		},
	}
}

func buildGPUQuotaTemplate(maxGPUs int) map[string]interface{} {
	return map[string]interface{}{
		"complianceType": "musthave",
		"objectDefinition": map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "caas-gpu-quota",
				"namespace": "open-cluster-management-agent",
				"annotations": map[string]interface{}{
					"caas/max-gpus":      fmt.Sprintf("%d", maxGPUs),
					"caas/quota-enforced": "true",
				},
			},
		},
	}
}

func buildRegistryRestrictionTemplates(registries []string) []interface{} {
	allowedList := make([]interface{}, len(registries))
	for i, r := range registries {
		allowedList[i] = r
	}

	return []interface{}{
		map[string]interface{}{
			"complianceType": "musthave",
			"objectDefinition": map[string]interface{}{
				"apiVersion": "config.openshift.io/v1",
				"kind":       "Image",
				"metadata": map[string]interface{}{
					"name": "cluster",
				},
				"spec": map[string]interface{}{
					"registrySources": map[string]interface{}{
						"allowedRegistries": allowedList,
					},
				},
			},
		},
	}
}
