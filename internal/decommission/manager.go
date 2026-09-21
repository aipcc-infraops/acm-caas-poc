package decommission

import (
	"context"
	"fmt"
	"log/slog"

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

func (m *Manager) Start(ctx context.Context, clusterName string, opts StartOpts) (*DecommissionState, error) {
	m.logger.Info("decommission.Start", "cluster", clusterName)
	existing, err := getState(ctx, m.client, clusterName)
	if err == nil {
		return existing, nil
	}

	state, err := createState(ctx, m.client, clusterName, opts)
	if err != nil {
		return nil, err
	}

	report, err := m.Audit(ctx, clusterName)
	if err != nil {
		return state, fmt.Errorf("audit failed: %w", err)
	}
	state.Audit = report
	state.Phase = PhaseAudited
	if report.Owner != "" && state.Owner == "" {
		state.Owner = report.Owner
	}
	state.addHistory(PhaseAudited, fmt.Sprintf("%d nodes, %s CPU, %s memory",
		report.NodeCount, report.CPUCapacity, report.MemoryCapacity))

	if err := setState(ctx, m.client, state); err != nil {
		return nil, fmt.Errorf("saving audit state: %w", err)
	}
	return state, nil
}

func (m *Manager) Advance(ctx context.Context, clusterName string) (*DecommissionState, error) {
	m.logger.Info("decommission.Advance", "cluster", clusterName)
	state, err := getState(ctx, m.client, clusterName)
	if err != nil {
		return nil, err
	}

	target := nextPhase(state.Phase)
	if target == state.Phase {
		return state, nil
	}

	var msg string
	switch target {
	case PhaseNotified:
		if err := m.Notify(ctx, clusterName, state.Owner, state.Deadline); err != nil {
			return state, err
		}
		msg = fmt.Sprintf("Owner %s notified, deadline %s", state.Owner, state.Deadline)
	case PhaseBackedUp:
		if err := m.Backup(ctx, clusterName, state.BackupPath); err != nil {
			return state, err
		}
		msg = "Cluster state backed up"
	case PhaseDrained:
		if err := m.Drain(ctx, clusterName, 0); err != nil {
			return state, err
		}
		msg = "Worker nodes drained"
	case PhaseDeleted:
		if err := validateSafeguards(state); err != nil {
			return state, err
		}
		hive, err := m.Delete(ctx, clusterName)
		if err != nil {
			return state, err
		}
		if hive {
			msg = "Cluster infrastructure deleted"
		} else {
			msg = "Cluster detached from ACM (external infrastructure not managed)"
		}
	case PhaseCleaned:
		if err := m.Cleanup(ctx, clusterName); err != nil {
			return state, err
		}
		state.Phase = PhaseCleaned
		state.addHistory(PhaseCleaned, "ACM resources cleaned up")
		return state, nil
	default:
		msg = fmt.Sprintf("Advanced to %s", target)
	}

	state, err = getState(ctx, m.client, clusterName)
	if err != nil {
		return nil, err
	}
	state.Phase = target
	state.addHistory(target, msg)
	if err := setState(ctx, m.client, state); err != nil {
		return nil, err
	}
	return state, nil
}

// validateSafeguards blocks destructive advancement unless prior safeguard
// phases left verifiable evidence. A workflow persisted by a previous version
// (where Notify/Backup/Drain were no-ops) will have phase=drained but no
// real evidence; this prevents that state from reaching deletion.
func validateSafeguards(state *DecommissionState) error {
	if state.NotifiedAt == "" {
		return fmt.Errorf("cannot delete: owner notification was never completed (phase may have been set by a previous version)")
	}
	if state.BackupPath == "" {
		return fmt.Errorf("cannot delete: cluster backup was never completed (phase may have been set by a previous version)")
	}
	return nil
}

func (m *Manager) GetState(ctx context.Context, clusterName string) (*DecommissionState, error) {
	m.logger.Info("decommission.GetState", "cluster", clusterName)
	return getState(ctx, m.client, clusterName)
}

func (m *Manager) List(ctx context.Context) ([]DecommissionState, error) {
	m.logger.Info("decommission.List")
	return listStates(ctx, m.client)
}

func (m *Manager) Cancel(ctx context.Context, clusterName string) error {
	m.logger.Info("decommission.Cancel", "cluster", clusterName)
	return deleteState(ctx, m.client, clusterName)
}
