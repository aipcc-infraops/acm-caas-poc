package addon

import (
	"context"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

const DefaultNamespace = "open-cluster-management"

type AddOnInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Status      string `json:"status"`
}

type AddOnDetail struct {
	Name             string   `json:"name"`
	DisplayName      string   `json:"displayName"`
	Description      string   `json:"description"`
	InstallNamespace string   `json:"installNamespace"`
	Configs          []string `json:"configs,omitempty"`
	Status           string   `json:"status"`
}

type AddOnConfigOpts struct {
	Name             string
	Namespace        string
	InstallNamespace string
	Values           map[string]string
}

type ConfigInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
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

func (m *Manager) ListAddOns(ctx context.Context) ([]AddOnInfo, error) {
	m.logger.Info("addon.ListAddOns")

	list, err := m.client.List(ctx, client.GVRClusterManagementAddOn, "", "")
	if err != nil {
		return nil, fmt.Errorf("listing ClusterManagementAddOns: %w", err)
	}

	infos := make([]AddOnInfo, 0, len(list.Items))
	for _, item := range list.Items {
		infos = append(infos, parseAddOnInfo(item.Object))
	}
	return infos, nil
}

func (m *Manager) GetAddOn(ctx context.Context, name string) (*AddOnDetail, error) {
	m.logger.Info("addon.GetAddOn", "name", name)

	obj, err := m.client.Get(ctx, client.GVRClusterManagementAddOn, "", name)
	if err != nil {
		return nil, fmt.Errorf("getting ClusterManagementAddOn %s: %w", name, err)
	}

	return parseAddOnDetail(obj.Object), nil
}

func (m *Manager) ConfigureAddOn(ctx context.Context, opts AddOnConfigOpts) error {
	m.logger.Info("addon.ConfigureAddOn", "name", opts.Name)

	if opts.Namespace == "" {
		opts.Namespace = DefaultNamespace
	}

	cfg := buildAddOnDeploymentConfig(opts)
	if err := m.client.CreateIfNotExists(ctx, client.GVRAddOnDeploymentConfig, opts.Namespace, cfg); err != nil {
		return fmt.Errorf("creating AddOnDeploymentConfig %s: %w", opts.Name, err)
	}
	return nil
}

func (m *Manager) RemoveConfig(ctx context.Context, name, namespace string) error {
	m.logger.Info("addon.RemoveConfig", "name", name)

	if namespace == "" {
		namespace = DefaultNamespace
	}

	_, err := m.client.Get(ctx, client.GVRAddOnDeploymentConfig, namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("AddOnDeploymentConfig %s not found", name)
		}
		return fmt.Errorf("checking AddOnDeploymentConfig %s: %w", name, err)
	}

	return m.client.DeleteIfExists(ctx, client.GVRAddOnDeploymentConfig, namespace, name)
}

func (m *Manager) ListConfigs(ctx context.Context) ([]ConfigInfo, error) {
	m.logger.Info("addon.ListConfigs")

	list, err := m.client.List(ctx, client.GVRAddOnDeploymentConfig, "", "")
	if err != nil {
		return nil, fmt.Errorf("listing AddOnDeploymentConfigs: %w", err)
	}

	infos := make([]ConfigInfo, 0, len(list.Items))
	for _, item := range list.Items {
		infos = append(infos, parseConfigInfo(item.Object))
	}
	return infos, nil
}

func parseAddOnInfo(obj map[string]interface{}) AddOnInfo {
	info := AddOnInfo{Status: "Available"}

	meta, _ := obj["metadata"].(map[string]interface{})
	if meta != nil {
		info.Name, _ = meta["name"].(string)
	}

	spec, _ := obj["spec"].(map[string]interface{})
	if spec != nil {
		if addOnMeta, ok := spec["addOnMeta"].(map[string]interface{}); ok {
			info.DisplayName, _ = addOnMeta["displayName"].(string)
		}
	}
	if info.DisplayName == "" {
		info.DisplayName = info.Name
	}

	return info
}

func parseAddOnDetail(obj map[string]interface{}) *AddOnDetail {
	detail := &AddOnDetail{Status: "Available"}

	meta, _ := obj["metadata"].(map[string]interface{})
	if meta != nil {
		detail.Name, _ = meta["name"].(string)
	}

	spec, _ := obj["spec"].(map[string]interface{})
	if spec != nil {
		addOnMeta, _ := spec["addOnMeta"].(map[string]interface{})
		if addOnMeta != nil {
			detail.DisplayName, _ = addOnMeta["displayName"].(string)
			detail.Description, _ = addOnMeta["description"].(string)
		}

		installStrategy, _ := spec["installStrategy"].(map[string]interface{})
		if installStrategy != nil {
			detail.InstallNamespace, _ = installStrategy["namespace"].(string)
		}

		configs, _ := spec["supportedConfigs"].([]interface{})
		for _, raw := range configs {
			cfg, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if name, ok := cfg["group"].(string); ok {
				detail.Configs = append(detail.Configs, name)
			}
		}
	}

	if detail.DisplayName == "" {
		detail.DisplayName = detail.Name
	}

	return detail
}

func parseConfigInfo(obj map[string]interface{}) ConfigInfo {
	info := ConfigInfo{Status: "Active"}

	meta, _ := obj["metadata"].(map[string]interface{})
	if meta != nil {
		info.Name, _ = meta["name"].(string)
		info.Namespace, _ = meta["namespace"].(string)
	}

	return info
}
