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

type PowerState string

const (
	PowerStateRunning     PowerState = "Running"
	PowerStateHibernating PowerState = "Hibernating"
	PowerStateStopping    PowerState = "Stopping"
	PowerStateResuming    PowerState = "Resuming"
	PowerStateUnknown     PowerState = "Unknown"
)

type Severity string

const (
	SeverityOK      Severity = "ok"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

type DiagnosticCheck struct {
	Name     string   `json:"name"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Detail   string   `json:"detail,omitempty"`
}

type DiagnosticReport struct {
	Cluster         string            `json:"cluster"`
	HivePowerSpec   PowerState        `json:"hivePowerSpec"`
	HivePowerStatus PowerState        `json:"hivePowerStatus"`
	ACMAvailable    string            `json:"acmAvailable"`
	ACMJoined       string            `json:"acmJoined"`
	Platform        string            `json:"platform,omitempty"`
	Checks          []DiagnosticCheck `json:"checks"`
	Suggestions     []string          `json:"suggestions,omitempty"`
}

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

func (m *Manager) Diagnose(ctx context.Context, namespace, name string) (*DiagnosticReport, error) {
	report := &DiagnosticReport{
		Cluster: fmt.Sprintf("%s/%s", namespace, name),
	}

	cd, err := m.client.Get(ctx, client.GVRClusterDeployment, namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("no ClusterDeployment found for cluster %s/%s: this cluster may be imported (not Hive-provisioned)", namespace, name)
		}
		return nil, fmt.Errorf("getting ClusterDeployment: %w", err)
	}

	specPower, _, _ := unstructured.NestedString(cd.Object, "spec", "powerState")
	statusPower, _, _ := unstructured.NestedString(cd.Object, "status", "powerState")
	report.HivePowerSpec = PowerState(specPower)
	report.HivePowerStatus = PowerState(statusPower)
	if report.HivePowerSpec == "" {
		report.HivePowerSpec = PowerStateUnknown
	}
	if report.HivePowerStatus == "" {
		report.HivePowerStatus = PowerStateUnknown
	}

	platform, _, _ := unstructured.NestedString(cd.Object, "metadata", "labels", "hive.openshift.io/cluster-platform")
	report.Platform = platform

	report.Checks = append(report.Checks, checkHiveTransition(report.HivePowerSpec, report.HivePowerStatus))
	report.Checks = append(report.Checks, checkHiveConditions(cd, report.HivePowerStatus)...)

	mc, err := m.client.Get(ctx, client.GVRManagedCluster, "", name)
	if err != nil {
		if errors.IsNotFound(err) {
			report.ACMAvailable = "not found"
			report.ACMJoined = "not found"
			report.Checks = append(report.Checks, DiagnosticCheck{
				Name:     "managedcluster-exists",
				Severity: SeverityWarning,
				Message:  "No ManagedCluster resource found",
				Detail:   "ClusterDeployment exists but ManagedCluster does not — cluster may not be registered with ACM",
			})
		} else {
			return nil, fmt.Errorf("getting ManagedCluster: %w", err)
		}
	} else {
		available, joined := extractMCConditions(mc)
		report.ACMAvailable = available
		report.ACMJoined = joined
		report.Checks = append(report.Checks, checkACMHealth(report.HivePowerSpec, available, joined)...)
	}

	report.Suggestions = deriveSuggestions(report.Checks)
	return report, nil
}

func checkHiveTransition(spec, status PowerState) DiagnosticCheck {
	if spec == status {
		return DiagnosticCheck{
			Name:     "hive-power-sync",
			Severity: SeverityOK,
			Message:  fmt.Sprintf("Power state consistent: %s", spec),
		}
	}
	return DiagnosticCheck{
		Name:     "hive-power-sync",
		Severity: SeverityWarning,
		Message:  fmt.Sprintf("Power state transitioning: spec=%s, status=%s", spec, status),
		Detail:   "Hive has not yet reached the desired power state",
	}
}

func checkHiveConditions(cd *unstructured.Unstructured, currentPower PowerState) []DiagnosticCheck {
	var checks []DiagnosticCheck
	conditions, found, _ := unstructured.NestedSlice(cd.Object, "status", "conditions")
	if !found {
		return checks
	}

	problemConditions := map[string]bool{
		"Unreachable":           true,
		"ProvisionFailed":       true,
		"SyncSetFailed":         true,
		"InstallLaunchError":    true,
		"AuthenticationFailure": true,
	}

	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _, _ := unstructured.NestedString(cond, "type")
		condStatus, _, _ := unstructured.NestedString(cond, "status")

		if !problemConditions[condType] {
			continue
		}
		if condStatus != "True" {
			continue
		}

		// Unreachable is expected when the cluster is hibernating or stopping
		if condType == "Unreachable" && (currentPower == PowerStateHibernating || currentPower == PowerStateStopping) {
			msg, _, _ := unstructured.NestedString(cond, "message")
			checks = append(checks, DiagnosticCheck{
				Name:     fmt.Sprintf("hive-%s", condType),
				Severity: SeverityOK,
				Message:  fmt.Sprintf("Unreachable (expected — cluster is %s)", currentPower),
				Detail:   msg,
			})
			continue
		}

		msg, _, _ := unstructured.NestedString(cond, "message")
		checks = append(checks, DiagnosticCheck{
			Name:     fmt.Sprintf("hive-%s", condType),
			Severity: SeverityError,
			Message:  fmt.Sprintf("Hive condition %s is True", condType),
			Detail:   msg,
		})
	}
	return checks
}

func extractMCConditions(mc *unstructured.Unstructured) (available, joined string) {
	conditions, found, _ := unstructured.NestedSlice(mc.Object, "status", "conditions")
	if !found {
		return "Unknown", "Unknown"
	}
	available = "Unknown"
	joined = "Unknown"
	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _, _ := unstructured.NestedString(cond, "type")
		condStatus, _, _ := unstructured.NestedString(cond, "status")
		switch condType {
		case "ManagedClusterConditionAvailable":
			available = condStatus
		case "ManagedClusterJoined":
			joined = condStatus
		}
	}
	return available, joined
}

func checkACMHealth(hivePower PowerState, available, joined string) []DiagnosticCheck {
	var checks []DiagnosticCheck
	isHibernated := hivePower == PowerStateHibernating || hivePower == PowerStateStopping

	if hivePower == PowerStateRunning && available != "True" {
		sev := SeverityWarning
		if available == "Unknown" {
			sev = SeverityError
		}
		detail := "Hive says Running but ACM agent is not reporting as available"
		if available == "Unknown" {
			detail = "Registration agent stopped updating its lease — klusterlet may need restart"
		}
		checks = append(checks, DiagnosticCheck{
			Name:     "acm-available-mismatch",
			Severity: sev,
			Message:  fmt.Sprintf("Hive power=Running but ACM Available=%s", available),
			Detail:   detail,
		})
	}

	if isHibernated && available == "True" {
		checks = append(checks, DiagnosticCheck{
			Name:     "acm-available-during-hibernate",
			Severity: SeverityWarning,
			Message:  "ACM reports Available=True while cluster is hibernating",
			Detail:   "Cluster is hibernating but ACM agent lease has not expired yet — will resolve when lease times out",
		})
	}

	if isHibernated && (available == "Unknown" || available == "False") {
		checks = append(checks, DiagnosticCheck{
			Name:     "acm-health",
			Severity: SeverityOK,
			Message:  fmt.Sprintf("ACM Available=%s (expected — cluster is %s)", available, hivePower),
		})
	}

	if joined != "True" && joined != "Unknown" {
		checks = append(checks, DiagnosticCheck{
			Name:     "acm-not-joined",
			Severity: SeverityError,
			Message:  fmt.Sprintf("ManagedCluster Joined=%s", joined),
			Detail:   "Cluster has not joined the hub — check klusterlet installation on the spoke",
		})
	}

	if len(checks) == 0 {
		checks = append(checks, DiagnosticCheck{
			Name:     "acm-health",
			Severity: SeverityOK,
			Message:  fmt.Sprintf("ACM health consistent: Available=%s, Joined=%s", available, joined),
		})
	}

	return checks
}

func deriveSuggestions(checks []DiagnosticCheck) []string {
	var suggestions []string
	for _, c := range checks {
		switch c.Name {
		case "acm-available-mismatch":
			if c.Severity == SeverityError {
				suggestions = append(suggestions,
					"Restart klusterlet agent pods: kubectl delete pods -n open-cluster-management-agent -l app=klusterlet-agent --context <spoke>",
					"Check klusterlet logs: kubectl logs -n open-cluster-management-agent -l app=klusterlet-agent --tail=50 --context <spoke>",
				)
			}
		case "hive-Unreachable":
			suggestions = append(suggestions,
				"Check cloud credentials and network connectivity to the cluster's API server",
			)
		case "hive-ProvisionFailed":
			suggestions = append(suggestions,
				"Review ClusterDeployment conditions: kubectl get clusterdeployment -n <namespace> <name> -o yaml",
				"Check Hive install logs: kubectl logs -n <namespace> -l hive.openshift.io/cluster-deployment-name=<name>",
			)
		case "acm-not-joined":
			suggestions = append(suggestions,
				"Re-import the cluster from ACM console or recreate the klusterlet",
			)
		case "managedcluster-exists":
			suggestions = append(suggestions,
				"Import the cluster into ACM: create a ManagedCluster resource matching the ClusterDeployment name",
			)
		}
	}
	return suggestions
}
