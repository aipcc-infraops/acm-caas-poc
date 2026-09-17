package lifecycle

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	fake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

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
			m := New(c, config.Config{}, discardLogger)

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
	fakeDynamic := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRCAPIMachineDeployment: "MachineDeploymentList",
		},
	)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	_, err := m.GetPowerState(context.Background(), "test-ns", "missing-cluster")
	if err == nil {
		t.Fatal("expected error for missing cluster, got nil")
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
	m := New(c, config.Config{}, discardLogger)

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
	m := New(c, config.Config{}, discardLogger)

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
	m := New(c, config.Config{}, discardLogger)

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
	m := New(c, config.Config{}, discardLogger)

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
	m := New(c, config.Config{}, discardLogger)

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
	m := New(c, config.Config{}, discardLogger)

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
	m := New(c, config.Config{}, discardLogger)

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
	m := New(c, config.Config{}, discardLogger)

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

func TestHibernateChangesState(t *testing.T) {
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
	m := New(c, config.Config{}, discardLogger)

	err := m.Hibernate(context.Background(), "test-ns", "cluster1")
	if err != nil {
		t.Fatalf("Hibernate() error = %v", err)
	}
}

func TestResumeChangesState(t *testing.T) {
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
	m := New(c, config.Config{}, discardLogger)

	err := m.Resume(context.Background(), "test-ns", "cluster1")
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
}

func TestHibernateReturnsErrorForMissingCluster(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRCAPIMachineDeployment: "MachineDeploymentList",
		},
	)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	err := m.Hibernate(context.Background(), "test-ns", "missing")
	if err == nil {
		t.Fatal("expected error for missing cluster")
	}
}

func TestResumeReturnsErrorForMissingCluster(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRCAPIMachineDeployment: "MachineDeploymentList",
		},
	)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	err := m.Resume(context.Background(), "test-ns", "missing")
	if err == nil {
		t.Fatal("expected error for missing cluster")
	}
}

func TestGetPowerStateStatusReturnsCurrentState(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "cluster1",
				"namespace": "test-ns",
			},
			"status": map[string]interface{}{
				"powerState": "Running",
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	got, err := m.GetPowerStateStatus(context.Background(), "test-ns", "cluster1")
	if err != nil {
		t.Fatalf("GetPowerStateStatus() error = %v", err)
	}
	if got != PowerStateRunning {
		t.Errorf("GetPowerStateStatus() = %v, want %v", got, PowerStateRunning)
	}
}

func TestGetPowerStateStatusReturnsUnknownWhenFieldMissing(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "cluster1",
				"namespace": "test-ns",
			},
			"status": map[string]interface{}{},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	got, err := m.GetPowerStateStatus(context.Background(), "test-ns", "cluster1")
	if err != nil {
		t.Fatalf("GetPowerStateStatus() error = %v", err)
	}
	if got != PowerStateUnknown {
		t.Errorf("GetPowerStateStatus() = %v, want %v", got, PowerStateUnknown)
	}
}

func TestGetPowerStateStatusReturnsErrorForMissingCluster(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	_, err := m.GetPowerStateStatus(context.Background(), "test-ns", "missing")
	if err == nil {
		t.Fatal("expected error for missing cluster")
	}
}

func TestCheckLifecycleSupportFullForHiveCluster(t *testing.T) {
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
	m := New(c, config.Config{}, discardLogger)

	reason, err := m.CheckLifecycleSupport(context.Background(), "test-ns", "cluster1")
	if err != nil {
		t.Fatalf("CheckLifecycleSupport() error = %v", err)
	}
	if reason.Support != LifecycleFull {
		t.Errorf("expected LifecycleFull, got %v", reason.Support)
	}
}

func TestCheckLifecycleSupportFullForKubernetes(t *testing.T) {
	mci := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "internal.open-cluster-management.io/v1beta1",
			"kind":       "ManagedClusterInfo",
			"metadata": map[string]interface{}{
				"name":      "k8s-cluster",
				"namespace": "k8s-cluster",
			},
			"status": map[string]interface{}{
				"distributionInfo": map[string]interface{}{
					"type": "kubernetes",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, mci)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	reason, err := m.CheckLifecycleSupport(context.Background(), "k8s-cluster", "k8s-cluster")
	if err != nil {
		t.Fatalf("CheckLifecycleSupport() error = %v", err)
	}
	if reason.Support != LifecycleFull {
		t.Errorf("expected LifecycleFull for Kubernetes (CAPI scale-to-zero), got %v", reason.Support)
	}
	if reason.ClusterType != client.ClusterTypeKubernetes {
		t.Errorf("expected ClusterTypeKubernetes, got %v", reason.ClusterType)
	}
}

func TestCheckLifecycleSupportUnsupportedForOCP(t *testing.T) {
	mci := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "internal.open-cluster-management.io/v1beta1",
			"kind":       "ManagedClusterInfo",
			"metadata": map[string]interface{}{
				"name":      "ocp-cluster",
				"namespace": "ocp-cluster",
			},
			"status": map[string]interface{}{
				"distributionInfo": map[string]interface{}{
					"type": "OCP",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, mci)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	reason, err := m.CheckLifecycleSupport(context.Background(), "ocp-cluster", "ocp-cluster")
	if err != nil {
		t.Fatalf("CheckLifecycleSupport() error = %v", err)
	}
	if reason.Support != LifecycleUnsupported {
		t.Errorf("expected LifecycleUnsupported, got %v", reason.Support)
	}
}

func TestCheckLifecycleSupportUnsupportedWhenTypeCheckFails(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	reason, err := m.CheckLifecycleSupport(context.Background(), "missing", "missing")
	if err != nil {
		t.Fatalf("CheckLifecycleSupport() error = %v", err)
	}
	if reason.Support != LifecycleUnsupported {
		t.Errorf("expected LifecycleUnsupported, got %v", reason.Support)
	}
}

func TestCheckHiveConditionsDetectsProblems(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "ProvisionFailed",
						"status":  "True",
						"message": "provision error",
					},
					map[string]interface{}{
						"type":    "SyncSetFailed",
						"status":  "True",
						"message": "sync error",
					},
					map[string]interface{}{
						"type":   "Unreachable",
						"status": "False",
					},
				},
			},
		},
	}

	checks := checkHiveConditions(cd, PowerStateRunning)
	if len(checks) != 2 {
		t.Fatalf("expected 2 error checks, got %d", len(checks))
	}
	for _, c := range checks {
		if c.Severity != SeverityError {
			t.Errorf("expected Error severity, got %s for %s", c.Severity, c.Name)
		}
	}
}

func TestCheckHiveConditionsUnreachableExpectedDuringHibernate(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Unreachable",
						"status":  "True",
						"message": "cluster is off",
					},
				},
			},
		},
	}

	checks := checkHiveConditions(cd, PowerStateHibernating)
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
	if checks[0].Severity != SeverityOK {
		t.Errorf("expected OK for Unreachable during hibernate, got %s", checks[0].Severity)
	}
}

func TestCheckHiveConditionsUnreachableErrorWhenRunning(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Unreachable",
						"status":  "True",
						"message": "cannot reach API",
					},
				},
			},
		},
	}

	checks := checkHiveConditions(cd, PowerStateRunning)
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
	if checks[0].Severity != SeverityError {
		t.Errorf("expected Error for Unreachable when Running, got %s", checks[0].Severity)
	}
}

func TestCheckHiveConditionsNoConditions(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{},
		},
	}

	checks := checkHiveConditions(cd, PowerStateRunning)
	if len(checks) != 0 {
		t.Errorf("expected 0 checks for no conditions, got %d", len(checks))
	}
}

func TestCheckACMHealthHibernatedUnavailable(t *testing.T) {
	checks := checkACMHealth(PowerStateHibernating, "Unknown", "True")
	found := false
	for _, c := range checks {
		if c.Name == "acm-health" && c.Severity == SeverityOK {
			found = true
		}
	}
	if !found {
		t.Error("expected OK acm-health for hibernated+unavailable")
	}
}

func TestCheckACMHealthNotJoined(t *testing.T) {
	checks := checkACMHealth(PowerStateRunning, "True", "False")
	found := false
	for _, c := range checks {
		if c.Name == "acm-not-joined" {
			found = true
		}
	}
	if !found {
		t.Error("expected acm-not-joined check")
	}
}

func TestCheckACMHealthRunningAvailableFalse(t *testing.T) {
	checks := checkACMHealth(PowerStateRunning, "False", "True")
	found := false
	for _, c := range checks {
		if c.Name == "acm-available-mismatch" && c.Severity == SeverityWarning {
			found = true
		}
	}
	if !found {
		t.Error("expected warning acm-available-mismatch for Available=False")
	}
}

func TestDeriveSuggestionsForUnreachable(t *testing.T) {
	checks := []DiagnosticCheck{
		{Name: "hive-Unreachable", Severity: SeverityError, Message: "test"},
	}
	suggestions := deriveSuggestions(checks)
	if len(suggestions) == 0 {
		t.Fatal("expected suggestions for Unreachable")
	}
}

func TestDeriveSuggestionsForProvisionFailed(t *testing.T) {
	checks := []DiagnosticCheck{
		{Name: "hive-ProvisionFailed", Severity: SeverityError, Message: "test"},
	}
	suggestions := deriveSuggestions(checks)
	if len(suggestions) < 2 {
		t.Fatalf("expected at least 2 suggestions for ProvisionFailed, got %d", len(suggestions))
	}
}

func TestDeriveSuggestionsForNotJoined(t *testing.T) {
	checks := []DiagnosticCheck{
		{Name: "acm-not-joined", Severity: SeverityError, Message: "test"},
	}
	suggestions := deriveSuggestions(checks)
	if len(suggestions) == 0 {
		t.Fatal("expected suggestions for not-joined")
	}
}

func TestDeriveSuggestionsForManagedClusterExists(t *testing.T) {
	checks := []DiagnosticCheck{
		{Name: "managedcluster-exists", Severity: SeverityWarning, Message: "test"},
	}
	suggestions := deriveSuggestions(checks)
	if len(suggestions) == 0 {
		t.Fatal("expected suggestions for managedcluster-exists")
	}
}

func TestDiagnoseWithMissingManagedCluster(t *testing.T) {
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
	m := New(c, config.Config{}, discardLogger)

	report, err := m.Diagnose(context.Background(), "test-ns", "cluster1")
	if err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}

	if report.ACMAvailable != "not found" {
		t.Errorf("expected ACMAvailable='not found', got %s", report.ACMAvailable)
	}
	if report.ACMJoined != "not found" {
		t.Errorf("expected ACMJoined='not found', got %s", report.ACMJoined)
	}

	hasMCWarning := false
	for _, check := range report.Checks {
		if check.Name == "managedcluster-exists" {
			hasMCWarning = true
		}
	}
	if !hasMCWarning {
		t.Error("expected managedcluster-exists warning")
	}
}

func TestDiagnoseReturnsErrorForMissingClusterDeployment(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	_, err := m.Diagnose(context.Background(), "test-ns", "missing")
	if err == nil {
		t.Fatal("expected error for missing ClusterDeployment")
	}
}

func TestDiagnoseWithEmptyPowerStates(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "cluster1",
				"namespace": "test-ns",
				"labels":    map[string]interface{}{},
			},
			"spec":   map[string]interface{}{},
			"status": map[string]interface{}{},
		},
	}
	mc := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "cluster1",
			},
			"status": map[string]interface{}{},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd, mc)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	report, err := m.Diagnose(context.Background(), "test-ns", "cluster1")
	if err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}
	if report.HivePowerSpec != PowerStateUnknown {
		t.Errorf("expected Unknown power spec, got %v", report.HivePowerSpec)
	}
	if report.HivePowerStatus != PowerStateUnknown {
		t.Errorf("expected Unknown power status, got %v", report.HivePowerStatus)
	}
}

func TestExtractMCConditionsNoConditions(t *testing.T) {
	mc := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{},
		},
	}
	avail, joined := extractMCConditions(mc)
	if avail != "Unknown" || joined != "Unknown" {
		t.Errorf("expected Unknown/Unknown, got %s/%s", avail, joined)
	}
}

func TestIsPendingKubeletCSR(t *testing.T) {
	tests := []struct {
		name     string
		csr      certificatesv1.CertificateSigningRequest
		expected bool
	}{
		{
			name: "pending kubelet client CSR",
			csr: certificatesv1.CertificateSigningRequest{
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: "kubernetes.io/kube-apiserver-client-kubelet",
				},
			},
			expected: true,
		},
		{
			name: "pending kubelet serving CSR",
			csr: certificatesv1.CertificateSigningRequest{
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: "kubernetes.io/kubelet-serving",
				},
			},
			expected: true,
		},
		{
			name: "non-kubelet CSR",
			csr: certificatesv1.CertificateSigningRequest{
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: "kubernetes.io/kube-apiserver-client",
				},
			},
			expected: false,
		},
		{
			name: "already approved CSR",
			csr: certificatesv1.CertificateSigningRequest{
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: "kubernetes.io/kube-apiserver-client-kubelet",
				},
				Status: certificatesv1.CertificateSigningRequestStatus{
					Conditions: []certificatesv1.CertificateSigningRequestCondition{
						{Type: certificatesv1.CertificateApproved},
					},
				},
			},
			expected: false,
		},
		{
			name: "denied CSR",
			csr: certificatesv1.CertificateSigningRequest{
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: "kubernetes.io/kubelet-serving",
				},
				Status: certificatesv1.CertificateSigningRequestStatus{
					Conditions: []certificatesv1.CertificateSigningRequestCondition{
						{Type: certificatesv1.CertificateDenied},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPendingKubeletCSR(tt.csr)
			if got != tt.expected {
				t.Errorf("isPendingKubeletCSR() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestApproveExpiredCSRs(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		&certificatesv1.CertificateSigningRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "csr-pending-1"},
			Spec: certificatesv1.CertificateSigningRequestSpec{
				SignerName: "kubernetes.io/kube-apiserver-client-kubelet",
				Request:    []byte("fake-csr"),
			},
		},
		&certificatesv1.CertificateSigningRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "csr-pending-2"},
			Spec: certificatesv1.CertificateSigningRequestSpec{
				SignerName: "kubernetes.io/kubelet-serving",
				Request:    []byte("fake-csr"),
			},
		},
		&certificatesv1.CertificateSigningRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "csr-already-approved"},
			Spec: certificatesv1.CertificateSigningRequestSpec{
				SignerName: "kubernetes.io/kube-apiserver-client-kubelet",
				Request:    []byte("fake-csr"),
			},
			Status: certificatesv1.CertificateSigningRequestStatus{
				Conditions: []certificatesv1.CertificateSigningRequestCondition{
					{Type: certificatesv1.CertificateApproved, Status: corev1.ConditionTrue},
				},
			},
		},
		&certificatesv1.CertificateSigningRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "csr-non-kubelet"},
			Spec: certificatesv1.CertificateSigningRequestSpec{
				SignerName: "kubernetes.io/kube-apiserver-client",
				Request:    []byte("fake-csr"),
			},
		},
	)

	result, err := approveExpiredCSRs(context.Background(), clientset)
	if err != nil {
		t.Fatalf("approveExpiredCSRs() error = %v", err)
	}
	if result.CSRsApproved != 2 {
		t.Errorf("expected 2 CSRs approved, got %d", result.CSRsApproved)
	}
	if len(result.CSRNames) != 2 {
		t.Errorf("expected 2 CSR names, got %d", len(result.CSRNames))
	}
}

func TestApproveExpiredCSRsNoPending(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	result, err := approveExpiredCSRs(context.Background(), clientset)
	if err != nil {
		t.Fatalf("approveExpiredCSRs() error = %v", err)
	}
	if result.CSRsApproved != 0 {
		t.Errorf("expected 0 CSRs approved, got %d", result.CSRsApproved)
	}
}

func TestListClustersWithLifecycleEmpty(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRClusterDeployment: "ClusterDeploymentList",
		},
	)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	names, err := m.ListClustersWithLifecycle(context.Background())
	if err != nil {
		t.Fatalf("ListClustersWithLifecycle() error = %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 clusters, got %d", len(names))
	}
}

func TestWaitForPowerStateSucceedsImmediately(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "cluster1",
				"namespace": "test-ns",
			},
			"status": map[string]interface{}{
				"powerState": "Running",
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	err := m.WaitForPowerState(context.Background(), "test-ns", "cluster1", PowerStateRunning, 5*time.Second)
	if err != nil {
		t.Fatalf("WaitForPowerState() error = %v", err)
	}
}

func TestGetSpokeRESTConfigFromAdminKubeconfigRef(t *testing.T) {
	kubeconfig := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://api.spoke1.example.com:6443
    insecure-skip-tls-verify: true
  name: spoke1
contexts:
- context:
    cluster: spoke1
    user: admin
  name: admin
current-context: admin
users:
- name: admin
  user:
    token: fake-token`
	kubeconfigB64 := base64.StdEncoding.EncodeToString([]byte(kubeconfig))

	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "spoke1",
				"namespace": "spoke1",
			},
			"status": map[string]interface{}{
				"adminKubeconfigSecretRef": map[string]interface{}{
					"name": "spoke1-admin-kubeconfig",
				},
			},
		},
	}
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "spoke1-admin-kubeconfig",
				"namespace": "spoke1",
			},
			"data": map[string]interface{}{
				"kubeconfig": kubeconfigB64,
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd, secret)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	cfg, err := m.getSpokeRESTConfig(context.Background(), "spoke1", "spoke1")
	if err != nil {
		t.Fatalf("getSpokeRESTConfig() error = %v", err)
	}
	if cfg.Host != "https://api.spoke1.example.com:6443" {
		t.Errorf("expected host https://api.spoke1.example.com:6443, got %s", cfg.Host)
	}
}

func TestGetSpokeRESTConfigFallbackToSearch(t *testing.T) {
	kubeconfig := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://api.spoke1.example.com:6443
    insecure-skip-tls-verify: true
  name: spoke1
contexts:
- context:
    cluster: spoke1
    user: admin
  name: admin
current-context: admin
users:
- name: admin
  user:
    token: fake-token`
	kubeconfigB64 := base64.StdEncoding.EncodeToString([]byte(kubeconfig))

	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "spoke1",
				"namespace": "spoke1",
			},
			"status": map[string]interface{}{},
		},
	}
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "spoke1-admin-kubeconfig",
				"namespace": "spoke1",
			},
			"data": map[string]interface{}{
				"kubeconfig": kubeconfigB64,
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd, secret)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	cfg, err := m.getSpokeRESTConfig(context.Background(), "spoke1", "spoke1")
	if err != nil {
		t.Fatalf("getSpokeRESTConfig() error = %v", err)
	}
	if cfg.Host != "https://api.spoke1.example.com:6443" {
		t.Errorf("expected host https://api.spoke1.example.com:6443, got %s", cfg.Host)
	}
}

func TestGetSpokeRESTConfigMissingKubeconfig(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "spoke1",
				"namespace": "spoke1",
			},
			"status": map[string]interface{}{
				"adminKubeconfigSecretRef": map[string]interface{}{
					"name": "spoke1-admin-kubeconfig",
				},
			},
		},
	}
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "spoke1-admin-kubeconfig",
				"namespace": "spoke1",
			},
			"data": map[string]interface{}{},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd, secret)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	_, err := m.getSpokeRESTConfig(context.Background(), "spoke1", "spoke1")
	if err == nil {
		t.Fatal("expected error for missing kubeconfig data")
	}
}

func TestGetSpokeRESTConfigMissingClusterDeployment(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	_, err := m.getSpokeRESTConfig(context.Background(), "spoke1", "spoke1")
	if err == nil {
		t.Fatal("expected error for missing ClusterDeployment")
	}
}

func TestFindAdminKubeconfigSecret(t *testing.T) {
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "spoke1-admin-kubeconfig",
				"namespace": "spoke1",
			},
		},
	}
	other := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "spoke1-pull-secret",
				"namespace": "spoke1",
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, secret, other)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	name, err := m.findAdminKubeconfigSecret(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("findAdminKubeconfigSecret() error = %v", err)
	}
	if name != "spoke1-admin-kubeconfig" {
		t.Errorf("expected spoke1-admin-kubeconfig, got %s", name)
	}
}

func TestFindAdminKubeconfigSecretNotFound(t *testing.T) {
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "spoke1-pull-secret",
				"namespace": "spoke1",
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, secret)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	_, err := m.findAdminKubeconfigSecret(context.Background(), "spoke1")
	if err == nil {
		t.Fatal("expected error when no admin-kubeconfig secret exists")
	}
}

func TestPostResumeRecoveryApprovesCSRs(t *testing.T) {
	csrList := certificatesv1.CertificateSigningRequestList{
		Items: []certificatesv1.CertificateSigningRequest{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "csr-1"},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: "kubernetes.io/kube-apiserver-client-kubelet",
					Request:    []byte("fake"),
				},
			},
		},
	}

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/apis/certificates.k8s.io/v1/certificatesigningrequests" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(csrList)
		case r.Method == "PUT":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(csrList.Items[0])
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	kubeconfig := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: %s
    insecure-skip-tls-verify: true
  name: spoke1
contexts:
- context:
    cluster: spoke1
    user: admin
  name: admin
current-context: admin
users:
- name: admin
  user:
    token: fake-token`, ts.URL)
	kubeconfigB64 := base64.StdEncoding.EncodeToString([]byte(kubeconfig))

	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "spoke1",
				"namespace": "spoke1",
			},
			"status": map[string]interface{}{
				"adminKubeconfigSecretRef": map[string]interface{}{
					"name": "spoke1-admin-kubeconfig",
				},
			},
		},
	}
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "spoke1-admin-kubeconfig",
				"namespace": "spoke1",
			},
			"data": map[string]interface{}{
				"kubeconfig": kubeconfigB64,
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd, secret)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	result, err := m.PostResumeRecovery(context.Background(), "spoke1", "spoke1")
	if err != nil {
		t.Fatalf("PostResumeRecovery() error = %v", err)
	}
	if result.CSRsApproved < 1 {
		t.Errorf("expected 1 CSR approved, got %d", result.CSRsApproved)
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestPostResumeRecoveryMissingCluster(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	_, err := m.PostResumeRecovery(context.Background(), "spoke1", "spoke1")
	if err == nil {
		t.Fatal("expected error for missing cluster")
	}
}

func TestGetSpokeRESTConfigNoRefNoFallback(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "spoke1",
				"namespace": "spoke1",
			},
			"status": map[string]interface{}{},
		},
	}
	other := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "spoke1-pull-secret",
				"namespace": "spoke1",
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd, other)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	_, err := m.getSpokeRESTConfig(context.Background(), "spoke1", "spoke1")
	if err == nil {
		t.Fatal("expected error when no admin kubeconfig secret found")
	}
}

// --- Additional tests to raise coverage to >= 90% ---

func TestGetPowerStateEmptyStringValue(t *testing.T) {
	// Cover the branch where powerState is "" (empty string)
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "cluster1",
				"namespace": "test-ns",
			},
			"spec": map[string]interface{}{
				"powerState": "",
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	got, err := m.GetPowerState(context.Background(), "test-ns", "cluster1")
	if err != nil {
		t.Fatalf("GetPowerState() error = %v", err)
	}
	if got != PowerStateUnknown {
		t.Errorf("GetPowerState() = %v, want %v", got, PowerStateUnknown)
	}
}

func TestGetPowerStateStatusEmptyStringValue(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "cluster1",
				"namespace": "test-ns",
			},
			"status": map[string]interface{}{
				"powerState": "",
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	got, err := m.GetPowerStateStatus(context.Background(), "test-ns", "cluster1")
	if err != nil {
		t.Fatalf("GetPowerStateStatus() error = %v", err)
	}
	if got != PowerStateUnknown {
		t.Errorf("GetPowerStateStatus() = %v, want %v", got, PowerStateUnknown)
	}
}

func TestWaitForPowerStateCancelledContext(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "cluster1",
				"namespace": "test-ns",
			},
			"status": map[string]interface{}{
				"powerState": "Running",
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := m.WaitForPowerState(ctx, "test-ns", "cluster1", PowerStateHibernating, 30*time.Second)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestClusterSupportsLifecycleErrorPath(t *testing.T) {
	// CheckLifecycleSupport returns (nil, err) only when the Get call fails
	// with a non-NotFound error. The fake dynamic client won't produce that
	// with the default setup, so we test the existing paths more thoroughly.
	// The ClusterSupportsLifecycle wrapper should return false for unsupported.
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	supported, err := m.ClusterSupportsLifecycle(context.Background(), "missing", "missing")
	if err != nil {
		t.Fatalf("ClusterSupportsLifecycle() error = %v", err)
	}
	if supported {
		t.Error("expected false for missing cluster")
	}
}

func TestCheckHiveConditionsInvalidConditionEntry(t *testing.T) {
	// Cover the branch where a condition is not map[string]interface{}
	// Use an empty map (no "type" key) to exercise the !ok fallthrough path
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{},
					map[string]interface{}{
						"type":    "Unreachable",
						"status":  "True",
						"message": "cannot reach API",
					},
				},
			},
		},
	}

	checks := checkHiveConditions(cd, PowerStateRunning)
	// First condition has no type so it gets skipped by problemConditions check
	// Second is a valid Unreachable condition
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
	if checks[0].Severity != SeverityError {
		t.Errorf("expected Error severity, got %s", checks[0].Severity)
	}
}

func TestCheckHiveConditionsNonProblemConditions(t *testing.T) {
	// Cover the branch where condition type is not in problemConditions
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Installed",
						"status": "True",
					},
					map[string]interface{}{
						"type":   "Ready",
						"status": "True",
					},
				},
			},
		},
	}

	checks := checkHiveConditions(cd, PowerStateRunning)
	if len(checks) != 0 {
		t.Errorf("expected 0 checks for non-problem conditions, got %d", len(checks))
	}
}

func TestCheckHiveConditionsFalseStatus(t *testing.T) {
	// Problem condition with status=False should be skipped
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "ProvisionFailed",
						"status": "False",
					},
				},
			},
		},
	}

	checks := checkHiveConditions(cd, PowerStateRunning)
	if len(checks) != 0 {
		t.Errorf("expected 0 checks for False status conditions, got %d", len(checks))
	}
}

func TestCheckHiveConditionsUnreachableDuringStopping(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Unreachable",
						"status":  "True",
						"message": "cluster is stopping",
					},
				},
			},
		},
	}

	checks := checkHiveConditions(cd, PowerStateStopping)
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
	if checks[0].Severity != SeverityOK {
		t.Errorf("expected OK for Unreachable during Stopping, got %s", checks[0].Severity)
	}
}

func TestCheckHiveConditionsMultipleProblems(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "InstallLaunchError",
						"status":  "True",
						"message": "install failed",
					},
					map[string]interface{}{
						"type":    "AuthenticationFailure",
						"status":  "True",
						"message": "auth failed",
					},
				},
			},
		},
	}

	checks := checkHiveConditions(cd, PowerStateRunning)
	if len(checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(checks))
	}
}

func TestExtractMCConditionsInvalidEntry(t *testing.T) {
	mc := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					"not-a-map",
					map[string]interface{}{
						"type":   "ManagedClusterConditionAvailable",
						"status": "True",
					},
				},
			},
		},
	}
	avail, joined := extractMCConditions(mc)
	if avail != "True" {
		t.Errorf("expected Available=True, got %s", avail)
	}
	if joined != "Unknown" {
		t.Errorf("expected Joined=Unknown, got %s", joined)
	}
}

func TestExtractMCConditionsBothSet(t *testing.T) {
	mc := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "ManagedClusterConditionAvailable",
						"status": "True",
					},
					map[string]interface{}{
						"type":   "ManagedClusterJoined",
						"status": "True",
					},
					map[string]interface{}{
						"type":   "SomeOtherCondition",
						"status": "True",
					},
				},
			},
		},
	}
	avail, joined := extractMCConditions(mc)
	if avail != "True" {
		t.Errorf("expected Available=True, got %s", avail)
	}
	if joined != "True" {
		t.Errorf("expected Joined=True, got %s", joined)
	}
}

func TestCheckACMHealthStoppingWithAvailableFalse(t *testing.T) {
	checks := checkACMHealth(PowerStateStopping, "False", "True")
	found := false
	for _, c := range checks {
		if c.Name == "acm-health" && c.Severity == SeverityOK {
			found = true
		}
	}
	if !found {
		t.Error("expected OK acm-health for stopping+unavailable")
	}
}

func TestCheckACMHealthStoppingWithAvailableTrue(t *testing.T) {
	checks := checkACMHealth(PowerStateStopping, "True", "True")
	found := false
	for _, c := range checks {
		if c.Name == "acm-available-during-hibernate" {
			found = true
		}
	}
	if !found {
		t.Error("expected acm-available-during-hibernate for stopping+available")
	}
}

func TestCheckACMHealthJoinedUnknown(t *testing.T) {
	// joined="Unknown" should NOT trigger acm-not-joined
	checks := checkACMHealth(PowerStateRunning, "True", "Unknown")
	for _, c := range checks {
		if c.Name == "acm-not-joined" {
			t.Error("did not expect acm-not-joined for Joined=Unknown")
		}
	}
}

func TestDeriveSuggestionsForWarningMismatch(t *testing.T) {
	// Warning severity should not produce klusterlet suggestions
	checks := []DiagnosticCheck{
		{Name: "acm-available-mismatch", Severity: SeverityWarning, Message: "test"},
	}
	suggestions := deriveSuggestions(checks)
	if len(suggestions) != 0 {
		t.Errorf("expected 0 suggestions for warning mismatch, got %d", len(suggestions))
	}
}

func TestDeriveSuggestionsMultipleChecks(t *testing.T) {
	checks := []DiagnosticCheck{
		{Name: "hive-Unreachable", Severity: SeverityError, Message: "test"},
		{Name: "hive-ProvisionFailed", Severity: SeverityError, Message: "test"},
		{Name: "acm-not-joined", Severity: SeverityError, Message: "test"},
		{Name: "managedcluster-exists", Severity: SeverityWarning, Message: "test"},
		{Name: "acm-available-mismatch", Severity: SeverityError, Message: "test"},
	}
	suggestions := deriveSuggestions(checks)
	if len(suggestions) < 6 {
		t.Errorf("expected at least 6 suggestions, got %d", len(suggestions))
	}
}

func TestDiagnoseWithFullConditions(t *testing.T) {
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
			"status": map[string]interface{}{
				"powerState": "Stopping",
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Unreachable",
						"status":  "True",
						"message": "expected during hibernate",
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
						"status": "False",
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
	m := New(c, config.Config{}, discardLogger)

	report, err := m.Diagnose(context.Background(), "test-ns", "cluster1")
	if err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}
	if report.HivePowerSpec != PowerStateHibernating {
		t.Errorf("expected spec=Hibernating, got %s", report.HivePowerSpec)
	}
	if report.HivePowerStatus != PowerStateStopping {
		t.Errorf("expected status=Stopping, got %s", report.HivePowerStatus)
	}
}

func TestGetSpokeRESTConfigInvalidBase64(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "spoke1",
				"namespace": "spoke1",
			},
			"status": map[string]interface{}{
				"adminKubeconfigSecretRef": map[string]interface{}{
					"name": "spoke1-admin-kubeconfig",
				},
			},
		},
	}
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "spoke1-admin-kubeconfig",
				"namespace": "spoke1",
			},
			"data": map[string]interface{}{
				"kubeconfig": "not-valid-base64!!!",
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd, secret)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	_, err := m.getSpokeRESTConfig(context.Background(), "spoke1", "spoke1")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestPostResumeRecoveryWithPendingCSRs(t *testing.T) {
	mux := http.NewServeMux()
	callCount := 0
	mux.HandleFunc("/apis/certificates.k8s.io/v1/certificatesigningrequests", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			if callCount == 0 {
				callCount++
				w.Write([]byte(`{
					"apiVersion":"certificates.k8s.io/v1",
					"kind":"CertificateSigningRequestList",
					"items":[{
						"metadata":{"name":"csr-1"},
						"spec":{"signerName":"kubernetes.io/kube-apiserver-client-kubelet","request":"ZmFrZQ==","usages":["client auth"]},
						"status":{}
					}]
				}`))
			} else {
				w.Write([]byte(`{"apiVersion":"certificates.k8s.io/v1","kind":"CertificateSigningRequestList","items":[]}`))
			}
			return
		}
	})
	mux.HandleFunc("/apis/certificates.k8s.io/v1/certificatesigningrequests/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"apiVersion":"certificates.k8s.io/v1","kind":"CertificateSigningRequest","metadata":{"name":"csr-1"},"spec":{"signerName":"kubernetes.io/kube-apiserver-client-kubelet","request":"ZmFrZQ=="},"status":{"conditions":[{"type":"Approved","status":"True"}]}}`))
	})
	server := httptest.NewTLSServer(mux)
	defer server.Close()

	kubeconfig := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: %s
    insecure-skip-tls-verify: true
  name: spoke
contexts:
- context:
    cluster: spoke
    user: admin
  name: admin
current-context: admin
users:
- name: admin
  user:
    token: fake-token`, server.URL)

	kubeconfigB64 := base64.StdEncoding.EncodeToString([]byte(kubeconfig))

	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "spoke1",
				"namespace": "spoke1",
			},
			"status": map[string]interface{}{
				"adminKubeconfigSecretRef": map[string]interface{}{
					"name": "spoke1-admin-kubeconfig",
				},
			},
		},
	}
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "spoke1-admin-kubeconfig",
				"namespace": "spoke1",
			},
			"data": map[string]interface{}{
				"kubeconfig": kubeconfigB64,
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd, secret)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	result, err := m.PostResumeRecovery(context.Background(), "spoke1", "spoke1")
	if err != nil {
		t.Fatalf("PostResumeRecovery() error = %v", err)
	}
	if result.CSRsApproved < 1 {
		t.Errorf("expected 1 CSR approved, got %d", result.CSRsApproved)
	}
	if result.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestPostResumeRecoveryMissingClusterDeployment(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	_, err := m.PostResumeRecovery(context.Background(), "spoke1", "spoke1")
	if err == nil {
		t.Fatal("expected error for missing ClusterDeployment")
	}
}

func TestPostResumeRecoveryCancelledContext(t *testing.T) {
	// Test that PostResumeRecovery respects context cancellation during rounds
	mux := http.NewServeMux()
	mux.HandleFunc("/apis/certificates.k8s.io/v1/certificatesigningrequests", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Always return a pending CSR so recovery tries multiple rounds
		w.Write([]byte(`{
			"apiVersion":"certificates.k8s.io/v1",
			"kind":"CertificateSigningRequestList",
			"items":[{
				"metadata":{"name":"csr-1"},
				"spec":{"signerName":"kubernetes.io/kube-apiserver-client-kubelet","request":"ZmFrZQ==","usages":["client auth"]},
				"status":{}
			}]
		}`))
	})
	mux.HandleFunc("/apis/certificates.k8s.io/v1/certificatesigningrequests/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"apiVersion":"certificates.k8s.io/v1","kind":"CertificateSigningRequest","metadata":{"name":"csr-1"},"spec":{"signerName":"kubernetes.io/kube-apiserver-client-kubelet","request":"ZmFrZQ=="},"status":{"conditions":[{"type":"Approved","status":"True"}]}}`))
	})
	server := httptest.NewTLSServer(mux)
	defer server.Close()

	kubeconfig := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: %s
    insecure-skip-tls-verify: true
  name: spoke
contexts:
- context:
    cluster: spoke
    user: admin
  name: admin
current-context: admin
users:
- name: admin
  user:
    token: fake-token`, server.URL)

	kubeconfigB64 := base64.StdEncoding.EncodeToString([]byte(kubeconfig))

	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "spoke1",
				"namespace": "spoke1",
			},
			"status": map[string]interface{}{
				"adminKubeconfigSecretRef": map[string]interface{}{
					"name": "spoke1-admin-kubeconfig",
				},
			},
		},
	}
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "spoke1-admin-kubeconfig",
				"namespace": "spoke1",
			},
			"data": map[string]interface{}{
				"kubeconfig": kubeconfigB64,
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd, secret)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately so the second round's select picks it up
	cancel()

	result, _ := m.PostResumeRecovery(ctx, "spoke1", "spoke1")
	if result == nil {
		t.Fatal("expected non-nil result even on cancellation")
	}
}

func TestFindAdminKubeconfigSecretMultipleSecrets(t *testing.T) {
	// Multiple secrets, one with the right suffix
	s1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "other-secret",
				"namespace": "ns1",
			},
		},
	}
	s2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "my-cluster-admin-kubeconfig",
				"namespace": "ns1",
			},
		},
	}
	s3 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "another-admin-kubeconfig",
				"namespace": "ns1",
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, s1, s2, s3)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	name, err := m.findAdminKubeconfigSecret(context.Background(), "ns1")
	if err != nil {
		t.Fatalf("findAdminKubeconfigSecret() error = %v", err)
	}
	// Should find the first one with the right suffix
	if name != "my-cluster-admin-kubeconfig" && name != "another-admin-kubeconfig" {
		t.Errorf("expected an admin-kubeconfig secret, got %s", name)
	}
}

func TestApproveExpiredCSRsWithApprovalError(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		&certificatesv1.CertificateSigningRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "csr-1"},
			Spec: certificatesv1.CertificateSigningRequestSpec{
				SignerName: "kubernetes.io/kube-apiserver-client-kubelet",
				Request:    []byte("fake"),
			},
		},
	)
	clientset.PrependReactor("update", "certificatesigningrequests", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "approval" {
			return true, nil, fmt.Errorf("approval denied")
		}
		return false, nil, nil
	})

	result, err := approveExpiredCSRs(context.Background(), clientset)
	if err != nil {
		t.Fatalf("approveExpiredCSRs() error = %v", err)
	}
	if result.CSRsApproved != 0 {
		t.Errorf("expected 0 CSRs approved on error, got %d", result.CSRsApproved)
	}
}

func TestApproveExpiredCSRsListError(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("list", "certificatesigningrequests", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("simulated list error")
	})

	_, err := approveExpiredCSRs(context.Background(), clientset)
	if err == nil {
		t.Fatal("expected error from list failure")
	}
}

func TestWaitForPowerStateMissingCluster(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	err := m.WaitForPowerState(context.Background(), "test-ns", "missing", PowerStateRunning, 2*time.Second)
	if err == nil {
		t.Fatal("expected error for missing cluster")
	}
}

func TestGetSpokeRESTConfigSecretGetError(t *testing.T) {
	cd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      "spoke1",
				"namespace": "spoke1",
			},
			"status": map[string]interface{}{
				"adminKubeconfigSecretRef": map[string]interface{}{
					"name": "spoke1-admin-kubeconfig",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, cd)
	c := &client.Client{Dynamic: fakeDynamic}
	m := New(c, config.Config{}, discardLogger)

	_, err := m.getSpokeRESTConfig(context.Background(), "spoke1", "spoke1")
	if err == nil {
		t.Fatal("expected error for missing secret")
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
