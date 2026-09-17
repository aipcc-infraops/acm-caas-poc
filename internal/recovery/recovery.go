package recovery

import (
	"context"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

const DefaultNamespace = "open-cluster-management-policies"

type DROpts struct {
	SourceCluster string
	TargetCluster string
	PairName      string
	Schedule      string
	TTL           string
	RepoURL       string
	Path          string
}

type FailoverStatus struct {
	SourceCluster string `json:"sourceCluster"`
	TargetCluster string `json:"targetCluster"`
	PairName      string `json:"pairName"`
	RestorePhase  string `json:"restorePhase"`
	GitOpsStatus  string `json:"gitOpsStatus"`
	Phase         string `json:"phase"`
}

type DRPair struct {
	Name          string `json:"name"`
	SourceCluster string `json:"sourceCluster"`
	TargetCluster string `json:"targetCluster"`
	SourceRole    string `json:"sourceRole"`
	TargetRole    string `json:"targetRole"`
	Status        string `json:"status"`
}

type Manager struct {
	client *client.Client
	cfg    config.Config
	logger *slog.Logger
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger}
}

func (m *Manager) EnableDR(ctx context.Context, opts DROpts) error {
	m.logger.Info("recovery.EnableDR", "source", opts.SourceCluster, "target", opts.TargetCluster)
	applyDRDefaults(&opts)

	sourceLabels := buildDRLabels(opts.SourceCluster, opts.PairName, "primary")
	if err := m.client.CreateIfNotExists(ctx, client.GVRManifestWork, opts.SourceCluster, sourceLabels); err != nil {
		return fmt.Errorf("labelling source cluster: %w", err)
	}

	targetLabels := buildDRLabels(opts.TargetCluster, opts.PairName, "standby")
	if err := m.client.CreateIfNotExists(ctx, client.GVRManifestWork, opts.TargetCluster, targetLabels); err != nil {
		return fmt.Errorf("labelling target cluster: %w", err)
	}

	veleroWork := buildVeleroManifestWork(opts)
	if err := m.client.CreateIfNotExists(ctx, client.GVRManifestWork, opts.SourceCluster, veleroWork); err != nil {
		return fmt.Errorf("creating Velero ManifestWork: %w", err)
	}

	policy := buildVeleroPolicy(opts)
	if err := m.client.CreateIfNotExists(ctx, client.GVRConfigurationPolicy, DefaultNamespace, policy); err != nil {
		return fmt.Errorf("creating Velero policy: %w", err)
	}

	return nil
}

func (m *Manager) TriggerFailover(ctx context.Context, sourceCluster, targetCluster string) error {
	m.logger.Info("recovery.TriggerFailover", "source", sourceCluster, "target", targetCluster)

	pairName := drPairName(sourceCluster, targetCluster)

	restoreWork := buildRestoreManifestWork(sourceCluster, targetCluster, pairName)
	if err := m.client.CreateIfNotExists(ctx, client.GVRManifestWork, targetCluster, restoreWork); err != nil {
		return fmt.Errorf("creating restore ManifestWork: %w", err)
	}

	appSet := buildFailoverApplicationSet(sourceCluster, targetCluster, pairName)
	if err := m.client.CreateIfNotExists(ctx, client.GVRApplicationSet, "openshift-gitops", appSet); err != nil {
		return fmt.Errorf("creating failover ApplicationSet: %w", err)
	}

	return nil
}

func (m *Manager) FailoverStatus(ctx context.Context, sourceCluster, targetCluster string) (*FailoverStatus, error) {
	m.logger.Info("recovery.FailoverStatus", "source", sourceCluster, "target", targetCluster)

	pairName := drPairName(sourceCluster, targetCluster)
	status := &FailoverStatus{
		SourceCluster: sourceCluster,
		TargetCluster: targetCluster,
		PairName:      pairName,
		Phase:         "Unknown",
	}

	restoreName := "dr-restore-" + pairName
	restoreObj, err := m.client.Get(ctx, client.GVRManifestWork, targetCluster, restoreName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			status.RestorePhase = "NotStarted"
		} else {
			return nil, fmt.Errorf("getting restore ManifestWork: %w", err)
		}
	} else {
		status.RestorePhase = parseManifestWorkPhase(restoreObj.Object)
	}

	appSetName := "dr-failover-" + pairName
	appSetObj, err := m.client.Get(ctx, client.GVRApplicationSet, "openshift-gitops", appSetName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			status.GitOpsStatus = "NotDeployed"
		} else {
			return nil, fmt.Errorf("getting failover ApplicationSet: %w", err)
		}
	} else {
		status.GitOpsStatus = parseAppSetPhase(appSetObj.Object)
	}

	status.Phase = deriveFailoverPhase(status.RestorePhase, status.GitOpsStatus)
	return status, nil
}

func (m *Manager) DisableDR(ctx context.Context, cluster string) error {
	m.logger.Info("recovery.DisableDR", "cluster", cluster)

	pairs, err := m.ListDRPairs(ctx)
	if err != nil {
		return fmt.Errorf("listing DR pairs: %w", err)
	}

	for _, pair := range pairs {
		if pair.SourceCluster != cluster && pair.TargetCluster != cluster {
			continue
		}
		_ = m.client.DeleteIfExists(ctx, client.GVRManifestWork, pair.SourceCluster, "dr-velero-"+pair.Name)
		_ = m.client.DeleteIfExists(ctx, client.GVRManifestWork, pair.SourceCluster, "dr-labels-"+pair.Name)
		_ = m.client.DeleteIfExists(ctx, client.GVRManifestWork, pair.TargetCluster, "dr-restore-"+pair.Name)
		_ = m.client.DeleteIfExists(ctx, client.GVRManifestWork, pair.TargetCluster, "dr-labels-"+pair.Name)
		_ = m.client.DeleteIfExists(ctx, client.GVRConfigurationPolicy, DefaultNamespace, "dr-velero-policy-"+pair.Name)
		_ = m.client.DeleteIfExists(ctx, client.GVRApplicationSet, "openshift-gitops", "dr-failover-"+pair.Name)
	}

	return nil
}

func (m *Manager) ListDRPairs(ctx context.Context) ([]DRPair, error) {
	m.logger.Info("recovery.ListDRPairs")

	list, err := m.client.List(ctx, client.GVRManifestWork, "", "acmlab.redhat.com/dr-role")
	if err != nil {
		return nil, fmt.Errorf("listing DR ManifestWorks: %w", err)
	}

	pairMap := make(map[string]*DRPair)
	for _, item := range list.Items {
		pair := parseDRManifestWork(item.Object)
		if pair.Name == "" {
			continue
		}
		if existing, ok := pairMap[pair.Name]; ok {
			mergeDRPair(existing, &pair)
		} else {
			pairMap[pair.Name] = &pair
		}
	}

	pairs := make([]DRPair, 0, len(pairMap))
	for _, p := range pairMap {
		p.Status = deriveDRStatus(p)
		pairs = append(pairs, *p)
	}
	return pairs, nil
}

func deriveFailoverPhase(restorePhase, gitOpsStatus string) string {
	if restorePhase == "NotStarted" && gitOpsStatus == "NotDeployed" {
		return "NotStarted"
	}
	if restorePhase == "Applied" && gitOpsStatus == "Synced" {
		return "Completed"
	}
	return "InProgress"
}

func deriveDRStatus(pair *DRPair) string {
	if pair.SourceCluster != "" && pair.TargetCluster != "" {
		return "Configured"
	}
	return "Partial"
}
