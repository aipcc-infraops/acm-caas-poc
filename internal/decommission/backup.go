package decommission

import (
	"context"
	"fmt"
)

func (m *Manager) Backup(ctx context.Context, clusterName, outputDir string) error {
	m.logger.Info("decommission.Backup", "cluster", clusterName)
	state, err := getState(ctx, m.client, clusterName)
	if err != nil {
		return err
	}

	if state.BackupPath != "" {
		return nil
	}

	return fmt.Errorf("backup not implemented: real cluster state export required before proceeding")
}
