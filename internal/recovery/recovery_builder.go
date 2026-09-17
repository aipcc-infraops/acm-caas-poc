package recovery

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func applyDRDefaults(opts *DROpts) {
	if opts.PairName == "" {
		opts.PairName = drPairName(opts.SourceCluster, opts.TargetCluster)
	}
	if opts.Schedule == "" {
		opts.Schedule = "0 */4 * * *"
	}
	if opts.TTL == "" {
		opts.TTL = "720h"
	}
	if opts.RepoURL == "" {
		opts.RepoURL = "https://github.com/example-org/cluster-configs"
	}
	if opts.Path == "" {
		opts.Path = "workloads/"
	}
}

func drPairName(source, target string) string {
	return fmt.Sprintf("%s-%s", source, target)
}

func buildDRLabels(cluster, pairName, role string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      "dr-labels-" + pairName,
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
					"acmlab.redhat.com/dr-role": role,
					"acmlab.redhat.com/dr-pair": pairName,
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{
						map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "ConfigMap",
							"metadata": map[string]interface{}{
								"name":      "dr-config",
								"namespace": "openshift-config",
							},
							"data": map[string]interface{}{
								"dr-role": role,
								"dr-pair": pairName,
							},
						},
					},
				},
			},
		},
	}
}

func buildVeleroManifestWork(opts DROpts) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      "dr-velero-" + opts.PairName,
				"namespace": opts.SourceCluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
					"acmlab.redhat.com/dr-pair": opts.PairName,
					"acmlab.redhat.com/dr-type": "velero-schedule",
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{
						map[string]interface{}{
							"apiVersion": "velero.io/v1",
							"kind":       "Schedule",
							"metadata": map[string]interface{}{
								"name":      "dr-backup-" + opts.PairName,
								"namespace": "openshift-adp",
							},
							"spec": map[string]interface{}{
								"schedule": opts.Schedule,
								"template": map[string]interface{}{
									"ttl":                     opts.TTL,
									"includedNamespaces":      []interface{}{"*"},
									"storageLocation":         "default",
									"volumeSnapshotLocations": []interface{}{"default"},
								},
							},
						},
					},
				},
			},
		},
	}
}

func buildVeleroPolicy(opts DROpts) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1",
			"kind":       "ConfigurationPolicy",
			"metadata": map[string]interface{}{
				"name":      "dr-velero-policy-" + opts.PairName,
				"namespace": DefaultNamespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
					"acmlab.redhat.com/dr-pair": opts.PairName,
				},
			},
			"spec": map[string]interface{}{
				"remediationAction": "enforce",
				"severity":          "high",
				"object-templates": []interface{}{
					map[string]interface{}{
						"complianceType": "musthave",
						"objectDefinition": map[string]interface{}{
							"apiVersion": "velero.io/v1",
							"kind":       "Schedule",
							"metadata": map[string]interface{}{
								"name":      "dr-backup-" + opts.PairName,
								"namespace": "openshift-adp",
							},
							"spec": map[string]interface{}{
								"schedule": opts.Schedule,
							},
						},
					},
				},
			},
		},
	}
}

func buildRestoreManifestWork(source, target, pairName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      "dr-restore-" + pairName,
				"namespace": target,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
					"acmlab.redhat.com/dr-pair": pairName,
					"acmlab.redhat.com/dr-type": "restore",
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{
						map[string]interface{}{
							"apiVersion": "velero.io/v1",
							"kind":       "Restore",
							"metadata": map[string]interface{}{
								"name":      "dr-restore-" + pairName,
								"namespace": "openshift-adp",
							},
							"spec": map[string]interface{}{
								"backupName":         "dr-backup-" + pairName,
								"includedNamespaces": []interface{}{"*"},
								"restorePVs":         true,
							},
						},
					},
				},
			},
		},
	}
}

func buildFailoverApplicationSet(source, target, pairName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "ApplicationSet",
			"metadata": map[string]interface{}{
				"name":      "dr-failover-" + pairName,
				"namespace": "openshift-gitops",
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
					"acmlab.redhat.com/dr-pair": pairName,
					"acmlab.redhat.com/dr-type": "failover",
				},
			},
			"spec": map[string]interface{}{
				"generators": []interface{}{
					map[string]interface{}{
						"clusterDecisionResource": map[string]interface{}{
							"configMapRef":      "acm-placement",
							"labelSelector":     map[string]interface{}{"matchLabels": map[string]interface{}{"acmlab.redhat.com/dr-pair": pairName}},
							"requeueAfterSeconds": int64(180),
						},
					},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "dr-{{name}}-" + pairName,
					},
					"spec": map[string]interface{}{
						"project": "default",
						"source": map[string]interface{}{
							"repoURL":        "https://github.com/example-org/cluster-configs",
							"path":           "workloads/",
							"targetRevision": "main",
						},
						"destination": map[string]interface{}{
							"server":    "{{server}}",
							"namespace": "default",
						},
					},
				},
			},
		},
	}
}

func parseManifestWorkPhase(obj map[string]interface{}) string {
	status, _ := obj["status"].(map[string]interface{})
	if status == nil {
		return "Pending"
	}
	conditions, _ := status["conditions"].([]interface{})
	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] == "Applied" && cond["status"] == "True" {
			return "Applied"
		}
	}
	return "Pending"
}

func parseAppSetPhase(obj map[string]interface{}) string {
	status, _ := obj["status"].(map[string]interface{})
	if status == nil {
		return "Pending"
	}
	conditions, _ := status["conditions"].([]interface{})
	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] == "ResourcesUpToDate" && cond["status"] == "True" {
			return "Synced"
		}
	}
	return "Pending"
}

func parseDRManifestWork(obj map[string]interface{}) DRPair {
	pair := DRPair{}
	meta, _ := obj["metadata"].(map[string]interface{})
	if meta == nil {
		return pair
	}
	labels, _ := meta["labels"].(map[string]interface{})
	if labels == nil {
		return pair
	}
	pair.Name, _ = labels["acmlab.redhat.com/dr-pair"].(string)
	role, _ := labels["acmlab.redhat.com/dr-role"].(string)
	ns, _ := meta["namespace"].(string)

	switch role {
	case "primary":
		pair.SourceCluster = ns
		pair.SourceRole = role
	case "standby":
		pair.TargetCluster = ns
		pair.TargetRole = role
	}
	return pair
}

func mergeDRPair(existing, incoming *DRPair) {
	if incoming.SourceCluster != "" {
		existing.SourceCluster = incoming.SourceCluster
		existing.SourceRole = incoming.SourceRole
	}
	if incoming.TargetCluster != "" {
		existing.TargetCluster = incoming.TargetCluster
		existing.TargetRole = incoming.TargetRole
	}
}
