package importing

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
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

type ImportOptions struct {
	Name       string
	Kubeconfig []byte
	Labels     map[string]string
	ClusterSet string
}

type ImportResult struct {
	Name       string `json:"name"`
	AutoImport bool   `json:"autoImport"`
	Message    string `json:"message"`
}

// Import creates a ManagedCluster, its namespace, KlusterletAddonConfig,
// and an auto-import secret containing the spoke kubeconfig. ACM's import
// controller picks up the secret and applies klusterlet manifests on the
// spoke automatically.
func (m *Manager) Import(ctx context.Context, opts ImportOptions) (*ImportResult, error) {
	m.logger.Info("importing.Import", "cluster", opts.Name)
	name := opts.Name

	if err := m.ensureNamespace(ctx, name); err != nil {
		return nil, fmt.Errorf("creating namespace: %w", err)
	}

	if err := m.createManagedCluster(ctx, name, opts.Labels, opts.ClusterSet); err != nil {
		return nil, fmt.Errorf("creating ManagedCluster: %w", err)
	}

	if err := m.createKlusterletAddonConfig(ctx, name); err != nil {
		return nil, fmt.Errorf("creating KlusterletAddonConfig: %w", err)
	}

	autoImport := len(opts.Kubeconfig) > 0
	if autoImport {
		if err := m.createAutoImportSecret(ctx, name, opts.Kubeconfig); err != nil {
			return nil, fmt.Errorf("creating auto-import secret: %w", err)
		}
	}

	var msg string
	if autoImport {
		msg = fmt.Sprintf("Cluster %s registered for import (auto-import enabled)", name)
	} else {
		msg = fmt.Sprintf("Cluster %s registered for import — apply import manifests manually on the spoke", name)
	}

	return &ImportResult{
		Name:       name,
		AutoImport: autoImport,
		Message:    msg,
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
	_, err := m.client.Create(ctx, client.GVRNamespace, "", ns)
	if errors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func (m *Manager) createManagedCluster(ctx context.Context, name string, labels map[string]string, clusterSet string) error {
	allLabels := map[string]interface{}{
		"name": name,
	}
	if clusterSet != "" {
		allLabels["cluster.open-cluster-management.io/clusterset"] = clusterSet
	} else {
		allLabels["cluster.open-cluster-management.io/clusterset"] = "default"
	}
	for k, v := range labels {
		allLabels[k] = v
	}

	mc := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name":   name,
				"labels": allLabels,
			},
			"spec": map[string]interface{}{
				"hubAcceptsClient": true,
			},
		},
	}

	_, err := m.client.Create(ctx, client.GVRManagedCluster, "", mc)
	if errors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func (m *Manager) createKlusterletAddonConfig(ctx context.Context, name string) error {
	kac := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "agent.open-cluster-management.io/v1",
			"kind":       "KlusterletAddonConfig",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": name,
			},
			"spec": map[string]interface{}{
				"applicationManager":    map[string]interface{}{"enabled": true},
				"certPolicyController":  map[string]interface{}{"enabled": true},
				"policyController":      map[string]interface{}{"enabled": true},
				"searchCollector":       map[string]interface{}{"enabled": true},
			},
		},
	}

	_, err := m.client.Create(ctx, client.GVRKlusterletAddonConfig, name, kac)
	if errors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func (m *Manager) createAutoImportSecret(ctx context.Context, name string, kubeconfig []byte) error {
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "auto-import-secret",
				"namespace": name,
			},
			"type": "Opaque",
			"data": map[string]interface{}{
				"kubeconfig": base64.StdEncoding.EncodeToString(kubeconfig),
			},
		},
	}

	_, err := m.client.Create(ctx, client.GVRSecret, name, secret)
	if errors.IsAlreadyExists(err) {
		return m.updateAutoImportSecret(ctx, name, kubeconfig)
	}
	return err
}

func (m *Manager) updateAutoImportSecret(ctx context.Context, name string, kubeconfig []byte) error {
	existing, err := m.client.Get(ctx, client.GVRSecret, name, "auto-import-secret")
	if err != nil {
		return err
	}
	if err := unstructured.SetNestedField(existing.Object, base64.StdEncoding.EncodeToString(kubeconfig), "data", "kubeconfig"); err != nil {
		return err
	}
	_, err = m.client.Update(ctx, client.GVRSecret, name, existing)
	return err
}

// Detach removes a ManagedCluster from ACM. This does NOT destroy the
// underlying cluster — it just removes it from ACM management.
func (m *Manager) Detach(ctx context.Context, name string) error {
	m.logger.Info("importing.Detach", "cluster", name)
	err := m.client.Delete(ctx, client.GVRManagedCluster, "", name)
	if errors.IsNotFound(err) {
		return nil
	}
	return err
}

// WaitForImport polls the ManagedCluster until ManagedClusterConditionAvailable=True.
func (m *Manager) WaitForImport(ctx context.Context, name string, timeout time.Duration) error {
	m.logger.Info("importing.WaitForImport", "cluster", name)
	deadline := time.After(timeout)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timed out waiting for cluster %s to become available", name)
		case <-ticker.C:
			mc, err := m.client.Get(ctx, client.GVRManagedCluster, "", name)
			if err != nil {
				continue
			}
			available, joined := extractConditions(mc)
			if available == "True" && joined == "True" {
				return nil
			}
		}
	}
}

// GetImportStatus returns the current import state of a cluster.
func (m *Manager) GetImportStatus(ctx context.Context, name string) (*ImportStatus, error) {
	m.logger.Info("importing.GetImportStatus", "cluster", name)
	mc, err := m.client.Get(ctx, client.GVRManagedCluster, "", name)
	if err != nil {
		return nil, fmt.Errorf("getting ManagedCluster %s: %w", name, err)
	}

	available, joined := extractConditions(mc)

	status := &ImportStatus{
		Name:      name,
		Available: available,
		Joined:    joined,
	}

	annotations, _, _ := unstructured.NestedStringMap(mc.Object, "metadata", "annotations")
	if v, ok := annotations["open-cluster-management/created-via"]; ok {
		status.CreatedVia = v
	}

	labels, _, _ := unstructured.NestedStringMap(mc.Object, "metadata", "labels")
	status.Labels = labels

	_, err = m.client.Get(ctx, client.GVRSecret, name, "auto-import-secret")
	if err == nil {
		status.AutoImport = true
	}

	return status, nil
}

type ImportStatus struct {
	Name       string            `json:"name"`
	Available  string            `json:"available"`
	Joined     string            `json:"joined"`
	CreatedVia string            `json:"createdVia,omitempty"`
	AutoImport bool              `json:"autoImport"`
	Labels     map[string]string `json:"labels,omitempty"`
}

func extractConditions(mc *unstructured.Unstructured) (available, joined string) {
	conditions, _, _ := unstructured.NestedSlice(mc.Object, "status", "conditions")
	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := cond["type"].(string)
		condStatus, _ := cond["status"].(string)
		switch condType {
		case "ManagedClusterConditionAvailable":
			available = condStatus
		case "ManagedClusterJoined":
			joined = condStatus
		}
	}
	return
}

// IsImported checks if a cluster was imported (not provisioned via Hive).
func (m *Manager) IsImported(ctx context.Context, name string) (bool, error) {
	m.logger.Info("importing.IsImported", "cluster", name)
	_, err := m.client.Get(ctx, client.GVRClusterDeployment, name, name)
	if errors.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, nil
}

// ListImported returns clusters that were imported (no ClusterDeployment).
func (m *Manager) ListImported(ctx context.Context) ([]ImportStatus, error) {
	m.logger.Info("importing.ListImported")
	mcList, err := m.client.List(ctx, client.GVRManagedCluster, "", "")
	if err != nil {
		return nil, fmt.Errorf("listing managed clusters: %w", err)
	}

	var imported []ImportStatus
	for _, mc := range mcList.Items {
		name := mc.GetName()
		if name == "local-cluster" {
			continue
		}
		_, err := m.client.Get(ctx, client.GVRClusterDeployment, name, name)
		if errors.IsNotFound(err) {
			available, joined := extractConditions(&mc)
			labels, _, _ := unstructured.NestedStringMap(mc.Object, "metadata", "labels")
			annotations, _, _ := unstructured.NestedStringMap(mc.Object, "metadata", "annotations")
			status := ImportStatus{
				Name:      name,
				Available: available,
				Joined:    joined,
				Labels:    labels,
			}
			if v, ok := annotations["open-cluster-management/created-via"]; ok {
				status.CreatedVia = v
			}
			imported = append(imported, status)
		}
	}

	return imported, nil
}
