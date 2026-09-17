package access

import (
	"context"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

const msaName = "acmlab-access"

type AccessOpts struct {
	TTL   string
	Roles []string
}

type AccessStatus struct {
	Cluster        string `json:"cluster"`
	Enabled        bool   `json:"enabled"`
	TokenAvailable bool   `json:"tokenAvailable"`
	TokenRotation  string `json:"tokenRotation"`
	AddonHealthy   bool   `json:"addonHealthy"`
	LastRotation   string `json:"lastRotation,omitempty"`
}

type AccessInfo struct {
	Cluster        string `json:"cluster"`
	Enabled        bool   `json:"enabled"`
	TokenAvailable bool   `json:"tokenAvailable"`
	AddonStatus    string `json:"addonStatus"`
}

type Manager struct {
	client *client.Client
	cfg    config.Config
	logger *slog.Logger
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger}
}

func (m *Manager) Enable(ctx context.Context, cluster string, opts AccessOpts) error {
	m.logger.Info("access.Enable", "cluster", cluster)
	if opts.TTL == "" {
		opts.TTL = "720h"
	}

	msaAddon := buildManagedServiceAccountAddOn(cluster)
	if err := m.client.CreateIfNotExists(ctx, client.GVRManagedClusterAddOn, cluster, msaAddon); err != nil {
		return fmt.Errorf("enabling managed-serviceaccount addon on %s: %w", cluster, err)
	}

	proxyAddon := buildClusterProxyAddOn(cluster)
	if err := m.client.CreateIfNotExists(ctx, client.GVRManagedClusterAddOn, cluster, proxyAddon); err != nil {
		return fmt.Errorf("enabling cluster-proxy addon on %s: %w", cluster, err)
	}

	msa := buildManagedServiceAccount(cluster, opts)
	if err := m.client.CreateIfNotExists(ctx, client.GVRManagedServiceAccount, cluster, msa); err != nil {
		return fmt.Errorf("creating ManagedServiceAccount on %s: %w", cluster, err)
	}

	return nil
}

func (m *Manager) Disable(ctx context.Context, cluster string) (bool, error) {
	m.logger.Info("access.Disable", "cluster", cluster)

	_, err := m.client.Get(ctx, client.GVRManagedServiceAccount, cluster, msaName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking ManagedServiceAccount on %s: %w", cluster, err)
	}

	if err := m.client.DeleteIfExists(ctx, client.GVRManagedServiceAccount, cluster, msaName); err != nil {
		return false, fmt.Errorf("removing ManagedServiceAccount on %s: %w", cluster, err)
	}
	if err := m.client.DeleteIfExists(ctx, client.GVRManagedClusterAddOn, cluster, "cluster-proxy"); err != nil {
		return false, fmt.Errorf("removing cluster-proxy addon on %s: %w", cluster, err)
	}
	if err := m.client.DeleteIfExists(ctx, client.GVRManagedClusterAddOn, cluster, "managed-serviceaccount"); err != nil {
		return false, fmt.Errorf("removing managed-serviceaccount addon on %s: %w", cluster, err)
	}

	return true, nil
}

func (m *Manager) GetStatus(ctx context.Context, cluster string) (*AccessStatus, error) {
	m.logger.Info("access.GetStatus", "cluster", cluster)

	if _, err := m.client.Get(ctx, client.GVRManagedCluster, "", cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("cluster %s not found", cluster)
		}
		return nil, fmt.Errorf("checking cluster %s: %w", cluster, err)
	}

	msaObj, err := m.client.Get(ctx, client.GVRManagedServiceAccount, cluster, msaName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return &AccessStatus{Cluster: cluster, Enabled: false}, nil
		}
		return nil, fmt.Errorf("getting ManagedServiceAccount on %s: %w", cluster, err)
	}

	proxyObj, err := m.client.Get(ctx, client.GVRManagedClusterAddOn, cluster, "cluster-proxy")
	var proxyMap map[string]interface{}
	if err == nil {
		proxyMap = proxyObj.Object
	}

	return parseAccessStatus(cluster, msaObj.Object, proxyMap), nil
}

type ProxyStatus struct {
	Cluster  string `json:"cluster"`
	Enabled  bool   `json:"enabled"`
	Healthy  bool   `json:"healthy"`
	Endpoint string `json:"endpoint,omitempty"`
}

func (m *Manager) EnableProxy(ctx context.Context, cluster string) error {
	m.logger.Info("access.EnableProxy", "cluster", cluster)

	addon := buildClusterProxyAddOn(cluster)
	if err := m.client.CreateIfNotExists(ctx, client.GVRManagedClusterAddOn, cluster, addon); err != nil {
		return fmt.Errorf("enabling cluster-proxy addon on %s: %w", cluster, err)
	}
	return nil
}

func (m *Manager) DisableProxy(ctx context.Context, cluster string) (bool, error) {
	m.logger.Info("access.DisableProxy", "cluster", cluster)

	_, err := m.client.Get(ctx, client.GVRManagedClusterAddOn, cluster, "cluster-proxy")
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking cluster-proxy addon on %s: %w", cluster, err)
	}

	if err := m.client.DeleteIfExists(ctx, client.GVRManagedClusterAddOn, cluster, "cluster-proxy"); err != nil {
		return false, fmt.Errorf("removing cluster-proxy addon on %s: %w", cluster, err)
	}
	return true, nil
}

func (m *Manager) GetProxyStatus(ctx context.Context, cluster string) (*ProxyStatus, error) {
	m.logger.Info("access.GetProxyStatus", "cluster", cluster)

	obj, err := m.client.Get(ctx, client.GVRManagedClusterAddOn, cluster, "cluster-proxy")
	if err != nil {
		if apierrors.IsNotFound(err) {
			return &ProxyStatus{Cluster: cluster, Enabled: false}, nil
		}
		return nil, fmt.Errorf("getting cluster-proxy addon on %s: %w", cluster, err)
	}

	return parseProxyStatus(cluster, obj.Object), nil
}

func parseProxyStatus(cluster string, obj map[string]interface{}) *ProxyStatus {
	ps := &ProxyStatus{
		Cluster: cluster,
		Enabled: true,
	}

	ps.Healthy = addonIsHealthy(obj)

	status, _ := obj["status"].(map[string]interface{})
	if status != nil {
		configs, _ := status["configReferences"].([]interface{})
		for _, raw := range configs {
			ref, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if ref["name"] == "proxy-server-host" {
				ps.Endpoint, _ = ref["desiredConfig"].(string)
			}
		}
	}

	if ps.Endpoint == "" && ps.Healthy {
		ps.Endpoint = "https://cluster-proxy-addon-user.open-cluster-management.svc:8092/" + cluster
	}

	return ps
}

func (m *Manager) List(ctx context.Context) ([]AccessInfo, error) {
	m.logger.Info("access.List")

	list, err := m.client.List(ctx, client.GVRManagedServiceAccount, "", "")
	if err != nil {
		return nil, fmt.Errorf("listing ManagedServiceAccounts: %w", err)
	}

	seen := make(map[string]bool)
	infos := make([]AccessInfo, 0, len(list.Items))
	for _, item := range list.Items {
		cluster := item.GetNamespace()
		if seen[cluster] {
			continue
		}
		if item.GetName() != msaName {
			continue
		}
		seen[cluster] = true
		infos = append(infos, parseAccessInfo(cluster, item.Object))
	}
	return infos, nil
}
