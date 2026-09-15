package decommission

import (
	"context"
	"fmt"
)

func (m *Manager) Backup(ctx context.Context, clusterName, outputDir string) error {
	state, err := getState(ctx, m.client, clusterName)
	if err != nil {
		return err
	}

	if state.BackupPath != "" {
		return nil
	}

	if outputDir == "" {
		outputDir = fmt.Sprintf("./decommission-backups/%s", clusterName)
	}

	state.BackupPath = outputDir
	return setState(ctx, m.client, state)
}
