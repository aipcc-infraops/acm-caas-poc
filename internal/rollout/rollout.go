package rollout

import (
	"context"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

const DefaultNamespace = "open-cluster-management"

type RolloutOpts struct {
	Name             string
	Namespace        string
	PlacementName    string
	Strategy         string
	MaxConcurrency   int
	MaxFailures      string
	ProgressDeadline string
	Manifests        []map[string]interface{}
}

type StrategyOpts struct {
	Type             string
	MaxConcurrency   int
	MaxFailures      string
	ProgressDeadline string
}

type RolloutInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Strategy  string `json:"strategy"`
	Applied   int    `json:"applied"`
	Total     int    `json:"total"`
	Failed    int    `json:"failed"`
	Status    string `json:"status"`
}

type Manager struct {
	client *client.Client
	cfg    config.Config
	logger *slog.Logger
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger}
}

func (m *Manager) Create(ctx context.Context, opts RolloutOpts) error {
	m.logger.Info("rollout.Create", "name", opts.Name)
	if opts.Namespace == "" {
		opts.Namespace = DefaultNamespace
	}
	if opts.Strategy == "" {
		opts.Strategy = "All"
	}
	if opts.MaxConcurrency <= 0 {
		opts.MaxConcurrency = 1
	}

	obj := buildManifestWorkReplicaSet(opts)
	return m.client.CreateIfNotExists(ctx, client.GVRManifestWorkReplicaSet, opts.Namespace, obj)
}

func (m *Manager) Get(ctx context.Context, name, namespace string) (*RolloutInfo, error) {
	m.logger.Info("rollout.Get", "name", name)
	if namespace == "" {
		namespace = DefaultNamespace
	}
	obj, err := m.client.Get(ctx, client.GVRManifestWorkReplicaSet, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("getting ManifestWorkReplicaSet %s: %w", name, err)
	}
	info := parseRolloutInfo(name, obj.Object)
	return info, nil
}

func (m *Manager) List(ctx context.Context, namespace string) ([]RolloutInfo, error) {
	m.logger.Info("rollout.List")
	if namespace == "" {
		namespace = DefaultNamespace
	}
	list, err := m.client.List(ctx, client.GVRManifestWorkReplicaSet, namespace, "")
	if err != nil {
		return nil, fmt.Errorf("listing ManifestWorkReplicaSets: %w", err)
	}
	infos := make([]RolloutInfo, 0, len(list.Items))
	for _, item := range list.Items {
		infos = append(infos, *parseRolloutInfo(item.GetName(), item.Object))
	}
	return infos, nil
}

func (m *Manager) Delete(ctx context.Context, name, namespace string) (bool, error) {
	m.logger.Info("rollout.Delete", "name", name)
	if namespace == "" {
		namespace = DefaultNamespace
	}
	_, err := m.client.Get(ctx, client.GVRManifestWorkReplicaSet, namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking ManifestWorkReplicaSet %s: %w", name, err)
	}
	if err := m.client.DeleteIfExists(ctx, client.GVRManifestWorkReplicaSet, namespace, name); err != nil {
		return false, fmt.Errorf("deleting ManifestWorkReplicaSet %s: %w", name, err)
	}
	return true, nil
}

func (m *Manager) UpdateStrategy(ctx context.Context, name, namespace string, strategy StrategyOpts) error {
	m.logger.Info("rollout.UpdateStrategy", "name", name, "strategy", strategy.Type)
	if namespace == "" {
		namespace = DefaultNamespace
	}
	obj, err := m.client.Get(ctx, client.GVRManifestWorkReplicaSet, namespace, name)
	if err != nil {
		return fmt.Errorf("getting ManifestWorkReplicaSet %s: %w", name, err)
	}

	placementRefs := getPlacementRefs(obj)
	if len(placementRefs) == 0 {
		return fmt.Errorf("ManifestWorkReplicaSet %s has no placementRefs", name)
	}

	firstRef, ok := placementRefs[0].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid placementRef in ManifestWorkReplicaSet %s", name)
	}

	firstRef["rolloutStrategy"] = buildRolloutStrategy(strategy.Type, strategy.MaxConcurrency, strategy.MaxFailures, strategy.ProgressDeadline)

	if err := unstructured.SetNestedSlice(obj.Object, placementRefs, "spec", "placementRefs"); err != nil {
		return fmt.Errorf("setting placementRefs: %w", err)
	}

	_, err = m.client.Update(ctx, client.GVRManifestWorkReplicaSet, namespace, obj)
	return err
}

func getPlacementRefs(obj *unstructured.Unstructured) []interface{} {
	refs, _, _ := unstructured.NestedSlice(obj.Object, "spec", "placementRefs")
	return refs
}
