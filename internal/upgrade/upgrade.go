package upgrade

import (
	"context"
	"fmt"
	"log/slog"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

type UpgradeMethod string

const (
	UpgradeMethodHive       UpgradeMethod = "hive"
	UpgradeMethodManifest   UpgradeMethod = "manifestwork"
	UpgradeMethodReportOnly UpgradeMethod = "report-only"
)

type UpgradeStatus struct {
	Cluster        string        `json:"cluster"`
	ClusterType    string        `json:"clusterType"`
	UpgradeMethod  UpgradeMethod `json:"upgradeMethod"`
	CurrentVersion string        `json:"currentVersion"`
	DesiredVersion string        `json:"desiredVersion,omitempty"`
	Channel        string        `json:"channel,omitempty"`
	UpgradeFailed  bool          `json:"upgradeFailed,omitempty"`
	FailureMessage string        `json:"failureMessage,omitempty"`
	Available      []string      `json:"availableUpdates,omitempty"`
	Progressing    bool          `json:"progressing,omitempty"`
}

type UpgradeableCluster struct {
	Cluster        string        `json:"cluster"`
	CurrentVersion string        `json:"currentVersion"`
	Channel        string        `json:"channel,omitempty"`
	Available      []string      `json:"availableUpdates,omitempty"`
	UpgradeMethod  UpgradeMethod `json:"upgradeMethod"`
}

type HistoryEntry struct {
	Version     string `json:"version"`
	State       string `json:"state"`
	StartedAt   string `json:"startedAt,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
}

type Manager struct {
	client *client.Client
	cfg    config.Config
	logger *slog.Logger
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger}
}

func (m *Manager) GetUpgradeStatus(ctx context.Context, clusterName string) (*UpgradeStatus, error) {
	m.logger.Info("upgrade.GetUpgradeStatus", "cluster", clusterName)

	clusterType, _ := m.client.GetClusterType(ctx, clusterName)
	method := m.detectUpgradeMethod(ctx, clusterName, clusterType)

	status := &UpgradeStatus{
		Cluster:       clusterName,
		ClusterType:   string(clusterType),
		UpgradeMethod: method,
	}

	info, err := m.client.Get(ctx, client.GVRManagedClusterInfo, clusterName, clusterName)
	if err != nil {
		return nil, fmt.Errorf("getting ManagedClusterInfo for %s: %w", clusterName, err)
	}

	switch clusterType {
	case client.ClusterTypeOCP:
		fillOCPStatus(info, status)
	case client.ClusterTypeKubernetes:
		fillK8sStatus(info, status)
	default:
		fillOCPStatus(info, status)
		if status.CurrentVersion == "" {
			fillK8sStatus(info, status)
		}
	}

	if method == UpgradeMethodHive {
		m.fillHiveDesiredVersion(ctx, clusterName, status)
	}

	return status, nil
}

func (m *Manager) ListUpgradeable(ctx context.Context) ([]UpgradeableCluster, error) {
	m.logger.Info("upgrade.ListUpgradeable")

	clusters, err := m.client.List(ctx, client.GVRManagedCluster, "", "")
	if err != nil {
		return nil, fmt.Errorf("listing ManagedClusters: %w", err)
	}

	var result []UpgradeableCluster
	for _, mc := range clusters.Items {
		name := mc.GetName()
		if name == "local-cluster" {
			continue
		}

		status, err := m.GetUpgradeStatus(ctx, name)
		if err != nil {
			m.logger.Warn("upgrade.ListUpgradeable: skipping cluster", "cluster", name, "error", err)
			continue
		}

		if status.UpgradeMethod == UpgradeMethodReportOnly {
			continue
		}

		if len(status.Available) == 0 {
			continue
		}

		result = append(result, UpgradeableCluster{
			Cluster:        name,
			CurrentVersion: status.CurrentVersion,
			Channel:        status.Channel,
			Available:      status.Available,
			UpgradeMethod:  status.UpgradeMethod,
		})
	}

	return result, nil
}

func (m *Manager) SetChannel(ctx context.Context, clusterName, channel string) error {
	m.logger.Info("upgrade.SetChannel", "cluster", clusterName, "channel", channel)

	clusterType, _ := m.client.GetClusterType(ctx, clusterName)
	method := m.detectUpgradeMethod(ctx, clusterName, clusterType)

	if method == UpgradeMethodReportOnly {
		return fmt.Errorf("cluster %s does not support channel changes (type: %s)", clusterName, clusterType)
	}

	return m.applyManifestWork(ctx, clusterName, fmt.Sprintf("%s-channel", clusterName),
		func(cluster, mwName string) *unstructured.Unstructured {
			return buildChannelManifestWork(cluster, mwName, channel)
		})
}

func (m *Manager) StartUpgrade(ctx context.Context, clusterName, version string) error {
	m.logger.Info("upgrade.StartUpgrade", "cluster", clusterName, "version", version)

	clusterType, _ := m.client.GetClusterType(ctx, clusterName)
	method := m.detectUpgradeMethod(ctx, clusterName, clusterType)

	if method == UpgradeMethodReportOnly {
		return fmt.Errorf("cluster %s does not support upgrades (type: %s)", clusterName, clusterType)
	}

	return m.applyManifestWork(ctx, clusterName, fmt.Sprintf("%s-upgrade", clusterName),
		func(cluster, mwName string) *unstructured.Unstructured {
			return buildUpgradeManifestWork(cluster, mwName, version)
		})
}

func (m *Manager) GetHistory(ctx context.Context, clusterName string) ([]HistoryEntry, error) {
	m.logger.Info("upgrade.GetHistory", "cluster", clusterName)

	info, err := m.client.Get(ctx, client.GVRManagedClusterInfo, clusterName, clusterName)
	if err != nil {
		return nil, fmt.Errorf("getting ManagedClusterInfo for %s: %w", clusterName, err)
	}

	history, found, _ := unstructured.NestedSlice(info.Object, "status", "distributionInfo", "ocp", "versionHistory")
	if !found {
		return nil, nil
	}

	var entries []HistoryEntry
	for _, raw := range history {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		e := HistoryEntry{}
		e.Version, _, _ = unstructured.NestedString(entry, "version")
		e.State, _, _ = unstructured.NestedString(entry, "state")
		e.StartedAt, _, _ = unstructured.NestedString(entry, "startedTime")
		e.CompletedAt, _, _ = unstructured.NestedString(entry, "completionTime")
		entries = append(entries, e)
	}

	return entries, nil
}

func (m *Manager) detectUpgradeMethod(ctx context.Context, clusterName string, clusterType client.ClusterType) UpgradeMethod {
	if clusterType == client.ClusterTypeKubernetes {
		return UpgradeMethodReportOnly
	}

	_, err := m.client.Get(ctx, client.GVRClusterDeployment, clusterName, clusterName)
	if err == nil {
		return UpgradeMethodHive
	}

	if clusterType == client.ClusterTypeOCP {
		return UpgradeMethodManifest
	}

	return UpgradeMethodReportOnly
}

// applyManifestWork creates or updates a ManifestWork using the provided builder function.
func (m *Manager) applyManifestWork(ctx context.Context, clusterName, mwName string, build func(string, string) *unstructured.Unstructured) error {
	mw := build(clusterName, mwName)

	_, err := m.client.Get(ctx, client.GVRManifestWork, clusterName, mwName)
	if err == nil {
		_, err = m.client.Update(ctx, client.GVRManifestWork, clusterName, mw)
		if err != nil {
			return fmt.Errorf("updating ManifestWork for %s on %s: %w", mwName, clusterName, err)
		}
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("checking ManifestWork for %s: %w", clusterName, err)
	}

	_, err = m.client.Create(ctx, client.GVRManifestWork, clusterName, mw)
	if err != nil {
		return fmt.Errorf("creating ManifestWork for %s on %s: %w", mwName, clusterName, err)
	}
	return nil
}

func fillOCPStatus(info *unstructured.Unstructured, status *UpgradeStatus) {
	version, _, _ := unstructured.NestedString(info.Object, "status", "distributionInfo", "ocp", "version")
	status.CurrentVersion = version

	channel, _, _ := unstructured.NestedString(info.Object, "status", "distributionInfo", "ocp", "channel")
	status.Channel = channel

	desiredVersion, _, _ := unstructured.NestedString(info.Object, "status", "distributionInfo", "ocp", "desiredVersion")
	status.DesiredVersion = desiredVersion

	upgradeFailed, _, _ := unstructured.NestedBool(info.Object, "status", "distributionInfo", "ocp", "upgradeFailed")
	status.UpgradeFailed = upgradeFailed

	if upgradeFailed {
		conditions, _, _ := unstructured.NestedSlice(info.Object, "status", "distributionInfo", "ocp", "lastAppliedManifestWorkStatus")
		if len(conditions) > 0 {
			if cond, ok := conditions[0].(map[string]interface{}); ok {
				msg, _, _ := unstructured.NestedString(cond, "message")
				status.FailureMessage = msg
			}
		}
	}

	if version != "" && desiredVersion != "" && version != desiredVersion {
		status.Progressing = true
	}

	available, found, _ := unstructured.NestedSlice(info.Object, "status", "distributionInfo", "ocp", "availableUpdates")
	if found {
		for _, v := range available {
			if s, ok := v.(string); ok {
				status.Available = append(status.Available, s)
			}
		}
	}
}

func fillK8sStatus(info *unstructured.Unstructured, status *UpgradeStatus) {
	version, _, _ := unstructured.NestedString(info.Object, "status", "distributionInfo", "k8s", "gitVersion")
	status.CurrentVersion = version
}

func (m *Manager) fillHiveDesiredVersion(ctx context.Context, clusterName string, status *UpgradeStatus) {
	cd, err := m.client.Get(ctx, client.GVRClusterDeployment, clusterName, clusterName)
	if err != nil {
		return
	}

	imageSetRef, _, _ := unstructured.NestedString(cd.Object, "spec", "provisioning", "imageSetRef", "name")
	if imageSetRef == "" {
		return
	}

	imageSet, err := m.client.Get(ctx, client.GVRClusterImageSet, "", imageSetRef)
	if err != nil {
		return
	}

	releaseImage, _, _ := unstructured.NestedString(imageSet.Object, "spec", "releaseImage")
	if releaseImage != "" && status.DesiredVersion == "" {
		status.DesiredVersion = releaseImage
	}
}
