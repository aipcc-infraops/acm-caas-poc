package gitops

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func buildApplicationSet(opts AppSetOpts) *unstructured.Unstructured {
	labels := map[string]interface{}{
		"acmlab.redhat.com/managed":   "true",
		"acmlab.redhat.com/gitops":    "true",
		"acmlab.redhat.com/generator": opts.Generator,
	}

	generator := buildGenerator(opts)

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "ApplicationSet",
			"metadata": map[string]interface{}{
				"name":      opts.Name,
				"namespace": opts.Namespace,
				"labels":    labels,
			},
			"spec": map[string]interface{}{
				"generators": []interface{}{generator},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "{{name}}-" + opts.Name,
					},
					"spec": map[string]interface{}{
						"project": opts.Project,
						"source": map[string]interface{}{
							"repoURL":        opts.RepoURL,
							"path":           opts.Path,
							"targetRevision": opts.Revision,
						},
						"destination": map[string]interface{}{
							"server":    "{{server}}",
							"namespace": "default",
						},
						"syncPolicy": map[string]interface{}{
							"automated": map[string]interface{}{
								"selfHeal": true,
								"prune":    true,
							},
						},
					},
				},
			},
		},
	}
}

func buildGenerator(opts AppSetOpts) map[string]interface{} {
	matchLabels := map[string]interface{}{}
	for k, v := range opts.LabelSelector {
		matchLabels[k] = v
	}

	if opts.Generator == "cluster" {
		return map[string]interface{}{
			"clusters": map[string]interface{}{
				"selector": map[string]interface{}{
					"matchLabels": matchLabels,
				},
			},
		}
	}

	return map[string]interface{}{
		"clusterDecisionResource": map[string]interface{}{
			"configMapRef": "acm-placement",
			"labelSelector": map[string]interface{}{
				"matchLabels": matchLabels,
			},
			"requeueAfterSeconds": int64(180),
		},
	}
}
