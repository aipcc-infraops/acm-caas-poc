package recovery

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

var gvrKinds = map[schema.GroupVersionResource]string{
	client.GVRManifestWork:        "ManifestWorkList",
	client.GVRConfigurationPolicy: "ConfigurationPolicyList",
	client.GVRApplicationSet:      "ApplicationSetList",
	client.GVRManagedCluster:      "ManagedClusterList",
}

func managedClusterObj(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": name,
			},
		},
	}
}

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrKinds, objs...)
	return &client.Client{Dynamic: fake}
}

func newManager(objs ...runtime.Object) *Manager {
	return New(fakeClient(objs...), config.Config{}, discardLogger)
}

func drLabelWork(cluster, pairName, role string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      "dr-labels-" + pairName,
				"namespace": cluster,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
					"acmlab.redhat.com/dr-role": role,
					"acmlab.redhat.com/dr-pair": pairName,
				},
			},
		},
	}
}

func restoreManifestWork(target, pairName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      "dr-restore-" + pairName,
				"namespace": target,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
					"acmlab.redhat.com/dr-pair": pairName,
				},
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Applied",
						"status": "True",
					},
				},
			},
		},
	}
}

func failoverAppSet(pairName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "ApplicationSet",
			"metadata": map[string]interface{}{
				"name":      "dr-failover-" + pairName,
				"namespace": "openshift-gitops",
				"labels": map[string]interface{}{
					"acmlab.redhat.com/managed": "true",
					"acmlab.redhat.com/dr-pair": pairName,
				},
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "ResourcesUpToDate",
						"status": "True",
					},
				},
			},
		},
	}
}

func TestNewReturnsManager(t *testing.T) {
	mgr := newManager()
	if mgr == nil {
		t.Fatal("New returned nil")
	}
}

func TestEnableDRCreatesResources(t *testing.T) {
	mgr := newManager(managedClusterObj("prod-east"), managedClusterObj("prod-west"))
	err := mgr.EnableDR(context.Background(), DROpts{
		SourceCluster: "prod-east",
		TargetCluster: "prod-west",
	})
	if err != nil {
		t.Fatalf("EnableDR failed: %v", err)
	}

	pairName := "prod-east-prod-west"
	_, err = mgr.client.Get(context.Background(), client.GVRManifestWork, "prod-east", "dr-labels-"+pairName)
	if err != nil {
		t.Errorf("source label ManifestWork not found: %v", err)
	}
	_, err = mgr.client.Get(context.Background(), client.GVRManifestWork, "prod-west", "dr-labels-"+pairName)
	if err != nil {
		t.Errorf("target label ManifestWork not found: %v", err)
	}
	_, err = mgr.client.Get(context.Background(), client.GVRManifestWork, "prod-east", "dr-velero-"+pairName)
	if err != nil {
		t.Errorf("Velero ManifestWork not found: %v", err)
	}
}

func TestEnableDRIdempotent(t *testing.T) {
	mgr := newManager(managedClusterObj("prod-east"), managedClusterObj("prod-west"))
	opts := DROpts{SourceCluster: "prod-east", TargetCluster: "prod-west"}
	if err := mgr.EnableDR(context.Background(), opts); err != nil {
		t.Fatalf("first enable: %v", err)
	}
	if err := mgr.EnableDR(context.Background(), opts); err != nil {
		t.Fatalf("second enable should be idempotent: %v", err)
	}
}

func TestEnableDRWithCustomOpts(t *testing.T) {
	mgr := newManager(managedClusterObj("prod-east"), managedClusterObj("prod-west"))
	err := mgr.EnableDR(context.Background(), DROpts{
		SourceCluster: "prod-east",
		TargetCluster: "prod-west",
		PairName:      "custom-pair",
		Schedule:      "0 0 * * *",
		TTL:           "168h",
	})
	if err != nil {
		t.Fatalf("EnableDR failed: %v", err)
	}

	_, err = mgr.client.Get(context.Background(), client.GVRManifestWork, "prod-east", "dr-velero-custom-pair")
	if err != nil {
		t.Errorf("custom pair Velero ManifestWork not found: %v", err)
	}
}

func TestEnableDRSourceError(t *testing.T) {
	c := fakeClient(managedClusterObj("prod-east"), managedClusterObj("prod-west"))
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "manifestworks", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("forbidden")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.EnableDR(context.Background(), DROpts{
		SourceCluster: "prod-east",
		TargetCluster: "prod-west",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "labelling source cluster") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTriggerFailover(t *testing.T) {
	mgr := newManager()
	err := mgr.TriggerFailover(context.Background(), "prod-east", "prod-west")
	if err != nil {
		t.Fatalf("TriggerFailover failed: %v", err)
	}

	pairName := "prod-east-prod-west"
	_, err = mgr.client.Get(context.Background(), client.GVRManifestWork, "prod-west", "dr-restore-"+pairName)
	if err != nil {
		t.Errorf("restore ManifestWork not found: %v", err)
	}
	_, err = mgr.client.Get(context.Background(), client.GVRApplicationSet, "openshift-gitops", "dr-failover-"+pairName)
	if err != nil {
		t.Errorf("failover ApplicationSet not found: %v", err)
	}
}

func TestTriggerFailoverError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "manifestworks", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("timeout")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.TriggerFailover(context.Background(), "prod-east", "prod-west")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFailoverStatusCompleted(t *testing.T) {
	pairName := "prod-east-prod-west"
	mgr := newManager(
		restoreManifestWork("prod-west", pairName),
		failoverAppSet(pairName),
	)

	status, err := mgr.FailoverStatus(context.Background(), "prod-east", "prod-west")
	if err != nil {
		t.Fatalf("FailoverStatus failed: %v", err)
	}
	if status.Phase != "Completed" {
		t.Errorf("Phase = %q, want Completed", status.Phase)
	}
	if status.RestorePhase != "Applied" {
		t.Errorf("RestorePhase = %q, want Applied", status.RestorePhase)
	}
	if status.GitOpsStatus != "Synced" {
		t.Errorf("GitOpsStatus = %q, want Synced", status.GitOpsStatus)
	}
}

func TestFailoverStatusNotStarted(t *testing.T) {
	mgr := newManager()
	status, err := mgr.FailoverStatus(context.Background(), "prod-east", "prod-west")
	if err != nil {
		t.Fatalf("FailoverStatus failed: %v", err)
	}
	if status.Phase != "NotStarted" {
		t.Errorf("Phase = %q, want NotStarted", status.Phase)
	}
}

func TestFailoverStatusError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("get", "manifestworks", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("timeout")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.FailoverStatus(context.Background(), "prod-east", "prod-west")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDisableDR(t *testing.T) {
	pairName := "prod-east-prod-west"
	mgr := newManager(
		drLabelWork("prod-east", pairName, "primary"),
		drLabelWork("prod-west", pairName, "standby"),
	)

	err := mgr.DisableDR(context.Background(), "prod-east")
	if err != nil {
		t.Fatalf("DisableDR failed: %v", err)
	}
}

func TestDisableDRNoMatchReturnsError(t *testing.T) {
	mgr := newManager()
	err := mgr.DisableDR(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent DR pair")
	}
}

func TestListDRPairs(t *testing.T) {
	pairName := "prod-east-prod-west"
	mgr := newManager(
		drLabelWork("prod-east", pairName, "primary"),
		drLabelWork("prod-west", pairName, "standby"),
	)

	pairs, err := mgr.ListDRPairs(context.Background())
	if err != nil {
		t.Fatalf("ListDRPairs failed: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("got %d pairs, want 1", len(pairs))
	}
	if pairs[0].SourceCluster != "prod-east" {
		t.Errorf("SourceCluster = %q", pairs[0].SourceCluster)
	}
	if pairs[0].TargetCluster != "prod-west" {
		t.Errorf("TargetCluster = %q", pairs[0].TargetCluster)
	}
	if pairs[0].Status != "Configured" {
		t.Errorf("Status = %q, want Configured", pairs[0].Status)
	}
}

func TestListDRPairsEmpty(t *testing.T) {
	mgr := newManager()
	pairs, err := mgr.ListDRPairs(context.Background())
	if err != nil {
		t.Fatalf("ListDRPairs failed: %v", err)
	}
	if len(pairs) != 0 {
		t.Errorf("got %d pairs, want 0", len(pairs))
	}
}

func TestListDRPairsError(t *testing.T) {
	c := fakeClient()
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "manifestworks", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("timeout")
	})
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.ListDRPairs(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDRPairName(t *testing.T) {
	name := drPairName("source", "target")
	if name != "source-target" {
		t.Errorf("drPairName = %q, want source-target", name)
	}
}

func TestDeriveFailoverPhase(t *testing.T) {
	tests := []struct {
		restore string
		gitops  string
		want    string
	}{
		{"NotStarted", "NotDeployed", "NotStarted"},
		{"Applied", "Synced", "Completed"},
		{"Pending", "Pending", "InProgress"},
		{"Applied", "Pending", "InProgress"},
	}
	for _, tt := range tests {
		got := deriveFailoverPhase(tt.restore, tt.gitops)
		if got != tt.want {
			t.Errorf("deriveFailoverPhase(%q, %q) = %q, want %q", tt.restore, tt.gitops, got, tt.want)
		}
	}
}

func TestParseManifestWorkPhase(t *testing.T) {
	tests := []struct {
		name string
		obj  map[string]interface{}
		want string
	}{
		{"no status", map[string]interface{}{}, "Pending"},
		{"applied", map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{"type": "Applied", "status": "True"},
				},
			},
		}, "Applied"},
		{"not applied", map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{"type": "Applied", "status": "False"},
				},
			},
		}, "Pending"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseManifestWorkPhase(tt.obj)
			if got != tt.want {
				t.Errorf("parseManifestWorkPhase = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildVeleroManifestWorkStructure(t *testing.T) {
	work := buildVeleroManifestWork(DROpts{
		SourceCluster: "prod-east",
		PairName:      "prod-east-prod-west",
		Schedule:      "0 */4 * * *",
		TTL:           "720h",
	})

	if work.GetName() != "dr-velero-prod-east-prod-west" {
		t.Errorf("Name = %q", work.GetName())
	}
	if work.GetNamespace() != "prod-east" {
		t.Errorf("Namespace = %q", work.GetNamespace())
	}
	labels := work.GetLabels()
	if labels["acmlab.redhat.com/dr-type"] != "velero-schedule" {
		t.Error("missing dr-type label")
	}
}

func TestBuildFailoverApplicationSetStructure(t *testing.T) {
	appSet := buildFailoverApplicationSet("prod-east", "prod-west", "prod-east-prod-west")
	if appSet.GetName() != "dr-failover-prod-east-prod-west" {
		t.Errorf("Name = %q", appSet.GetName())
	}
	if appSet.GetNamespace() != "openshift-gitops" {
		t.Errorf("Namespace = %q", appSet.GetNamespace())
	}
}
