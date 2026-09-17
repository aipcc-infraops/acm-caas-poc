package gitops

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

const DefaultNamespace = "openshift-gitops"

type AppSetOpts struct {
	Name          string
	Namespace     string
	RepoURL       string
	Path          string
	Revision      string
	Generator     string
	LabelSelector map[string]string
	Project       string
}

type AppSetInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	RepoURL   string `json:"repoURL"`
	Path      string `json:"path"`
	Generator string `json:"generator"`
	Status    string `json:"status"`
	AppCount  int    `json:"appCount"`
}

type Manager struct {
	client *client.Client
	cfg    config.Config
	logger *slog.Logger
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger}
}

func (m *Manager) Create(ctx context.Context, opts AppSetOpts) error {
	m.logger.Info("gitops.Create", "name", opts.Name)
	applyDefaults(&opts)

	appSet := buildApplicationSet(opts)
	if err := m.client.CreateIfNotExists(ctx, client.GVRApplicationSet, opts.Namespace, appSet); err != nil {
		return fmt.Errorf("creating ApplicationSet: %w", err)
	}
	return nil
}

func (m *Manager) Get(ctx context.Context, name, namespace string) (*AppSetInfo, error) {
	m.logger.Info("gitops.Get", "name", name, "namespace", namespace)
	if namespace == "" {
		namespace = DefaultNamespace
	}

	obj, err := m.client.Get(ctx, client.GVRApplicationSet, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("getting ApplicationSet %s: %w", name, err)
	}
	return parseAppSetInfo(obj.Object), nil
}

func (m *Manager) List(ctx context.Context, namespace string) ([]AppSetInfo, error) {
	m.logger.Info("gitops.List", "namespace", namespace)
	if namespace == "" {
		namespace = DefaultNamespace
	}

	list, err := m.client.List(ctx, client.GVRApplicationSet, namespace, "acmlab.redhat.com/gitops")
	if err != nil {
		return nil, fmt.Errorf("listing ApplicationSets: %w", err)
	}
	infos := make([]AppSetInfo, 0, len(list.Items))
	for _, item := range list.Items {
		infos = append(infos, *parseAppSetInfo(item.Object))
	}
	return infos, nil
}

func (m *Manager) Delete(ctx context.Context, name, namespace string) (bool, error) {
	m.logger.Info("gitops.Delete", "name", name, "namespace", namespace)
	if namespace == "" {
		namespace = DefaultNamespace
	}

	_, err := m.client.Get(ctx, client.GVRApplicationSet, namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking ApplicationSet %s: %w", name, err)
	}

	if err := m.client.DeleteIfExists(ctx, client.GVRApplicationSet, namespace, name); err != nil {
		return false, fmt.Errorf("deleting ApplicationSet %s: %w", name, err)
	}
	return true, nil
}

func (m *Manager) Sync(ctx context.Context, name, namespace string) error {
	m.logger.Info("gitops.Sync", "name", name, "namespace", namespace)
	if namespace == "" {
		namespace = DefaultNamespace
	}

	_, err := m.client.Get(ctx, client.GVRApplicationSet, namespace, name)
	if err != nil {
		return fmt.Errorf("getting ApplicationSet %s for sync: %w", name, err)
	}

	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": map[string]interface{}{
				"argocd.argoproj.io/refresh": time.Now().UTC().Format(time.RFC3339),
			},
		},
	}
	patchData, _ := json.Marshal(patch)
	if _, err := m.client.Patch(ctx, client.GVRApplicationSet, namespace, name, types.MergePatchType, patchData); err != nil {
		return fmt.Errorf("patching ApplicationSet %s for sync: %w", name, err)
	}
	return nil
}

func applyDefaults(opts *AppSetOpts) {
	if opts.Namespace == "" {
		opts.Namespace = DefaultNamespace
	}
	if opts.Revision == "" {
		opts.Revision = "main"
	}
	if opts.Generator == "" {
		opts.Generator = "placement"
	}
	if opts.Project == "" {
		opts.Project = "default"
	}
}

func parseAppSetInfo(obj map[string]interface{}) *AppSetInfo {
	info := &AppSetInfo{Status: "Pending"}

	meta, _ := obj["metadata"].(map[string]interface{})
	if meta != nil {
		info.Name, _ = meta["name"].(string)
		info.Namespace, _ = meta["namespace"].(string)
		if labels, ok := meta["labels"].(map[string]interface{}); ok {
			info.Generator, _ = labels["acmlab.redhat.com/generator"].(string)
		}
	}

	spec, _ := obj["spec"].(map[string]interface{})
	if spec != nil {
		tmpl, _ := spec["template"].(map[string]interface{})
		if tmpl != nil {
			tmplSpec, _ := tmpl["spec"].(map[string]interface{})
			if tmplSpec != nil {
				source, _ := tmplSpec["source"].(map[string]interface{})
				if source != nil {
					info.RepoURL, _ = source["repoURL"].(string)
					info.Path, _ = source["path"].(string)
				}
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
			if cond["type"] == "ResourcesUpToDate" && cond["status"] == "True" {
				info.Status = "Synced"
			}
		}
		if resources, ok := status["resources"].([]interface{}); ok {
			info.AppCount = len(resources)
		}
	}

	return info
}
