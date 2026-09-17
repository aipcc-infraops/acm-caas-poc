package addon

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func buildAddOnDeploymentConfig(opts AddOnConfigOpts) *unstructured.Unstructured {
	customVars := make([]interface{}, 0, len(opts.Values))
	for k, v := range opts.Values {
		customVars = append(customVars, map[string]interface{}{
			"name":  k,
			"value": v,
		})
	}

	spec := map[string]interface{}{
		"customizedVariables": customVars,
	}

	if opts.InstallNamespace != "" {
		spec["agentInstallNamespace"] = opts.InstallNamespace
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "addon.open-cluster-management.io/v1alpha1",
			"kind":       "AddOnDeploymentConfig",
			"metadata": map[string]interface{}{
				"name":      opts.Name,
				"namespace": opts.Namespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
				},
			},
			"spec": spec,
		},
	}
}
