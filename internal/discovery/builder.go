package discovery

import (
	"encoding/base64"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func buildDiscoverySecret(namespace, ocmToken string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "ocm-api-token",
				"namespace": namespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":   "true",
					"acmlab.redhat.com/discovery": "true",
				},
			},
			"type": "Opaque",
			"data": map[string]interface{}{
				"ocmAPIToken": base64.StdEncoding.EncodeToString([]byte(ocmToken)),
			},
		},
	}
}

func buildDiscoveryConfig(namespace string, lastActive int, versions []string) *unstructured.Unstructured {
	filters := map[string]interface{}{
		"lastActive": int64(lastActive),
	}
	if len(versions) > 0 {
		vList := make([]interface{}, len(versions))
		for i, v := range versions {
			vList[i] = v
		}
		filters["openShiftVersions"] = vList
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "discovery.open-cluster-management.io/v1alpha1",
			"kind":       "DiscoveryConfig",
			"metadata": map[string]interface{}{
				"name":      "discovery",
				"namespace": namespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":   "true",
					"acmlab.redhat.com/discovery": "true",
				},
			},
			"spec": map[string]interface{}{
				"credential": "ocm-api-token",
				"filters":    filters,
			},
		},
	}
}
