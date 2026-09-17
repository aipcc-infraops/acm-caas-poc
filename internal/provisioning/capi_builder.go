package provisioning

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func buildCAPICluster(opts CAPIClusterOpts) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.x-k8s.io/v1beta1",
			"kind":       "Cluster",
			"metadata": map[string]interface{}{
				"name":      opts.Name,
				"namespace": opts.Namespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":     "true",
					"acmlab.redhat.com/provisioner":  "capi",
					"acmlab.redhat.com/infra-provider": opts.InfraProvider,
				},
			},
			"spec": map[string]interface{}{
				"clusterNetwork": map[string]interface{}{
					"pods": map[string]interface{}{
						"cidrBlocks": []interface{}{"192.168.0.0/16"},
					},
					"services": map[string]interface{}{
						"cidrBlocks": []interface{}{"10.128.0.0/12"},
					},
				},
				"infrastructureRef": map[string]interface{}{
					"apiVersion": fmt.Sprintf("infrastructure.cluster.x-k8s.io/v1beta1"),
					"kind":       infraClusterKind(opts.InfraProvider),
					"name":       opts.Name,
					"namespace":  opts.Namespace,
				},
				"controlPlaneRef": map[string]interface{}{
					"apiVersion": "controlplane.cluster.x-k8s.io/v1beta1",
					"kind":       "KubeadmControlPlane",
					"name":       opts.Name + "-control-plane",
					"namespace":  opts.Namespace,
				},
			},
		},
	}
}

func infraClusterKind(provider string) string {
	switch provider {
	case "aws":
		return "AWSCluster"
	case "azure":
		return "AzureCluster"
	case "gcp":
		return "GCPCluster"
	case "docker":
		return "DockerCluster"
	default:
		return "InfrastructureCluster"
	}
}

func buildCAPIMachineDeployment(opts CAPIClusterOpts) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.x-k8s.io/v1beta1",
			"kind":       "MachineDeployment",
			"metadata": map[string]interface{}{
				"name":      opts.Name + "-workers",
				"namespace": opts.Namespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
					"cluster.x-k8s.io/cluster-name": opts.Name,
				},
			},
			"spec": map[string]interface{}{
				"clusterName": opts.Name,
				"replicas":    opts.WorkerReplicas,
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"cluster.x-k8s.io/cluster-name": opts.Name,
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"cluster.x-k8s.io/cluster-name": opts.Name,
						},
					},
					"spec": map[string]interface{}{
						"clusterName": opts.Name,
						"version":     opts.KubernetesVersion,
						"bootstrap": map[string]interface{}{
							"configRef": map[string]interface{}{
								"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
								"kind":       "KubeadmConfigTemplate",
								"name":       opts.Name + "-workers",
								"namespace":  opts.Namespace,
							},
						},
						"infrastructureRef": map[string]interface{}{
							"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
							"kind":       infraMachineTemplateKind(opts.InfraProvider),
							"name":       opts.Name + "-workers",
							"namespace":  opts.Namespace,
						},
					},
				},
			},
		},
	}
}

func infraMachineTemplateKind(provider string) string {
	switch provider {
	case "aws":
		return "AWSMachineTemplate"
	case "azure":
		return "AzureMachineTemplate"
	case "gcp":
		return "GCPMachineTemplate"
	case "docker":
		return "DockerMachineTemplate"
	default:
		return "InfrastructureMachineTemplate"
	}
}

func buildCAPIManagedCluster(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": name,
				"labels": map[string]interface{}{
					"vendor":                                        "Kubernetes",
					"cluster.open-cluster-management.io/clusterset": "default",
					"acmlab.redhat.com/managed":                     "true",
					"acmlab.redhat.com/provisioner":                  "capi",
				},
			},
			"spec": map[string]interface{}{
				"hubAcceptsClient": true,
			},
		},
	}
}

func parseCAPIClusterInfo(obj map[string]interface{}) *CAPIClusterInfo {
	info := &CAPIClusterInfo{}
	if meta, ok := obj["metadata"].(map[string]interface{}); ok {
		info.Name, _ = meta["name"].(string)
		info.Namespace, _ = meta["namespace"].(string)
	}
	if status, ok := obj["status"].(map[string]interface{}); ok {
		info.Phase, _ = status["phase"].(string)
		conditions, _ := status["conditions"].([]interface{})
		for _, raw := range conditions {
			cond, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			condType, _ := cond["type"].(string)
			condStatus, _ := cond["status"].(string)
			info.Conditions = append(info.Conditions, fmt.Sprintf("%s=%s", condType, condStatus))
			if condType == "Ready" && condStatus == "True" {
				info.Ready = true
			}
		}
	}
	if spec, ok := obj["spec"].(map[string]interface{}); ok {
		if tmpl, ok := spec["topology"].(map[string]interface{}); ok {
			info.KubernetesVersion, _ = tmpl["version"].(string)
		}
	}
	if labels, ok := getLabels(obj); ok {
		if info.KubernetesVersion == "" {
			info.KubernetesVersion, _ = labels["acmlab.redhat.com/kubernetes-version"]
		}
	}
	return info
}

func getLabels(obj map[string]interface{}) (map[string]string, bool) {
	meta, ok := obj["metadata"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	rawLabels, ok := meta["labels"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	labels := make(map[string]string, len(rawLabels))
	for k, v := range rawLabels {
		labels[k], _ = v.(string)
	}
	return labels, true
}
