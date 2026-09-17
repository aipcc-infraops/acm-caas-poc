package lifecycle

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type CuratorOpts struct {
	Cluster   string
	Namespace string
	PreHook   *CuratorHook
	PostHook  *CuratorHook
}

type CuratorHook struct {
	Name       string
	Type       string
	Image      string
	Commands   []string
	JobTTL     int
	ExtraVars  map[string]string
}

type CuratorInfo struct {
	Name      string       `json:"name"`
	Namespace string       `json:"namespace"`
	PreHook   *HookInfo    `json:"preHook,omitempty"`
	PostHook  *HookInfo    `json:"postHook,omitempty"`
	Status    string       `json:"status"`
}

type HookInfo struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status string `json:"status,omitempty"`
}

func (m *Manager) ApplyCurator(ctx context.Context, opts CuratorOpts) error {
	m.logger.Info("lifecycle.ApplyCurator", "cluster", opts.Cluster)
	ns := opts.Namespace
	if ns == "" {
		ns = opts.Cluster
	}

	curator := buildClusterCurator(opts)
	if err := m.client.CreateIfNotExists(ctx, client.GVRClusterCurator, ns, curator); err != nil {
		return fmt.Errorf("creating ClusterCurator: %w", err)
	}
	return nil
}

func (m *Manager) GetCurator(ctx context.Context, cluster, namespace string) (*CuratorInfo, error) {
	m.logger.Info("lifecycle.GetCurator", "cluster", cluster)
	if namespace == "" {
		namespace = cluster
	}

	obj, err := m.client.Get(ctx, client.GVRClusterCurator, namespace, cluster)
	if err != nil {
		return nil, fmt.Errorf("getting ClusterCurator %s: %w", cluster, err)
	}

	return parseCuratorInfo(obj.Object), nil
}

func (m *Manager) RemoveCurator(ctx context.Context, cluster, namespace string) (bool, error) {
	m.logger.Info("lifecycle.RemoveCurator", "cluster", cluster)
	if namespace == "" {
		namespace = cluster
	}

	_, err := m.client.Get(ctx, client.GVRClusterCurator, namespace, cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking ClusterCurator: %w", err)
	}

	if err := m.client.DeleteIfExists(ctx, client.GVRClusterCurator, namespace, cluster); err != nil {
		return false, fmt.Errorf("removing ClusterCurator: %w", err)
	}
	return true, nil
}

func (m *Manager) ListCurators(ctx context.Context, namespace string) ([]CuratorInfo, error) {
	m.logger.Info("lifecycle.ListCurators")

	list, err := m.client.List(ctx, client.GVRClusterCurator, namespace, "")
	if err != nil {
		return nil, fmt.Errorf("listing ClusterCurators: %w", err)
	}

	curators := make([]CuratorInfo, 0, len(list.Items))
	for _, item := range list.Items {
		curators = append(curators, *parseCuratorInfo(item.Object))
	}
	return curators, nil
}

func parseCuratorInfo(obj map[string]interface{}) *CuratorInfo {
	info := &CuratorInfo{}

	if meta, ok := obj["metadata"].(map[string]interface{}); ok {
		info.Name, _ = meta["name"].(string)
		info.Namespace, _ = meta["namespace"].(string)
	}

	spec, _ := obj["spec"].(map[string]interface{})
	if spec != nil {
		if pre, ok := spec["prehook"].([]interface{}); ok && len(pre) > 0 {
			if hook, ok := pre[0].(map[string]interface{}); ok {
				info.PreHook = parseHookInfo(hook)
			}
		}
		if post, ok := spec["posthook"].([]interface{}); ok && len(post) > 0 {
			if hook, ok := post[0].(map[string]interface{}); ok {
				info.PostHook = parseHookInfo(hook)
			}
		}
	}

	status, _ := obj["status"].(map[string]interface{})
	if status != nil {
		conditions, _ := status["conditions"].([]interface{})
		for _, raw := range conditions {
			cond, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			condType, _ := cond["type"].(string)
			condStatus, _ := cond["status"].(string)
			if condType == "clustercurator-job" {
				if condStatus == "True" {
					info.Status = "Complete"
				} else {
					info.Status = "InProgress"
				}
			}
		}
	}
	if info.Status == "" {
		info.Status = "Pending"
	}

	return info
}

func parseHookInfo(obj map[string]interface{}) *HookInfo {
	info := &HookInfo{}
	info.Name, _ = obj["name"].(string)
	info.Type, _ = obj["type"].(string)
	return info
}
