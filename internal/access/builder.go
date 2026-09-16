package access

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func buildManagedServiceAccount(cluster string, opts AccessOpts) *unstructured.Unstructured {
	ttl := opts.TTL
	if ttl == "" {
		ttl = "720h"
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "authentication.open-cluster-management.io/v1beta1",
			"kind":       "ManagedServiceAccount",
			"metadata": map[string]interface{}{
				"name":      msaName,
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"rotation": map[string]interface{}{
					"enabled":  true,
					"validity": ttl,
				},
			},
		},
	}
}

func buildClusterProxyAddOn(cluster string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "addon.open-cluster-management.io/v1alpha1",
			"kind":       "ManagedClusterAddOn",
			"metadata": map[string]interface{}{
				"name":      "cluster-proxy",
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"installNamespace": "open-cluster-management-agent-addon",
			},
		},
	}
}

func buildManagedServiceAccountAddOn(cluster string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "addon.open-cluster-management.io/v1alpha1",
			"kind":       "ManagedClusterAddOn",
			"metadata": map[string]interface{}{
				"name":      "managed-serviceaccount",
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"installNamespace": "open-cluster-management-agent-addon",
			},
		},
	}
}

func parseAccessStatus(cluster string, msaObj, proxyAddonObj map[string]interface{}) *AccessStatus {
	status := &AccessStatus{
		Cluster: cluster,
		Enabled: true,
	}

	if spec, ok := msaObj["spec"].(map[string]interface{}); ok {
		if rotation, ok := spec["rotation"].(map[string]interface{}); ok {
			status.TokenRotation, _ = rotation["validity"].(string)
		}
	}

	if msaStatus, ok := msaObj["status"].(map[string]interface{}); ok {
		if tokenRef, ok := msaStatus["tokenSecretRef"].(map[string]interface{}); ok {
			if name, _ := tokenRef["name"].(string); name != "" {
				status.TokenAvailable = true
			}
		}
		if rotationTS, ok := msaStatus["expirationTimestamp"].(string); ok {
			status.LastRotation = rotationTS
		}
	}

	if proxyAddonObj != nil {
		status.AddonHealthy = addonIsHealthy(proxyAddonObj)
	}

	return status
}

func addonIsHealthy(obj map[string]interface{}) bool {
	addonStatus, ok := obj["status"].(map[string]interface{})
	if !ok {
		return false
	}
	conditions, _ := addonStatus["conditions"].([]interface{})
	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] == "Available" && cond["status"] == "True" {
			return true
		}
	}
	return false
}

func parseAccessInfo(cluster string, msaObj map[string]interface{}) AccessInfo {
	info := AccessInfo{
		Cluster: cluster,
		Enabled: true,
	}

	if msaStatus, ok := msaObj["status"].(map[string]interface{}); ok {
		if tokenRef, ok := msaStatus["tokenSecretRef"].(map[string]interface{}); ok {
			if name, _ := tokenRef["name"].(string); name != "" {
				info.TokenAvailable = true
			}
		}
		conditions, _ := msaStatus["conditions"].([]interface{})
		for _, raw := range conditions {
			cond, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if cond["type"] == "TokenReported" {
				if cond["status"] == "True" {
					info.AddonStatus = "TokenReported"
				} else {
					info.AddonStatus = "Pending"
				}
			}
		}
	}

	if info.AddonStatus == "" {
		info.AddonStatus = "Pending"
	}

	return info
}
