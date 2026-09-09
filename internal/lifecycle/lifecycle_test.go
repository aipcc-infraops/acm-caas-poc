package lifecycle

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

func TestGetPowerStateReturnsCurrentState(t *testing.T) {
	tests := []struct {
		name          string
		namespace     string
		clusterName   string
		powerState    string
		expectedState PowerState
	}{
		{
			name:          "running cluster",
			namespace:     "test-ns",
			clusterName:   "cluster1",
			powerState:    "Running",
			expectedState: PowerStateRunning,
		},
		{
			name:          "hibernating cluster",
			namespace:     "test-ns",
			clusterName:   "cluster2",
			powerState:    "Hibernating",
			expectedState: PowerStateHibernating,
		},
		{
			name:          "stopping cluster",
			namespace:     "test-ns",
			clusterName:   "cluster3",
			powerState:    "Stopping",
			expectedState: PowerStateStopping,
		},
		{
			name:          "resuming cluster",
			namespace:     "test-ns",
			clusterName:   "cluster4",
			powerState:    "Resuming",
			expectedState: PowerStateResuming,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cd := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "hive.openshift.io/v1",
					"kind":       "ClusterDeployment",
					"metadata": map[string]interface{}{
						"name":      tt.clusterName,
						"namespace": tt.namespace,
					},
					"spec": map[string]interface{}{
						"powerState": tt.powerState,
					},
				},
			}

			scheme := runtime.NewScheme()
			fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd)
			c := &client.Client{Dynamic: fakeDynamic}
			m := New(c, config.Config{})

			got, err := m.GetPowerState(context.Background(), tt.namespace, tt.clusterName)
			if err != nil {
				t.Fatalf("GetPowerState() error = %v", err)
			}
			if got != tt.expectedState {
				t.Errorf("GetPowerState() = %v, want %v", got, tt.expectedState)
			}
		})
	}
}

func TestGetPowerStateReturnsErrorForMissingCluster(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{})

	_, err := m.GetPowerState(context.Background(), "test-ns", "missing-cluster")
	if err == nil {
		t.Fatal("expected error for missing cluster, got nil")
	}
	if !contains(err.Error(), "no ClusterDeployment found") {
		t.Errorf("expected error message to contain 'no ClusterDeployment found', got: %v", err)
	}
}

func TestGetPowerStateReturnsUnknownWhenFieldMissing(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "cluster1",
				"namespace": "test-ns",
			},
			"spec": map[string]interface{}{
				// no powerState field
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{})

	got, err := m.GetPowerState(context.Background(), "test-ns", "cluster1")
	if err != nil {
		t.Fatalf("GetPowerState() error = %v", err)
	}
	if got != PowerStateUnknown {
		t.Errorf("GetPowerState() = %v, want %v", got, PowerStateUnknown)
	}
}

func TestHibernateIsIdempotent(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "cluster1",
				"namespace": "test-ns",
			},
			"spec": map[string]interface{}{
				"powerState": "Hibernating",
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{})

	// Hibernate an already hibernating cluster should succeed without error
	err := m.Hibernate(context.Background(), "test-ns", "cluster1")
	if err != nil {
		t.Fatalf("Hibernate() on already hibernating cluster error = %v", err)
	}

	// Verify state is still Hibernating
	state, err := m.GetPowerState(context.Background(), "test-ns", "cluster1")
	if err != nil {
		t.Fatalf("GetPowerState() error = %v", err)
	}
	if state != PowerStateHibernating {
		t.Errorf("GetPowerState() = %v, want %v", state, PowerStateHibernating)
	}
}

func TestResumeIsIdempotent(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "cluster1",
				"namespace": "test-ns",
			},
			"spec": map[string]interface{}{
				"powerState": "Running",
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{})

	// Resume an already running cluster should succeed without error
	err := m.Resume(context.Background(), "test-ns", "cluster1")
	if err != nil {
		t.Fatalf("Resume() on already running cluster error = %v", err)
	}

	// Verify state is still Running
	state, err := m.GetPowerState(context.Background(), "test-ns", "cluster1")
	if err != nil {
		t.Fatalf("GetPowerState() error = %v", err)
	}
	if state != PowerStateRunning {
		t.Errorf("GetPowerState() = %v, want %v", state, PowerStateRunning)
	}
}

func TestClusterSupportsLifecycleReturnsTrueWhenClusterDeploymentExists(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "cluster1",
				"namespace": "test-ns",
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{})

	supported, err := m.ClusterSupportsLifecycle(context.Background(), "test-ns", "cluster1")
	if err != nil {
		t.Fatalf("ClusterSupportsLifecycle() error = %v", err)
	}
	if !supported {
		t.Error("ClusterSupportsLifecycle() = false, want true")
	}
}

func TestClusterSupportsLifecycleReturnsFalseWhenClusterDeploymentMissing(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{})

	supported, err := m.ClusterSupportsLifecycle(context.Background(), "test-ns", "missing-cluster")
	if err != nil {
		t.Fatalf("ClusterSupportsLifecycle() error = %v", err)
	}
	if supported {
		t.Error("ClusterSupportsLifecycle() = true, want false for imported cluster")
	}
}

func TestWaitForPowerStateTimesOutWhenStateNotReached(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "cluster1",
				"namespace": "test-ns",
			},
			"spec": map[string]interface{}{
				"powerState": "Running",
			},
			"status": map[string]interface{}{
				"powerState": "Running",
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{})

	// Wait for Hibernating state with very short timeout (state will never be reached)
	ctx := context.Background()
	err := m.WaitForPowerState(ctx, "test-ns", "cluster1", PowerStateHibernating, 1*time.Second)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestListClustersWithLifecycleReturnsAllClusterDeployments(t *testing.T) {
	cd1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "cluster1",
				"namespace": "ns1",
			},
		},
	}
	cd2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "cluster2",
				"namespace": "ns2",
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd1, cd2)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{})

	names, err := m.ListClustersWithLifecycle(context.Background())
	if err != nil {
		t.Fatalf("ListClustersWithLifecycle() error = %v", err)
	}
	if len(names) != 2 {
		t.Errorf("ListClustersWithLifecycle() returned %d clusters, want 2", len(names))
	}
	// Check that both clusters are present (order doesn't matter)
	found := make(map[string]bool)
	for _, name := range names {
		found[name] = true
	}
	if !found["ns1/cluster1"] || !found["ns2/cluster2"] {
		t.Errorf("ListClustersWithLifecycle() = %v, want [ns1/cluster1, ns2/cluster2]", names)
	}
}

func TestCheckHiveTransitionReportsSync(t *testing.T) {
	check := checkHiveTransition(PowerStateRunning, PowerStateRunning)
	if check.Severity != SeverityOK {
		t.Errorf("expected OK severity, got %s", check.Severity)
	}
}

func TestCheckHiveTransitionReportsTransitioning(t *testing.T) {
	check := checkHiveTransition(PowerStateHibernating, PowerStateRunning)
	if check.Severity != SeverityWarning {
		t.Errorf("expected Warning severity, got %s", check.Severity)
	}
}

func TestCheckACMHealthDetectsKlusterletIssue(t *testing.T) {
	checks := checkACMHealth(PowerStateRunning, "Unknown", "True")
	if len(checks) == 0 {
		t.Fatal("expected at least one check, got none")
	}
	if checks[0].Severity != SeverityError {
		t.Errorf("expected Error severity for Running+Available=Unknown, got %s", checks[0].Severity)
	}
	if checks[0].Name != "acm-available-mismatch" {
		t.Errorf("expected acm-available-mismatch, got %s", checks[0].Name)
	}
}

func TestCheckACMHealthOKWhenConsistent(t *testing.T) {
	checks := checkACMHealth(PowerStateRunning, "True", "True")
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
	if checks[0].Severity != SeverityOK {
		t.Errorf("expected OK severity, got %s", checks[0].Severity)
	}
}

func TestCheckACMHealthHibernatingWithAvailableTrue(t *testing.T) {
	checks := checkACMHealth(PowerStateHibernating, "True", "True")
	if len(checks) == 0 {
		t.Fatal("expected at least one check, got none")
	}
	if checks[0].Name != "acm-available-during-hibernate" {
		t.Errorf("expected acm-available-during-hibernate, got %s", checks[0].Name)
	}
}

func TestDeriveSuggestionsForKlusterletIssue(t *testing.T) {
	checks := []DiagnosticCheck{
		{Name: "acm-available-mismatch", Severity: SeverityError, Message: "test"},
	}
	suggestions := deriveSuggestions(checks)
	if len(suggestions) < 2 {
		t.Fatalf("expected at least 2 suggestions for klusterlet issue, got %d", len(suggestions))
	}
}

func TestDeriveSuggestionsEmptyForHealthy(t *testing.T) {
	checks := []DiagnosticCheck{
		{Name: "hive-power-sync", Severity: SeverityOK, Message: "ok"},
		{Name: "acm-health", Severity: SeverityOK, Message: "ok"},
	}
	suggestions := deriveSuggestions(checks)
	if len(suggestions) != 0 {
		t.Errorf("expected 0 suggestions for healthy cluster, got %d", len(suggestions))
	}
}

func TestDiagnoseIntegratesHiveAndACM(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "cluster1",
				"namespace": "test-ns",
				"labels": map[string]interface{}{
					"hive.openshift.io/cluster-platform": "ibmcloud",
				},
			},
			"spec": map[string]interface{}{
				"powerState": "Running",
			},
			"status": map[string]interface{}{
				"powerState": "Running",
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Unreachable",
						"status": "False",
					},
				},
			},
		},
	}
	mc := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "cluster1",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "ManagedClusterConditionAvailable",
						"status": "Unknown",
					},
					map[string]interface{}{
						"type":   "ManagedClusterJoined",
						"status": "True",
					},
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd, mc)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{})

	report, err := m.Diagnose(context.Background(), "test-ns", "cluster1")
	if err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}

	if report.Platform != "ibmcloud" {
		t.Errorf("expected platform ibmcloud, got %s", report.Platform)
	}
	if report.ACMAvailable != "Unknown" {
		t.Errorf("expected ACM Available=Unknown, got %s", report.ACMAvailable)
	}
	if len(report.Suggestions) == 0 {
		t.Error("expected suggestions for klusterlet issue, got none")
	}

	hasError := false
	for _, check := range report.Checks {
		if check.Name == "acm-available-mismatch" && check.Severity == SeverityError {
			hasError = true
		}
	}
	if !hasError {
		t.Error("expected acm-available-mismatch error check in report")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
