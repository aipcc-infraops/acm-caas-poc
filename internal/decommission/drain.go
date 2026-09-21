package decommission

import (
	"context"
	"fmt"
	"time"
)

const defaultDrainTimeout = 5 * time.Minute

func (m *Manager) Drain(ctx context.Context, clusterName string, timeout time.Duration) error {
	m.logger.Info("decommission.Drain", "cluster", clusterName)
	if timeout == 0 {
		timeout = defaultDrainTimeout
	}
	return fmt.Errorf("drain not implemented: real node drain required before cluster deletion")
}

func (m *Manager) Notify(ctx context.Context, clusterName, owner, deadline string) error {
	m.logger.Info("decommission.Notify", "cluster", clusterName, "owner", owner)
	state, err := getState(ctx, m.client, clusterName)
	if err != nil {
		return err
	}
	if state.NotifiedAt != "" {
		return nil
	}
	return fmt.Errorf("notify not implemented: owner notification and acknowledgement required before proceeding")
}
