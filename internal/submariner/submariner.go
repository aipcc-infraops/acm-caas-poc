package submariner

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

type SubmarinerStatus struct {
	ClusterSet string          `json:"clusterSet"`
	Clusters   []ClusterStatus `json:"clusters"`
	Connected  bool            `json:"connected"`
}

type ClusterStatus struct {
	Name               string `json:"name"`
	AddonAvailable     bool   `json:"addonAvailable"`
	BrokerConfigured   bool   `json:"brokerConfigured"`
	GatewayReady       bool   `json:"gatewayReady"`
	AgentReady         bool   `json:"agentReady"`
	Connections        int    `json:"connections"`
	ConnectionDegraded bool   `json:"connectionDegraded"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
}

type SubmarinerInfo struct {
	ClusterSet string `json:"clusterSet"`
	Clusters   int    `json:"clusters"`
	Enabled    bool   `json:"enabled"`
}

type Manager struct {
	client *client.Client
	cfg    config.Config
	logger *slog.Logger
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger}
}

type EnableOpts struct {
	Globalnet bool
}

func (m *Manager) Enable(ctx context.Context, clusterSet string, opts EnableOpts) error {
	m.logger.Info("submariner.Enable", "clusterSet", clusterSet, "globalnet", opts.Globalnet)

	clusters, err := m.clustersInSet(ctx, clusterSet)
	if err != nil {
		return err
	}
	if len(clusters) == 0 {
		return fmt.Errorf("no clusters found in ClusterSet %q", clusterSet)
	}

	for i, name := range clusters {
		credsSecret, err := m.lookupCredentialsSecret(ctx, name)
		if err != nil {
			m.logger.Info("submariner.Enable", "cluster", name, "creds_lookup", err.Error())
		}

		addOn := buildSubmarinerAddOn(name)
		if err := m.client.CreateIfNotExists(ctx, client.GVRManagedClusterAddOn, name, addOn); err != nil {
			return fmt.Errorf("creating submariner addon for %s: %w", name, err)
		}

		cfgOpts := SubmarinerConfigOpts{CredentialsSecret: credsSecret}
		if opts.Globalnet {
			cfgOpts.GlobalCIDR = defaultGlobalCIDR(i)
		}
		cfg := buildSubmarinerConfig(name, cfgOpts)
		if err := m.createOrUpdateSubmarinerConfig(ctx, name, cfg); err != nil {
			return fmt.Errorf("creating/updating submariner config for %s: %w", name, err)
		}

		patch, _ := json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{"submariner": "enabled"},
			},
		})
		if _, err := m.client.Patch(ctx, client.GVRManagedCluster, "", name, types.MergePatchType, patch); err != nil {
			return fmt.Errorf("labelling cluster %s: %w", name, err)
		}
	}
	return nil
}

func (m *Manager) createOrUpdateSubmarinerConfig(ctx context.Context, namespace string, desired *unstructured.Unstructured) error {
	existing, err := m.client.Get(ctx, client.GVRSubmarinerConfig, namespace, "submariner")
	if err != nil {
		return m.client.CreateIfNotExists(ctx, client.GVRSubmarinerConfig, namespace, desired)
	}
	desired.SetResourceVersion(existing.GetResourceVersion())
	_, err = m.client.Update(ctx, client.GVRSubmarinerConfig, namespace, desired)
	return err
}

func (m *Manager) lookupCredentialsSecret(ctx context.Context, cluster string) (string, error) {
	cd, err := m.client.Get(ctx, client.GVRClusterDeployment, cluster, cluster)
	if err != nil {
		return "", fmt.Errorf("ClusterDeployment not found for %s: %w", cluster, err)
	}
	for _, platform := range []string{"aws", "gcp", "azure", "ibmcloud", "openstack", "vsphere"} {
		ref, found, _ := unstructured.NestedString(cd.Object, "spec", "platform", platform, "credentialsSecretRef", "name")
		if found && ref != "" {
			return ref, nil
		}
	}
	return "", fmt.Errorf("no cloud credentials found in ClusterDeployment for %s", cluster)
}

func (m *Manager) WaitForReady(ctx context.Context, clusterSet string, timeout time.Duration) (*SubmarinerStatus, error) {
	m.logger.Info("submariner.WaitForReady", "clusterSet", clusterSet, "timeout", timeout)
	deadline := time.Now().Add(timeout)
	var lastStatus *SubmarinerStatus

	for {
		if time.Now().After(deadline) {
			reason := summarizeBlocker(lastStatus)
			return lastStatus, fmt.Errorf("timed out after %s waiting for Submariner readiness on %q: %s", timeout, clusterSet, reason)
		}

		status, err := m.Status(ctx, clusterSet)
		if err != nil {
			m.logger.Info("submariner.WaitForReady", "poll_error", err.Error())
		} else {
			lastStatus = status
			if status.Connected {
				return status, nil
			}
		}

		select {
		case <-ctx.Done():
			return lastStatus, ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
}

func summarizeBlocker(status *SubmarinerStatus) string {
	if status == nil {
		return "no status available"
	}
	for _, cs := range status.Clusters {
		if !cs.AddonAvailable {
			return fmt.Sprintf("add-on not available on %s", cs.Name)
		}
		if !cs.BrokerConfigured {
			return fmt.Sprintf("broker config not applied on %s — check cloud credentials in SubmarinerConfig credentialsSecret", cs.Name)
		}
		if !cs.GatewayReady {
			return fmt.Sprintf("no gateway node labelled on %s — the add-on controller needs valid cloud credentials to provision a gateway", cs.Name)
		}
		if !cs.AgentReady {
			return fmt.Sprintf("agent not ready on %s", cs.Name)
		}
		if cs.ConnectionDegraded {
			msg := fmt.Sprintf("connections degraded on %s", cs.Name)
			if cs.Message != "" {
				msg += ": " + cs.Message
			}
			return msg
		}
		if cs.Connections == 0 {
			return fmt.Sprintf("0 connections on %s", cs.Name)
		}
	}
	return "unknown — check 'acmlab submariner diagnose'"
}

func defaultGlobalCIDR(index int) string {
	return fmt.Sprintf("242.%d.0.0/16", index)
}

func (m *Manager) Disable(ctx context.Context, clusterSet string) error {
	m.logger.Info("submariner.Disable", "clusterSet", clusterSet)

	clusters, err := m.clustersInSet(ctx, clusterSet)
	if err != nil {
		return err
	}

	for _, name := range clusters {
		_ = m.client.DeleteIfExists(ctx, client.GVRManagedClusterAddOn, name, "submariner")
		_ = m.client.DeleteIfExists(ctx, client.GVRSubmarinerConfig, name, "submariner")

		patch, _ := json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{"submariner": nil},
			},
		})
		_, _ = m.client.Patch(ctx, client.GVRManagedCluster, "", name, types.MergePatchType, patch)
	}

	m.waitForAddonCleanup(ctx, clusters)
	return nil
}

func (m *Manager) waitForAddonCleanup(ctx context.Context, clusters []string) {
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		allGone := true
		for _, name := range clusters {
			_, err := m.client.Get(ctx, client.GVRManagedClusterAddOn, name, "submariner")
			if err == nil {
				allGone = false
				break
			}
		}
		if allGone {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
	m.logger.Info("submariner.waitForAddonCleanup", "result", "timed out waiting for addon removal")
}

func (m *Manager) Status(ctx context.Context, clusterSet string) (*SubmarinerStatus, error) {
	m.logger.Info("submariner.Status", "clusterSet", clusterSet)

	clusters, err := m.clustersInSet(ctx, clusterSet)
	if err != nil {
		return nil, err
	}
	if len(clusters) == 0 {
		return nil, fmt.Errorf("no clusters found in ClusterSet %q", clusterSet)
	}

	status := &SubmarinerStatus{ClusterSet: clusterSet, Connected: true}
	for _, name := range clusters {
		addOn, err := m.client.Get(ctx, client.GVRManagedClusterAddOn, name, "submariner")
		if err != nil {
			status.Clusters = append(status.Clusters, ClusterStatus{Name: name})
			status.Connected = false
			continue
		}
		cs := parseClusterStatus(name, addOn.Object)
		if !cs.BrokerConfigured || !cs.GatewayReady || !cs.AgentReady || cs.ConnectionDegraded || cs.Connections == 0 {
			status.Connected = false
		}
		status.Clusters = append(status.Clusters, cs)
	}
	return status, nil
}

func (m *Manager) List(ctx context.Context) ([]SubmarinerInfo, error) {
	m.logger.Info("submariner.List")

	list, err := m.client.List(ctx, client.GVRManagedCluster, "", "submariner=enabled")
	if err != nil {
		return nil, fmt.Errorf("listing submariner clusters: %w", err)
	}

	objs := make([]map[string]interface{}, len(list.Items))
	for i, item := range list.Items {
		objs[i] = item.Object
	}
	return groupByClusterSet(objs), nil
}

func (m *Manager) clustersInSet(ctx context.Context, clusterSet string) ([]string, error) {
	sel := fmt.Sprintf("cluster.open-cluster-management.io/clusterset=%s", clusterSet)
	list, err := m.client.List(ctx, client.GVRManagedCluster, "", sel)
	if err != nil {
		return nil, fmt.Errorf("listing clusters in set %q: %w", clusterSet, err)
	}
	names := make([]string, len(list.Items))
	for i, item := range list.Items {
		names[i] = item.GetName()
	}
	return names, nil
}
