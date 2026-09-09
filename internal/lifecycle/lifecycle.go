package lifecycle

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

// PowerState represents the power state of a Hive-managed cluster
type PowerState string

const (
	PowerStateRunning     PowerState = "Running"
	PowerStateHibernating PowerState = "Hibernating"
	PowerStateStopping    PowerState = "Stopping"
	PowerStateResuming    PowerState = "Resuming"
	PowerStateUnknown     PowerState = "Unknown"
)

// Manager handles cluster lifecycle operations (hibernate/resume)
type Manager struct {
	client *client.Client
	cfg    config.Config
}

// New creates a new lifecycle Manager
func New(c *client.Client, cfg config.Config) *Manager {
	return &Manager{
		client: c,
		cfg:    cfg,
	}
}

// Hibernate hibernates a Hive-provisioned cluster by patching powerState to Hibernating
func (m *Manager) Hibernate(ctx context.Context, namespace, name string) error {
	currentState, err := m.GetPowerState(ctx, namespace, name)
	if err != nil {
		return fmt.Errorf("checking current power state: %w", err)
	}

	// Idempotent: if already hibernating, do nothing
	if currentState == PowerStateHibernating {
		return nil
	}

	return m.setPowerState(ctx, namespace, name, PowerStateHibernating)
}

// Resume resumes a hibernated cluster by patching powerState to Running
func (m *Manager) Resume(ctx context.Context, namespace, name string) error {
	currentState, err := m.GetPowerState(ctx, namespace, name)
	if err != nil {
		return fmt.Errorf("checking current power state: %w", err)
	}

	// Idempotent: if already running, do nothing
	if currentState == PowerStateRunning {
		return nil
	}

	return m.setPowerState(ctx, namespace, name, PowerStateRunning)
}

// GetPowerState returns the current power state of a cluster
func (m *Manager) GetPowerState(ctx context.Context, namespace, name string) (PowerState, error) {
	cd, err := m.client.Get(ctx, client.GVRClusterDeployment, namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			return "", fmt.Errorf("no ClusterDeployment found for cluster %s/%s: this cluster may be imported (not Hive-provisioned)", namespace, name)
		}
		return "", fmt.Errorf("getting ClusterDeployment: %w", err)
	}

	// Read spec.powerState
	powerState, found, err := unstructured.NestedString(cd.Object, "spec", "powerState")
	if err != nil {
		return "", fmt.Errorf("reading spec.powerState: %w", err)
	}
	if !found || powerState == "" {
		return PowerStateUnknown, nil
	}

	return PowerState(powerState), nil
}

// GetPowerStateStatus returns the power state from status.powerState (reflects actual state)
func (m *Manager) GetPowerStateStatus(ctx context.Context, namespace, name string) (PowerState, error) {
	cd, err := m.client.Get(ctx, client.GVRClusterDeployment, namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			return "", fmt.Errorf("no ClusterDeployment found for cluster %s/%s", namespace, name)
		}
		return "", fmt.Errorf("getting ClusterDeployment: %w", err)
	}

	// Read status.powerState
	powerState, found, err := unstructured.NestedString(cd.Object, "status", "powerState")
	if err != nil {
		return "", fmt.Errorf("reading status.powerState: %w", err)
	}
	if !found || powerState == "" {
		return PowerStateUnknown, nil
	}

	return PowerState(powerState), nil
}

// WaitForPowerState waits for the cluster power state to reach the target state
func (m *Manager) WaitForPowerState(ctx context.Context, namespace, name string, target PowerState, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(ctx, 10*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		state, err := m.GetPowerStateStatus(ctx, namespace, name)
		if err != nil {
			return false, err
		}
		return state == target, nil
	})
}

// setPowerState patches the ClusterDeployment spec.powerState field
func (m *Manager) setPowerState(ctx context.Context, namespace, name string, state PowerState) error {
	patch := []byte(fmt.Sprintf(`{"spec":{"powerState":"%s"}}`, state))
	_, err := m.client.Patch(ctx, client.GVRClusterDeployment, namespace, name, types.MergePatchType, patch)
	if err != nil {
		return fmt.Errorf("patching ClusterDeployment %s/%s powerState to %s: %w", namespace, name, state, err)
	}
	return nil
}

// ClusterSupportsLifecycle checks if a cluster supports lifecycle operations (has ClusterDeployment)
func (m *Manager) ClusterSupportsLifecycle(ctx context.Context, namespace, name string) (bool, error) {
	_, err := m.client.Get(ctx, client.GVRClusterDeployment, namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking ClusterDeployment existence: %w", err)
	}
	return true, nil
}

// ListClustersWithLifecycle returns all clusters that support lifecycle operations
func (m *Manager) ListClustersWithLifecycle(ctx context.Context) ([]string, error) {
	cds, err := m.client.List(ctx, client.GVRClusterDeployment, "", "")
	if err != nil {
		return nil, fmt.Errorf("listing ClusterDeployments: %w", err)
	}

	var names []string
	for _, item := range cds.Items {
		name := item.GetName()
		namespace := item.GetNamespace()
		names = append(names, fmt.Sprintf("%s/%s", namespace, name))
	}
	return names, nil
}
