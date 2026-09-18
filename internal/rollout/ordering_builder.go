package rollout

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

func buildOrderedManifestWork(cluster, name string, manifests []OrderedManifest) *unstructured.Unstructured {
	manifestList := make([]interface{}, len(manifests))
	manifestConfigs := make([]interface{}, len(manifests))

	for i, m := range manifests {
		manifestList[i] = m.Object

		gvk := extractGVK(m.Object)
		manifestConfigs[i] = map[string]interface{}{
			"resourceIdentifier": map[string]interface{}{
				"ordinal": int64(m.Ordinal),
				"group":   gvk.group,
				"resource": gvk.resource,
				"name":    gvk.name,
				"namespace": gvk.namespace,
			},
			"updateStrategy": map[string]interface{}{
				"type": "ServerSideApply",
			},
		}
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":      "true",
					"acmlab.redhat.com/ordered-work": "true",
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": manifestList,
				},
				"manifestConfigs": manifestConfigs,
			},
		},
	}
}

type gvkInfo struct {
	group     string
	resource  string
	name      string
	namespace string
}

func extractGVK(obj map[string]interface{}) gvkInfo {
	meta, _ := obj["metadata"].(map[string]interface{})
	name, _ := meta["name"].(string)
	namespace, _ := meta["namespace"].(string)
	kind, _ := obj["kind"].(string)

	resourceMap := map[string]string{
		"Namespace":  "namespaces",
		"ConfigMap":  "configmaps",
		"Secret":     "secrets",
		"Deployment": "deployments",
		"Service":    "services",
		"ServiceAccount": "serviceaccounts",
	}

	resource := resourceMap[kind]
	if resource == "" {
		resource = kind
	}

	apiVersion, _ := obj["apiVersion"].(string)
	group := ""
	if apiVersion != "" && apiVersion != "v1" {
		parts := splitAPIVersion(apiVersion)
		group = parts
	}

	return gvkInfo{group: group, resource: resource, name: name, namespace: namespace}
}

func splitAPIVersion(apiVersion string) string {
	for i := len(apiVersion) - 1; i >= 0; i-- {
		if apiVersion[i] == '/' {
			return apiVersion[:i]
		}
	}
	return ""
}
