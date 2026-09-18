package fleet

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/types"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type Taint struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Effect string `json:"effect"`
}

func (i *Inspector) AddTaint(ctx context.Context, cluster, key, value, effect string) error {
	i.logger.Info("fleet.AddTaint", "cluster", cluster, "key", key, "effect", effect)

	mc, err := i.client.Get(ctx, client.GVRManagedCluster, "", cluster)
	if err != nil {
		return fmt.Errorf("cluster %s not found: %w", cluster, err)
	}

	taints := extractTaints(mc.Object)
	for _, t := range taints {
		if t.Key == key {
			return fmt.Errorf("taint with key %q already exists on cluster %s", key, cluster)
		}
	}

	taints = append(taints, Taint{Key: key, Value: value, Effect: effect})
	return i.patchTaints(ctx, cluster, taints)
}

func (i *Inspector) RemoveTaint(ctx context.Context, cluster, key string) error {
	i.logger.Info("fleet.RemoveTaint", "cluster", cluster, "key", key)

	mc, err := i.client.Get(ctx, client.GVRManagedCluster, "", cluster)
	if err != nil {
		return fmt.Errorf("cluster %s not found: %w", cluster, err)
	}

	taints := extractTaints(mc.Object)
	filtered := make([]Taint, 0, len(taints))
	found := false
	for _, t := range taints {
		if t.Key == key {
			found = true
			continue
		}
		filtered = append(filtered, t)
	}
	if !found {
		return fmt.Errorf("taint with key %q not found on cluster %s", key, cluster)
	}
	return i.patchTaints(ctx, cluster, filtered)
}

func (i *Inspector) ListTaints(ctx context.Context, cluster string) ([]Taint, error) {
	i.logger.Info("fleet.ListTaints", "cluster", cluster)

	mc, err := i.client.Get(ctx, client.GVRManagedCluster, "", cluster)
	if err != nil {
		return nil, fmt.Errorf("cluster %s not found: %w", cluster, err)
	}
	return extractTaints(mc.Object), nil
}

func (i *Inspector) CreateTolerantPlacement(ctx context.Context, name, namespace string, tolerations []Taint, clusterSets []string) error {
	i.logger.Info("fleet.CreateTolerantPlacement", "name", name)
	if namespace == "" {
		namespace = "open-cluster-management"
	}

	placement := buildTolerantPlacement(name, namespace, tolerations, clusterSets)
	if err := i.client.CreateIfNotExists(ctx, client.GVRPlacement, namespace, placement); err != nil {
		return fmt.Errorf("creating tolerant placement %s: %w", name, err)
	}
	return nil
}

func (i *Inspector) patchTaints(ctx context.Context, cluster string, taints []Taint) error {
	taintsList := make([]interface{}, len(taints))
	for idx, t := range taints {
		taintsList[idx] = map[string]interface{}{
			"key":    t.Key,
			"value":  t.Value,
			"effect": t.Effect,
		}
	}
	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"taints": taintsList,
		},
	}
	data, _ := json.Marshal(patch)
	if _, err := i.client.Patch(ctx, client.GVRManagedCluster, "", cluster, types.MergePatchType, data); err != nil {
		return fmt.Errorf("patching taints on %s: %w", cluster, err)
	}
	return nil
}

func extractTaints(obj map[string]interface{}) []Taint {
	spec, _ := obj["spec"].(map[string]interface{})
	if spec == nil {
		return nil
	}
	raw, _ := spec["taints"].([]interface{})
	result := make([]Taint, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		result = append(result, Taint{
			Key:    strVal(m, "key"),
			Value:  strVal(m, "value"),
			Effect: strVal(m, "effect"),
		})
	}
	return result
}

func strVal(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}
