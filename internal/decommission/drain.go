package decommission

import (
	"context"
	"time"
)

const defaultDrainTimeout = 5 * time.Minute

func (m *Manager) Drain(ctx context.Context, clusterName string, timeout time.Duration) error {
	if timeout == 0 {
		timeout = defaultDrainTimeout
	}
	_ = timeout
	return nil
}

func (m *Manager) Notify(ctx context.Context, clusterName, owner, deadline string) error {
	state, err := getState(ctx, m.client, clusterName)
	if err != nil {
		return err
	}
	if state.NotifiedAt != "" {
		return nil
	}
	state.NotifiedAt = time.Now().UTC().Format(time.RFC3339)
	if owner != "" {
		state.Owner = owner
	}
	if deadline != "" {
		state.Deadline = deadline
	}
	return setState(ctx, m.client, state)
}
