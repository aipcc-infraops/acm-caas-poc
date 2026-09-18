package discovery

import (
	"context"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

type Manager struct {
	client *client.Client
	cfg    config.Config
	logger *slog.Logger
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger}
}

type EnableDiscoveryOpts struct {
	Namespace  string
	OCMToken   string
	LastActive int
	Versions   []string
}

type DiscoveredClusterInfo struct {
	Name             string `json:"name"`
	DisplayName      string `json:"displayName"`
	CloudProvider    string `json:"cloudProvider"`
	Region           string `json:"region"`
	OpenshiftVersion string `json:"openshiftVersion"`
	Status           string `json:"status"`
	ApiURL           string `json:"apiURL"`
}

type DiscoveryStatusInfo struct {
	Namespace      string `json:"namespace"`
	Credential     string `json:"credential"`
	LastActive     int64  `json:"lastActive"`
	ClusterCount   int    `json:"clusterCount"`
}

func (m *Manager) EnableDiscovery(ctx context.Context, opts EnableDiscoveryOpts) error {
	m.logger.Info("discovery.EnableDiscovery", "namespace", opts.Namespace)

	if opts.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if opts.OCMToken == "" {
		return fmt.Errorf("OCM API token is required")
	}
	if opts.LastActive <= 0 {
		opts.LastActive = 7
	}

	secret := buildDiscoverySecret(opts.Namespace, opts.OCMToken)
	if err := m.client.CreateIfNotExists(ctx, client.GVRSecret, opts.Namespace, secret); err != nil {
		return fmt.Errorf("creating OCM credential secret: %w", err)
	}

	dc := buildDiscoveryConfig(opts.Namespace, opts.LastActive, opts.Versions)
	if err := m.client.CreateIfNotExists(ctx, client.GVRDiscoveryConfig, opts.Namespace, dc); err != nil {
		return fmt.Errorf("creating DiscoveryConfig: %w", err)
	}

	return nil
}

func (m *Manager) DisableDiscovery(ctx context.Context, namespace string) error {
	m.logger.Info("discovery.DisableDiscovery", "namespace", namespace)

	_, dcErr := m.client.Get(ctx, client.GVRDiscoveryConfig, namespace, "discovery")
	_, secretErr := m.client.Get(ctx, client.GVRSecret, namespace, "ocm-api-token")

	if apierrors.IsNotFound(dcErr) && apierrors.IsNotFound(secretErr) {
		return fmt.Errorf("no discovery configuration found in namespace %s", namespace)
	}

	_ = m.client.DeleteIfExists(ctx, client.GVRDiscoveryConfig, namespace, "discovery")
	_ = m.client.DeleteIfExists(ctx, client.GVRSecret, namespace, "ocm-api-token")
	return nil
}

func (m *Manager) ListDiscovered(ctx context.Context, namespace string) ([]DiscoveredClusterInfo, error) {
	m.logger.Info("discovery.ListDiscovered", "namespace", namespace)

	list, err := m.client.List(ctx, client.GVRDiscoveredCluster, namespace, "")
	if err != nil {
		return nil, fmt.Errorf("listing discovered clusters: %w", err)
	}

	results := make([]DiscoveredClusterInfo, 0, len(list.Items))
	for _, item := range list.Items {
		info := extractDiscoveredInfo(item)
		results = append(results, info)
	}
	return results, nil
}

func (m *Manager) ImportDiscovered(ctx context.Context, name, namespace string) error {
	m.logger.Info("discovery.ImportDiscovered", "name", name, "namespace", namespace)

	obj, err := m.client.Get(ctx, client.GVRDiscoveredCluster, namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("discovered cluster %s not found in namespace %s", name, namespace)
		}
		return fmt.Errorf("getting discovered cluster: %w", err)
	}

	displayName, _, _ := unstructured.NestedString(obj.Object, "spec", "displayName")
	if displayName == "" {
		displayName = name
	}

	if err := m.ensureNamespace(ctx, displayName); err != nil {
		return fmt.Errorf("creating namespace for import: %w", err)
	}

	mc := buildManagedClusterForImport(displayName)
	if err := m.client.CreateIfNotExists(ctx, client.GVRManagedCluster, "", mc); err != nil {
		return fmt.Errorf("creating ManagedCluster: %w", err)
	}

	kac := buildKlusterletAddonConfig(displayName)
	if err := m.client.CreateIfNotExists(ctx, client.GVRKlusterletAddonConfig, displayName, kac); err != nil {
		return fmt.Errorf("creating KlusterletAddonConfig: %w", err)
	}

	return nil
}

func (m *Manager) DiscoveryStatus(ctx context.Context, namespace string) (*DiscoveryStatusInfo, error) {
	m.logger.Info("discovery.DiscoveryStatus", "namespace", namespace)

	dc, err := m.client.Get(ctx, client.GVRDiscoveryConfig, namespace, "discovery")
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("no discovery configuration found in namespace %s", namespace)
		}
		return nil, fmt.Errorf("getting DiscoveryConfig: %w", err)
	}

	credential, _, _ := unstructured.NestedString(dc.Object, "spec", "credential")
	lastActive, _, _ := unstructured.NestedInt64(dc.Object, "spec", "filters", "lastActive")

	list, err := m.client.List(ctx, client.GVRDiscoveredCluster, namespace, "")
	clusterCount := 0
	if err == nil {
		clusterCount = len(list.Items)
	}

	return &DiscoveryStatusInfo{
		Namespace:    namespace,
		Credential:   credential,
		LastActive:   lastActive,
		ClusterCount: clusterCount,
	}, nil
}

func (m *Manager) ensureNamespace(ctx context.Context, name string) error {
	ns := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": name,
			},
		},
	}
	return m.client.CreateIfNotExists(ctx, client.GVRNamespace, "", ns)
}

func extractDiscoveredInfo(item unstructured.Unstructured) DiscoveredClusterInfo {
	spec := item.Object["spec"]
	info := DiscoveredClusterInfo{Name: item.GetName()}

	if specMap, ok := spec.(map[string]interface{}); ok {
		if v, ok := specMap["displayName"].(string); ok {
			info.DisplayName = v
		}
		if v, ok := specMap["cloudProvider"].(string); ok {
			info.CloudProvider = v
		}
		if v, ok := specMap["openshiftVersion"].(string); ok {
			info.OpenshiftVersion = v
		}
		if v, ok := specMap["region"].(string); ok {
			info.Region = v
		}
		if v, ok := specMap["apiURL"].(string); ok {
			info.ApiURL = v
		}
		if v, ok := specMap["status"].(string); ok {
			info.Status = v
		}
	}
	return info
}

func buildManagedClusterForImport(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": name,
				"labels": map[string]interface{}{
					"name":                        name,
					"acmlab.redhat.com/managed":   "true",
					"acmlab.redhat.com/discovery": "true",
					"created-via":                 "discovery",
					"cluster.open-cluster-management.io/clusterset": "default",
				},
			},
			"spec": map[string]interface{}{
				"hubAcceptsClient": true,
			},
		},
	}
}

func buildKlusterletAddonConfig(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "agent.open-cluster-management.io/v1",
			"kind":       "KlusterletAddonConfig",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": name,
			},
			"spec": map[string]interface{}{
				"applicationManager":   map[string]interface{}{"enabled": true},
				"certPolicyController": map[string]interface{}{"enabled": true},
				"policyController":     map[string]interface{}{"enabled": true},
				"searchCollector":      map[string]interface{}{"enabled": true},
			},
		},
	}
}
