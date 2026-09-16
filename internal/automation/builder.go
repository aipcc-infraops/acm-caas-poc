package automation

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func buildPolicyAutomation(opts AutomationOpts) *unstructured.Unstructured {
	extraVars := map[string]interface{}{
		"policy_name":     "{{ policy_name }}",
		"target_clusters": "{{ target_clusters }}",
	}
	for k, v := range opts.ExtraVars {
		extraVars[k] = v
	}

	automationDef := map[string]interface{}{
		"name":       opts.JobTemplate,
		"secret":     opts.TowerSecret,
		"type":       "AnsibleJob",
		"extra_vars": extraVars,
	}

	spec := map[string]interface{}{
		"policyRef":     opts.PolicyName,
		"mode":          opts.Mode,
		"automationDef": automationDef,
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1beta1",
			"kind":       "PolicyAutomation",
			"metadata": map[string]interface{}{
				"name":      opts.Name,
				"namespace": opts.Namespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":    "true",
					"acmlab.redhat.com/automation": "true",
				},
			},
			"spec": spec,
		},
	}
}
