package rollout

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func buildManifestWorkReplicaSet(opts RolloutOpts) *unstructured.Unstructured {
	manifests := make([]interface{}, 0, len(opts.Manifests))
	for _, m := range opts.Manifests {
		manifests = append(manifests, m)
	}

	rolloutStrategy := buildRolloutStrategy(opts.Strategy, opts.MaxConcurrency, opts.MaxFailures, opts.ProgressDeadline)

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1alpha1",
			"kind":       "ManifestWorkReplicaSet",
			"metadata": map[string]interface{}{
				"name":      opts.Name,
				"namespace": opts.Namespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
				},
			},
			"spec": map[string]interface{}{
				"placementRefs": []interface{}{
					map[string]interface{}{
						"name":            opts.PlacementName,
						"rolloutStrategy": rolloutStrategy,
					},
				},
				"manifestWorkTemplate": map[string]interface{}{
					"workload": map[string]interface{}{
						"manifests": manifests,
					},
				},
			},
		},
	}
}

func buildRolloutStrategy(strategyType string, maxConcurrency int, maxFailures, progressDeadline string) map[string]interface{} {
	if strategyType == "" {
		strategyType = "All"
	}

	strategy := map[string]interface{}{
		"rolloutType": strategyType,
	}

	if strategyType == "Progressive" || strategyType == "ProgressivePerGroup" {
		progressive := map[string]interface{}{}
		if maxConcurrency > 0 {
			progressive["maxConcurrency"] = int64(maxConcurrency)
		}
		if maxFailures != "" {
			progressive["maxFailures"] = maxFailures
		}
		if progressDeadline != "" {
			progressive["progressDeadline"] = progressDeadline
		}

		key := "progressive"
		if strategyType == "ProgressivePerGroup" {
			key = "progressivePerGroup"
		}
		strategy[key] = progressive
	}

	return strategy
}

func parseRolloutInfo(name string, obj map[string]interface{}) *RolloutInfo {
	info := &RolloutInfo{Name: name, Status: "Unknown"}

	if meta, ok := obj["metadata"].(map[string]interface{}); ok {
		info.Namespace, _ = meta["namespace"].(string)
	}

	if spec, ok := obj["spec"].(map[string]interface{}); ok {
		if refs, ok := spec["placementRefs"].([]interface{}); ok && len(refs) > 0 {
			if ref, ok := refs[0].(map[string]interface{}); ok {
				if rs, ok := ref["rolloutStrategy"].(map[string]interface{}); ok {
					info.Strategy, _ = rs["rolloutType"].(string)
				}
			}
		}
	}

	status, _ := obj["status"].(map[string]interface{})
	if status == nil {
		info.Status = "Pending"
		return info
	}

	if summary, ok := status["summary"].(map[string]interface{}); ok {
		if applied, ok := summary["applied"].(int64); ok {
			info.Applied = int(applied)
		}
		if total, ok := summary["total"].(int64); ok {
			info.Total = int(total)
		}
		if failed, ok := summary["failed"].(int64); ok {
			info.Failed = int(failed)
		}
	}

	conditions, _ := status["conditions"].([]interface{})
	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := cond["type"].(string)
		condStatus, _ := cond["status"].(string)
		if condType == "PlacementVerified" && condStatus == "True" {
			info.Status = "Active"
		}
		if condType == "PlacementVerified" && condStatus == "False" {
			info.Status = "PlacementNotFound"
		}
	}

	if info.Failed > 0 {
		info.Status = "Degraded"
	}
	if info.Applied > 0 && info.Applied == info.Total && info.Failed == 0 {
		info.Status = "Complete"
	}

	return info
}
