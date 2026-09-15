package decommission

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			client.GVRConfigMap:          "ConfigMapList",
			client.GVRManagedCluster:     "ManagedClusterList",
			client.GVRManagedClusterInfo: "ManagedClusterInfoList",
			client.GVRClusterDeployment:  "ClusterDeploymentList",
			client.GVRNamespace:          "NamespaceList",
			client.GVRManifestWork:       "ManifestWorkList",
		}, objs...)
	return &client.Client{Dynamic: fc}
}

func makeNamespace(name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "Namespace"})
	obj.SetName(name)
	return obj
}

func TestNextPhase(t *testing.T) {
	tests := []struct {
		current  Phase
		expected Phase
	}{
		{PhaseImported, PhaseAudited},
		{PhaseAudited, PhaseNotified},
		{PhaseNotified, PhaseBackedUp},
		{PhaseBackedUp, PhaseDrained},
		{PhaseDrained, PhaseDeleted},
		{PhaseDeleted, PhaseCleaned},
		{PhaseCleaned, PhaseCleaned},
	}
	for _, tt := range tests {
		t.Run(string(tt.current), func(t *testing.T) {
			got := nextPhase(tt.current)
			if got != tt.expected {
				t.Errorf("nextPhase(%s) = %s, want %s", tt.current, got, tt.expected)
			}
		})
	}
}

func TestStateRoundTrip(t *testing.T) {
	state := &DecommissionState{
		ClusterName: "spoke1",
		Phase:       PhaseAudited,
		Owner:       "team-alpha@example.com",
		Deadline:    "2026-09-28T00:00:00Z",
		History: []HistoryEntry{
			{Phase: PhaseImported, Timestamp: "2026-09-14T10:00:00Z", Message: "Cluster imported"},
		},
	}
	cm := stateToConfigMap(state)
	got, err := stateFromConfigMap(cm)
	if err != nil {
		t.Fatalf("stateFromConfigMap: %v", err)
	}
	if got.ClusterName != state.ClusterName {
		t.Errorf("ClusterName = %s, want %s", got.ClusterName, state.ClusterName)
	}
	if got.Phase != state.Phase {
		t.Errorf("Phase = %s, want %s", got.Phase, state.Phase)
	}
	if got.Owner != state.Owner {
		t.Errorf("Owner = %s, want %s", got.Owner, state.Owner)
	}
	if len(got.History) != 1 {
		t.Errorf("History len = %d, want 1", len(got.History))
	}
}

func TestStateRoundTripWithAudit(t *testing.T) {
	state := &DecommissionState{
		ClusterName: "spoke1",
		Phase:       PhaseAudited,
		Audit: &AuditReport{
			NodeCount:      3,
			CPUCapacity:    "24",
			MemoryCapacity: "96Gi",
			Owner:          "team-alpha@example.com",
			Platform:       "aws",
		},
	}
	cm := stateToConfigMap(state)
	got, err := stateFromConfigMap(cm)
	if err != nil {
		t.Fatalf("stateFromConfigMap: %v", err)
	}
	if got.Audit == nil {
		t.Fatal("Audit is nil after round-trip")
	}
	if got.Audit.NodeCount != 3 {
		t.Errorf("Audit.NodeCount = %d, want 3", got.Audit.NodeCount)
	}
	if got.Audit.Platform != "aws" {
		t.Errorf("Audit.Platform = %s, want aws", got.Audit.Platform)
	}
}

func TestCreateAndGetState(t *testing.T) {
	ns := makeNamespace("spoke1")
	c := fakeClient(ns)
	ctx := context.Background()

	opts := StartOpts{Owner: "team-alpha@example.com", Deadline: "2026-09-28T00:00:00Z"}
	state, err := createState(ctx, c, "spoke1", opts)
	if err != nil {
		t.Fatalf("createState: %v", err)
	}
	if state.Phase != PhaseImported {
		t.Errorf("initial phase = %s, want imported", state.Phase)
	}

	got, err := getState(ctx, c, "spoke1")
	if err != nil {
		t.Fatalf("getState: %v", err)
	}
	if got.Phase != PhaseImported {
		t.Errorf("getState phase = %s, want imported", got.Phase)
	}
	if got.Owner != "team-alpha@example.com" {
		t.Errorf("Owner = %s, want team-alpha@example.com", got.Owner)
	}
}

func TestSetState(t *testing.T) {
	ns := makeNamespace("spoke1")
	c := fakeClient(ns)
	ctx := context.Background()

	opts := StartOpts{Owner: "team-alpha@example.com"}
	state, _ := createState(ctx, c, "spoke1", opts)

	state.Phase = PhaseAudited
	state.addHistory(PhaseAudited, "Audit complete")
	if err := setState(ctx, c, state); err != nil {
		t.Fatalf("setState: %v", err)
	}

	got, _ := getState(ctx, c, "spoke1")
	if got.Phase != PhaseAudited {
		t.Errorf("phase = %s, want audited", got.Phase)
	}
	if len(got.History) != 2 {
		t.Errorf("history len = %d, want 2", len(got.History))
	}
}

func TestDeleteState(t *testing.T) {
	ns := makeNamespace("spoke1")
	c := fakeClient(ns)
	ctx := context.Background()

	createState(ctx, c, "spoke1", StartOpts{})

	if err := deleteState(ctx, c, "spoke1"); err != nil {
		t.Fatalf("deleteState: %v", err)
	}

	_, err := getState(ctx, c, "spoke1")
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestListStates(t *testing.T) {
	ns1 := makeNamespace("spoke1")
	ns2 := makeNamespace("spoke2")
	c := fakeClient(ns1, ns2)
	ctx := context.Background()

	createState(ctx, c, "spoke1", StartOpts{})
	createState(ctx, c, "spoke2", StartOpts{})

	states, err := listStates(ctx, c)
	if err != nil {
		t.Fatalf("listStates: %v", err)
	}
	if len(states) != 2 {
		t.Errorf("listStates len = %d, want 2", len(states))
	}
}

func TestAddHistory(t *testing.T) {
	s := &DecommissionState{ClusterName: "test"}
	s.addHistory(PhaseImported, "imported")
	s.addHistory(PhaseAudited, "audited")
	if len(s.History) != 2 {
		t.Errorf("history len = %d, want 2", len(s.History))
	}
	if s.History[1].Phase != PhaseAudited {
		t.Errorf("history[1].Phase = %s, want audited", s.History[1].Phase)
	}
	if s.History[1].Message != "audited" {
		t.Errorf("history[1].Message = %s, want audited", s.History[1].Message)
	}
}
