package migration

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

const DefaultNamespace = "open-cluster-management-policies"

type MigrationOpts struct {
	SourceCluster string
	TargetCluster string
	Namespaces    []string
}

type MigrationPlan struct {
	PlanID        string   `json:"planID"`
	SourceCluster string   `json:"sourceCluster"`
	TargetCluster string   `json:"targetCluster"`
	Workloads     int      `json:"workloads"`
	Namespaces    []string `json:"namespaces"`
	Phase         string   `json:"phase"`
	CreatedAt     string   `json:"createdAt"`
}

type MigrationStatus struct {
	PlanID        string `json:"planID"`
	SourceCluster string `json:"sourceCluster"`
	TargetCluster string `json:"targetCluster"`
	Phase         string `json:"phase"`
	Workloads     int    `json:"workloads"`
	Migrated      int    `json:"migrated"`
	Message       string `json:"message,omitempty"`
}

type MigrationSummary struct {
	PlanID        string `json:"planID"`
	SourceCluster string `json:"sourceCluster"`
	TargetCluster string `json:"targetCluster"`
	Phase         string `json:"phase"`
	CreatedAt     string `json:"createdAt"`
}

type Manager struct {
	client *client.Client
	cfg    config.Config
	logger *slog.Logger
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger}
}

func (m *Manager) PlanMigration(ctx context.Context, opts MigrationOpts) (*MigrationPlan, error) {
	m.logger.Info("migration.PlanMigration", "source", opts.SourceCluster, "target", opts.TargetCluster)

	workloads, err := m.countWorkloads(ctx, opts.SourceCluster)
	if err != nil {
		return nil, fmt.Errorf("counting source workloads: %w", err)
	}

	planID := migrationPlanID(opts.SourceCluster, opts.TargetCluster)
	plan := &MigrationPlan{
		PlanID:        planID,
		SourceCluster: opts.SourceCluster,
		TargetCluster: opts.TargetCluster,
		Workloads:     workloads,
		Namespaces:    opts.Namespaces,
		Phase:         "Planned",
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	if len(plan.Namespaces) == 0 {
		plan.Namespaces = []string{"*"}
	}

	planCM := buildMigrationPlan(plan)
	if err := m.client.CreateIfNotExists(ctx, client.GVRConfigMap, DefaultNamespace, planCM); err != nil {
		return nil, fmt.Errorf("creating migration plan: %w", err)
	}

	return plan, nil
}

func (m *Manager) ExecuteMigration(ctx context.Context, planID string) error {
	m.logger.Info("migration.ExecuteMigration", "planID", planID)

	plan, err := m.getPlan(ctx, planID)
	if err != nil {
		return fmt.Errorf("getting migration plan: %w", err)
	}

	if err := m.updatePlanPhase(ctx, planID, "Deploying"); err != nil {
		return fmt.Errorf("updating plan phase: %w", err)
	}

	cordonWork := buildCordonLabels(plan.SourceCluster, planID)
	if err := m.client.CreateIfNotExists(ctx, client.GVRManifestWork, plan.SourceCluster, cordonWork); err != nil {
		return fmt.Errorf("cordoning source cluster: %w", err)
	}

	targetWork := buildTargetManifestWork(plan)
	if err := m.client.CreateIfNotExists(ctx, client.GVRManifestWork, plan.TargetCluster, targetWork); err != nil {
		return fmt.Errorf("deploying to target cluster: %w", err)
	}

	if err := m.updatePlanPhase(ctx, planID, "Verifying"); err != nil {
		return fmt.Errorf("updating plan phase: %w", err)
	}

	return nil
}

func (m *Manager) MigrationStatus(ctx context.Context, planID string) (*MigrationStatus, error) {
	m.logger.Info("migration.MigrationStatus", "planID", planID)

	plan, err := m.getPlan(ctx, planID)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return &MigrationStatus{PlanID: planID, Phase: "NotFound"}, nil
		}
		return nil, fmt.Errorf("getting migration plan: %w", err)
	}

	status := &MigrationStatus{
		PlanID:        planID,
		SourceCluster: plan.SourceCluster,
		TargetCluster: plan.TargetCluster,
		Phase:         plan.Phase,
		Workloads:     plan.Workloads,
	}

	targetWorkName := "migration-workloads-" + planID
	targetObj, err := m.client.Get(ctx, client.GVRManifestWork, plan.TargetCluster, targetWorkName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("getting target ManifestWork: %w", err)
		}
		status.Message = "Target workloads not yet deployed"
		return status, nil
	}

	if isManifestWorkApplied(targetObj.Object) {
		status.Migrated = plan.Workloads
		if plan.Phase == "Verifying" {
			status.Phase = "Completed"
		}
	}

	return status, nil
}

func (m *Manager) RollbackMigration(ctx context.Context, planID string) error {
	m.logger.Info("migration.RollbackMigration", "planID", planID)

	plan, err := m.getPlan(ctx, planID)
	if err != nil {
		return fmt.Errorf("getting migration plan: %w", err)
	}

	_ = m.client.DeleteIfExists(ctx, client.GVRManifestWork, plan.TargetCluster, "migration-workloads-"+planID)
	_ = m.client.DeleteIfExists(ctx, client.GVRManifestWork, plan.SourceCluster, "migration-cordon-"+planID)

	if err := m.updatePlanPhase(ctx, planID, "RolledBack"); err != nil {
		return fmt.Errorf("updating plan phase: %w", err)
	}

	return nil
}

func (m *Manager) ListMigrations(ctx context.Context) ([]MigrationSummary, error) {
	m.logger.Info("migration.ListMigrations")

	list, err := m.client.List(ctx, client.GVRConfigMap, DefaultNamespace, "acmlab.redhat.com/migration-plan")
	if err != nil {
		return nil, fmt.Errorf("listing migration plans: %w", err)
	}

	summaries := make([]MigrationSummary, 0, len(list.Items))
	for _, item := range list.Items {
		summaries = append(summaries, parseMigrationSummary(item.Object))
	}
	return summaries, nil
}

func (m *Manager) countWorkloads(ctx context.Context, cluster string) (int, error) {
	list, err := m.client.List(ctx, client.GVRManifestWork, cluster, "")
	if err != nil {
		return 0, err
	}
	return len(list.Items), nil
}

func (m *Manager) getPlan(ctx context.Context, planID string) (*MigrationPlan, error) {
	obj, err := m.client.Get(ctx, client.GVRConfigMap, DefaultNamespace, "migration-plan-"+planID)
	if err != nil {
		return nil, err
	}
	return parseMigrationPlanFromConfigMap(obj.Object), nil
}

func (m *Manager) updatePlanPhase(ctx context.Context, planID, phase string) error {
	obj, err := m.client.Get(ctx, client.GVRConfigMap, DefaultNamespace, "migration-plan-"+planID)
	if err != nil {
		return err
	}
	data, _ := obj.Object["data"].(map[string]interface{})
	if data == nil {
		data = make(map[string]interface{})
	}
	data["phase"] = phase
	obj.Object["data"] = data
	_, err = m.client.Update(ctx, client.GVRConfigMap, DefaultNamespace, obj)
	return err
}

func isManifestWorkApplied(obj map[string]interface{}) bool {
	status, _ := obj["status"].(map[string]interface{})
	if status == nil {
		return false
	}
	conditions, _ := status["conditions"].([]interface{})
	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] == "Applied" && cond["status"] == "True" {
			return true
		}
	}
	return false
}
