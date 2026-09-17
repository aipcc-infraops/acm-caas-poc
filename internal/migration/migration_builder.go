package migration

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func migrationPlanID(source, target string) string {
	return fmt.Sprintf("%s-to-%s", source, target)
}

func buildMigrationPlan(plan *MigrationPlan) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "migration-plan-" + plan.PlanID,
				"namespace": DefaultNamespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":        "true",
					"acmlab.redhat.com/migration-plan": "true",
					"acmlab.redhat.com/source":         plan.SourceCluster,
					"acmlab.redhat.com/target":         plan.TargetCluster,
				},
			},
			"data": map[string]interface{}{
				"planID":        plan.PlanID,
				"sourceCluster": plan.SourceCluster,
				"targetCluster": plan.TargetCluster,
				"workloads":     fmt.Sprintf("%d", plan.Workloads),
				"namespaces":    strings.Join(plan.Namespaces, ","),
				"phase":         plan.Phase,
				"createdAt":     plan.CreatedAt,
			},
		},
	}
}

func buildCordonLabels(cluster, planID string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      "migration-cordon-" + planID,
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":   "true",
					"acmlab.redhat.com/migration": "draining",
					"acmlab.redhat.com/plan-id":   planID,
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{
						map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "ConfigMap",
							"metadata": map[string]interface{}{
								"name":      "migration-status",
								"namespace": "openshift-config",
							},
							"data": map[string]interface{}{
								"migration":   "draining",
								"plan-id":     planID,
								"cordon-time": "auto",
							},
						},
					},
				},
			},
		},
	}
}

func buildTargetManifestWork(plan *MigrationPlan) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      "migration-workloads-" + plan.PlanID,
				"namespace": plan.TargetCluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed":   "true",
					"acmlab.redhat.com/migration": "deploying",
					"acmlab.redhat.com/plan-id":   plan.PlanID,
					"acmlab.redhat.com/source":    plan.SourceCluster,
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": []interface{}{
						map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "ConfigMap",
							"metadata": map[string]interface{}{
								"name":      "migration-manifest",
								"namespace": "openshift-config",
							},
							"data": map[string]interface{}{
								"source":     plan.SourceCluster,
								"target":     plan.TargetCluster,
								"namespaces": strings.Join(plan.Namespaces, ","),
							},
						},
					},
				},
			},
		},
	}
}

func parseMigrationPlanFromConfigMap(obj map[string]interface{}) *MigrationPlan {
	plan := &MigrationPlan{}
	data, _ := obj["data"].(map[string]interface{})
	if data == nil {
		return plan
	}
	plan.PlanID, _ = data["planID"].(string)
	plan.SourceCluster, _ = data["sourceCluster"].(string)
	plan.TargetCluster, _ = data["targetCluster"].(string)
	plan.Phase, _ = data["phase"].(string)
	plan.CreatedAt, _ = data["createdAt"].(string)

	if w, ok := data["workloads"].(string); ok {
		fmt.Sscanf(w, "%d", &plan.Workloads)
	}
	if ns, ok := data["namespaces"].(string); ok && ns != "" {
		plan.Namespaces = strings.Split(ns, ",")
	}
	return plan
}

func parseMigrationSummary(obj map[string]interface{}) MigrationSummary {
	s := MigrationSummary{}
	data, _ := obj["data"].(map[string]interface{})
	if data == nil {
		return s
	}
	s.PlanID, _ = data["planID"].(string)
	s.SourceCluster, _ = data["sourceCluster"].(string)
	s.TargetCluster, _ = data["targetCluster"].(string)
	s.Phase, _ = data["phase"].(string)
	s.CreatedAt, _ = data["createdAt"].(string)
	return s
}
