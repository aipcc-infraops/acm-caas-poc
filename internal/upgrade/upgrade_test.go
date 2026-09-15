package upgrade

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
	client.GVRManagedCluster:     "ManagedClusterList",
	client.GVRManagedClusterInfo: "ManagedClusterInfoList",
	client.GVRClusterDeployment:  "ClusterDeploymentList",
	client.GVRClusterImageSet:    "ClusterImageSetList",
	client.GVRManifestWork:       "ManifestWorkList",
	client.GVRClusterCurator:     "ClusterCuratorList",
}

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrKinds, objs...)
	return &client.Client{Dynamic: fake}
}

func newManager(objs ...runtime.Object) *Manager {
	return New(fakeClient(objs...), config.Config{}, discardLogger)
}

func managedClusterInfo(name string, ocpVersion, channel, desiredVersion string, availableUpdates []string) *unstructured.Unstructured {
	ocp := map[string]interface{}{
		"version": ocpVersion,
		"channel": channel,
	}
	if desiredVersion != "" {
		ocp["desiredVersion"] = desiredVersion
	}
	if availableUpdates != nil {
		updates := make([]interface{}, len(availableUpdates))
		for i, u := range availableUpdates {
			updates[i] = u
		}
		ocp["availableUpdates"] = updates
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "internal.open-cluster-management.io/v1beta1",
			"kind":       "ManagedClusterInfo",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": name,
			},
			"status": map[string]interface{}{
				"distributionInfo": map[string]interface{}{
					"type": "OCP",
					"ocp":  ocp,
				},
			},
		},
	}
}

func managedClusterInfoK8s(name, gitVersion string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "internal.open-cluster-management.io/v1beta1",
			"kind":       "ManagedClusterInfo",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": name,
			},
			"status": map[string]interface{}{
				"distributionInfo": map[string]interface{}{
					"type": "kubernetes",
					"k8s": map[string]interface{}{
						"gitVersion": gitVersion,
					},
				},
			},
		},
	}
}

func managedClusterInfoWithHistory(name string, history []interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "internal.open-cluster-management.io/v1beta1",
			"kind":       "ManagedClusterInfo",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": name,
			},
			"status": map[string]interface{}{
				"distributionInfo": map[string]interface{}{
					"type": "OCP",
					"ocp": map[string]interface{}{
						"version":        "4.16.5",
						"channel":        "stable-4.16",
						"versionHistory": history,
					},
				},
			},
		},
	}
}

func clusterDeployment(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": name,
			},
			"spec": map[string]interface{}{
				"platform": map[string]interface{}{
					"aws": map[string]interface{}{},
				},
			},
		},
	}
}

func managedCluster(name string) *unstructured.Unstructured {
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

func TestNewReturnsManager(t *testing.T) {
	c := fakeClient()
	mgr := New(c, config.Config{}, discardLogger)
	if mgr == nil {
		t.Fatal("New returned nil")
	}
	if mgr.client != c {
		t.Error("manager client mismatch")
	}
}

func TestGetUpgradeStatusOCPHive(t *testing.T) {
	cd := clusterDeployment("spoke1")
	mci := managedClusterInfo("spoke1", "4.16.5", "stable-4.16", "", []string{"4.16.6", "4.16.7"})
	mgr := newManager(cd, mci)

	status, err := mgr.GetUpgradeStatus(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.UpgradeMethod != UpgradeMethodHive {
		t.Errorf("UpgradeMethod = %q, want %q", status.UpgradeMethod, UpgradeMethodHive)
	}
	if status.CurrentVersion != "4.16.5" {
		t.Errorf("CurrentVersion = %q, want %q", status.CurrentVersion, "4.16.5")
	}
	if status.Channel != "stable-4.16" {
		t.Errorf("Channel = %q, want %q", status.Channel, "stable-4.16")
	}
	if len(status.Available) != 2 {
		t.Errorf("Available = %v, want 2 versions", status.Available)
	}
}

func TestGetUpgradeStatusOCPImported(t *testing.T) {
	mci := managedClusterInfo("imported1", "4.15.10", "stable-4.15", "", []string{"4.15.11"})
	mgr := newManager(mci)

	status, err := mgr.GetUpgradeStatus(context.Background(), "imported1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.UpgradeMethod != UpgradeMethodManifest {
		t.Errorf("UpgradeMethod = %q, want %q", status.UpgradeMethod, UpgradeMethodManifest)
	}
	if status.CurrentVersion != "4.15.10" {
		t.Errorf("CurrentVersion = %q, want %q", status.CurrentVersion, "4.15.10")
	}
}

func TestGetUpgradeStatusK8s(t *testing.T) {
	mci := managedClusterInfoK8s("eks1", "v1.29.3")
	mgr := newManager(mci)

	status, err := mgr.GetUpgradeStatus(context.Background(), "eks1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.UpgradeMethod != UpgradeMethodReportOnly {
		t.Errorf("UpgradeMethod = %q, want %q", status.UpgradeMethod, UpgradeMethodReportOnly)
	}
	if status.CurrentVersion != "v1.29.3" {
		t.Errorf("CurrentVersion = %q, want %q", status.CurrentVersion, "v1.29.3")
	}
}

func TestGetUpgradeStatusProgressing(t *testing.T) {
	mci := managedClusterInfo("spoke1", "4.16.5", "stable-4.16", "4.16.6", nil)
	cd := clusterDeployment("spoke1")
	mgr := newManager(cd, mci)

	status, err := mgr.GetUpgradeStatus(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Progressing {
		t.Error("expected Progressing=true when version != desiredVersion")
	}
}

func TestGetUpgradeStatusMissingInfo(t *testing.T) {
	mgr := newManager()

	_, err := mgr.GetUpgradeStatus(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing ManagedClusterInfo")
	}
}

func TestListUpgradeableFiltersCorrectly(t *testing.T) {
	mc1 := managedCluster("spoke1")
	mc2 := managedCluster("eks1")
	mc3 := managedCluster("spoke2")
	mc4 := managedCluster("local-cluster")

	mci1 := managedClusterInfo("spoke1", "4.16.5", "stable-4.16", "", []string{"4.16.6"})
	cd1 := clusterDeployment("spoke1")
	mci2 := managedClusterInfoK8s("eks1", "v1.29.3")
	mci3 := managedClusterInfo("spoke2", "4.16.7", "stable-4.16", "", nil)
	cd3 := clusterDeployment("spoke2")

	mgr := newManager(mc1, mc2, mc3, mc4, mci1, cd1, mci2, mci3, cd3)

	result, err := mgr.ListUpgradeable(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 upgradeable cluster, got %d", len(result))
	}
	if result[0].Cluster != "spoke1" {
		t.Errorf("expected spoke1, got %s", result[0].Cluster)
	}
}

func TestListUpgradeableEmpty(t *testing.T) {
	mgr := newManager()

	result, err := mgr.ListUpgradeable(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 upgradeable clusters, got %d", len(result))
	}
}

func TestSetChannelHive(t *testing.T) {
	cd := clusterDeployment("spoke1")
	mci := managedClusterInfo("spoke1", "4.16.5", "stable-4.16", "", nil)
	mgr := newManager(cd, mci)

	err := mgr.SetChannel(context.Background(), "spoke1", "fast-4.16")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cc, err := mgr.client.Get(context.Background(), client.GVRClusterCurator, "spoke1", "spoke1")
	if err != nil {
		t.Fatalf("ClusterCurator not created: %v", err)
	}
	labels := cc.GetLabels()
	if labels["caas-poc/operation"] != "upgrade" {
		t.Errorf("missing caas-poc/operation label")
	}
	curation, _, _ := unstructured.NestedString(cc.Object, "spec", "desiredCuration")
	if curation != "upgrade" {
		t.Errorf("desiredCuration = %s, want upgrade", curation)
	}
	channel, _, _ := unstructured.NestedString(cc.Object, "spec", "upgrade", "channel")
	if channel != "fast-4.16" {
		t.Errorf("channel = %s, want fast-4.16", channel)
	}
}

func TestSetChannelImported(t *testing.T) {
	mci := managedClusterInfo("imported1", "4.15.10", "stable-4.15", "", nil)
	mgr := newManager(mci)

	err := mgr.SetChannel(context.Background(), "imported1", "fast-4.15")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetChannelK8s(t *testing.T) {
	mci := managedClusterInfoK8s("eks1", "v1.29.3")
	mgr := newManager(mci)

	err := mgr.SetChannel(context.Background(), "eks1", "stable")
	if err == nil {
		t.Fatal("expected error for k8s cluster")
	}
	if !strings.Contains(err.Error(), "does not support channel changes") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSetChannelIdempotent(t *testing.T) {
	cd := clusterDeployment("spoke1")
	mci := managedClusterInfo("spoke1", "4.16.5", "stable-4.16", "", nil)
	mgr := newManager(cd, mci)

	err := mgr.SetChannel(context.Background(), "spoke1", "fast-4.16")
	if err != nil {
		t.Fatalf("first SetChannel failed: %v", err)
	}

	err = mgr.SetChannel(context.Background(), "spoke1", "candidate-4.16")
	if err != nil {
		t.Fatalf("second SetChannel failed: %v", err)
	}

	cc, _ := mgr.client.Get(context.Background(), client.GVRClusterCurator, "spoke1", "spoke1")
	channel, _, _ := unstructured.NestedString(cc.Object, "spec", "upgrade", "channel")
	if channel != "candidate-4.16" {
		t.Errorf("channel = %s, want candidate-4.16", channel)
	}
}

func TestStartUpgradeHive(t *testing.T) {
	cd := clusterDeployment("spoke1")
	mci := managedClusterInfo("spoke1", "4.16.5", "stable-4.16", "", []string{"4.16.6"})
	mgr := newManager(cd, mci)

	err := mgr.StartUpgrade(context.Background(), "spoke1", "4.16.6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cc, err := mgr.client.Get(context.Background(), client.GVRClusterCurator, "spoke1", "spoke1")
	if err != nil {
		t.Fatalf("ClusterCurator not created: %v", err)
	}
	labels := cc.GetLabels()
	if labels["caas-poc/operation"] != "upgrade" {
		t.Errorf("missing caas-poc/operation label")
	}
	desiredUpdate, _, _ := unstructured.NestedString(cc.Object, "spec", "upgrade", "desiredUpdate")
	if desiredUpdate != "4.16.6" {
		t.Errorf("desiredUpdate = %s, want 4.16.6", desiredUpdate)
	}
}

func TestStartUpgradeImported(t *testing.T) {
	mci := managedClusterInfo("imported1", "4.15.10", "stable-4.15", "", []string{"4.15.11"})
	mgr := newManager(mci)

	err := mgr.StartUpgrade(context.Background(), "imported1", "4.15.11")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartUpgradeK8s(t *testing.T) {
	mci := managedClusterInfoK8s("eks1", "v1.29.3")
	mgr := newManager(mci)

	err := mgr.StartUpgrade(context.Background(), "eks1", "v1.30.0")
	if err == nil {
		t.Fatal("expected error for k8s cluster")
	}
	if !strings.Contains(err.Error(), "does not support upgrades") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStartUpgradeIdempotent(t *testing.T) {
	cd := clusterDeployment("spoke1")
	mci := managedClusterInfo("spoke1", "4.16.5", "stable-4.16", "", nil)
	mgr := newManager(cd, mci)

	err := mgr.StartUpgrade(context.Background(), "spoke1", "4.16.6")
	if err != nil {
		t.Fatalf("first StartUpgrade failed: %v", err)
	}

	err = mgr.StartUpgrade(context.Background(), "spoke1", "4.16.7")
	if err != nil {
		t.Fatalf("second StartUpgrade failed: %v", err)
	}

	cc, _ := mgr.client.Get(context.Background(), client.GVRClusterCurator, "spoke1", "spoke1")
	desiredUpdate, _, _ := unstructured.NestedString(cc.Object, "spec", "upgrade", "desiredUpdate")
	if desiredUpdate != "4.16.7" {
		t.Errorf("desiredUpdate = %s, want 4.16.7", desiredUpdate)
	}
}

func TestGetHistorySuccess(t *testing.T) {
	history := []interface{}{
		map[string]interface{}{
			"version":        "4.16.5",
			"state":          "Completed",
			"startedTime":    "2026-09-01T10:00:00Z",
			"completionTime": "2026-09-01T11:30:00Z",
		},
		map[string]interface{}{
			"version":     "4.16.4",
			"state":       "Completed",
			"startedTime": "2026-08-15T08:00:00Z",
		},
	}
	mci := managedClusterInfoWithHistory("spoke1", history)
	mgr := newManager(mci)

	entries, err := mgr.GetHistory(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(entries))
	}
	if entries[0].Version != "4.16.5" {
		t.Errorf("Version = %q, want %q", entries[0].Version, "4.16.5")
	}
	if entries[0].State != "Completed" {
		t.Errorf("State = %q, want %q", entries[0].State, "Completed")
	}
	if entries[0].CompletedAt != "2026-09-01T11:30:00Z" {
		t.Errorf("CompletedAt = %q, want %q", entries[0].CompletedAt, "2026-09-01T11:30:00Z")
	}
}

func TestGetHistoryNoHistory(t *testing.T) {
	mci := managedClusterInfo("spoke1", "4.16.5", "stable-4.16", "", nil)
	mgr := newManager(mci)

	entries, err := mgr.GetHistory(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil history, got %v", entries)
	}
}

func TestGetHistoryMissingCluster(t *testing.T) {
	mgr := newManager()

	_, err := mgr.GetHistory(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing cluster")
	}
}

func TestGetHistoryBadEntry(t *testing.T) {
	history := []interface{}{
		"not-a-map",
		map[string]interface{}{
			"version": "4.16.5",
			"state":   "Completed",
		},
	}
	mci := managedClusterInfoWithHistory("spoke1", history)
	mgr := newManager(mci)

	entries, err := mgr.GetHistory(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 valid entry, got %d", len(entries))
	}
}

func TestDetectUpgradeMethodHive(t *testing.T) {
	cd := clusterDeployment("spoke1")
	mgr := newManager(cd)

	method := mgr.detectUpgradeMethod(context.Background(), "spoke1", client.ClusterTypeOCP)
	if method != UpgradeMethodHive {
		t.Errorf("method = %q, want %q", method, UpgradeMethodHive)
	}
}

func TestDetectUpgradeMethodManifest(t *testing.T) {
	mgr := newManager()

	method := mgr.detectUpgradeMethod(context.Background(), "imported1", client.ClusterTypeOCP)
	if method != UpgradeMethodManifest {
		t.Errorf("method = %q, want %q", method, UpgradeMethodManifest)
	}
}

func TestDetectUpgradeMethodK8s(t *testing.T) {
	mgr := newManager()

	method := mgr.detectUpgradeMethod(context.Background(), "eks1", client.ClusterTypeKubernetes)
	if method != UpgradeMethodReportOnly {
		t.Errorf("method = %q, want %q", method, UpgradeMethodReportOnly)
	}
}

func TestDetectUpgradeMethodUnknown(t *testing.T) {
	mgr := newManager()

	method := mgr.detectUpgradeMethod(context.Background(), "unknown1", client.ClusterTypeUnknown)
	if method != UpgradeMethodReportOnly {
		t.Errorf("method = %q, want %q", method, UpgradeMethodReportOnly)
	}
}

func TestBuildChannelManifestWork(t *testing.T) {
	mw := buildChannelManifestWork("spoke1", "spoke1-channel", "fast-4.16")

	if mw.GetName() != "spoke1-channel" {
		t.Errorf("Name = %q, want %q", mw.GetName(), "spoke1-channel")
	}
	if mw.GetNamespace() != "spoke1" {
		t.Errorf("Namespace = %q, want %q", mw.GetNamespace(), "spoke1")
	}
	labels := mw.GetLabels()
	if labels["caas-poc/operation"] != "upgrade" {
		t.Error("missing caas-poc/operation label")
	}
}

func TestBuildUpgradeManifestWork(t *testing.T) {
	mw := buildUpgradeManifestWork("spoke1", "spoke1-upgrade", "4.16.6")

	if mw.GetName() != "spoke1-upgrade" {
		t.Errorf("Name = %q, want %q", mw.GetName(), "spoke1-upgrade")
	}
	labels := mw.GetLabels()
	if labels["caas-poc/cluster"] != "spoke1" {
		t.Error("missing caas-poc/cluster label")
	}
}

func fakeClientWithReactor(verb, resource string, reactorErr error, objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrKinds, objs...)
	fake.PrependReactor(verb, resource, func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, reactorErr
	})
	return &client.Client{Dynamic: fake}
}

func TestSetChannelCreateError(t *testing.T) {
	cd := clusterDeployment("spoke1")
	mci := managedClusterInfo("spoke1", "4.16.5", "stable-4.16", "", nil)
	c := fakeClient(cd, mci)
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "clustercurators", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("quota exceeded")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.SetChannel(context.Background(), "spoke1", "fast-4.16")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "creating ClusterCurator") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStartUpgradeCreateError(t *testing.T) {
	cd := clusterDeployment("spoke1")
	mci := managedClusterInfo("spoke1", "4.16.5", "stable-4.16", "", nil)
	c := fakeClient(cd, mci)
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "clustercurators", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("network error")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.StartUpgrade(context.Background(), "spoke1", "4.16.6")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "creating ClusterCurator") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestListUpgradeableAPIError(t *testing.T) {
	c := fakeClientWithReactor("list", "managedclusters", fmt.Errorf("connection refused"))
	mgr := New(c, config.Config{}, discardLogger)

	_, err := mgr.ListUpgradeable(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "listing ManagedClusters") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFillOCPStatusUpgradeFailed(t *testing.T) {
	mci := managedClusterInfo("spoke1", "4.16.5", "stable-4.16", "", nil)
	unstructured.SetNestedField(mci.Object, true, "status", "distributionInfo", "ocp", "upgradeFailed")
	mgr := newManager(mci)

	status, err := mgr.GetUpgradeStatus(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.UpgradeFailed {
		t.Error("expected UpgradeFailed=true")
	}
}

func TestFillHiveDesiredVersionWithImageSet(t *testing.T) {
	cd := clusterDeployment("spoke1")
	unstructured.SetNestedField(cd.Object, "ocp-4.16.6", "spec", "provisioning", "imageSetRef", "name")

	imageSet := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterImageSet",
			"metadata": map[string]interface{}{
				"name": "ocp-4.16.6",
			},
			"spec": map[string]interface{}{
				"releaseImage": "quay.io/openshift-release-dev/ocp-release:4.16.6-x86_64",
			},
		},
	}

	mci := managedClusterInfo("spoke1", "4.16.5", "stable-4.16", "", nil)
	mgr := newManager(cd, imageSet, mci)

	status, err := mgr.GetUpgradeStatus(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.DesiredVersion != "quay.io/openshift-release-dev/ocp-release:4.16.6-x86_64" {
		t.Errorf("DesiredVersion = %q, want release image", status.DesiredVersion)
	}
}

func TestFillHiveDesiredVersionNoImageSetRef(t *testing.T) {
	cd := clusterDeployment("spoke1")
	mci := managedClusterInfo("spoke1", "4.16.5", "stable-4.16", "", nil)
	mgr := newManager(cd, mci)

	status, err := mgr.GetUpgradeStatus(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.DesiredVersion != "" {
		t.Errorf("DesiredVersion = %q, want empty", status.DesiredVersion)
	}
}

func TestFillHiveDesiredVersionImageSetNotFound(t *testing.T) {
	cd := clusterDeployment("spoke1")
	unstructured.SetNestedField(cd.Object, "missing-set", "spec", "provisioning", "imageSetRef", "name")
	mci := managedClusterInfo("spoke1", "4.16.5", "stable-4.16", "", nil)
	mgr := newManager(cd, mci)

	status, err := mgr.GetUpgradeStatus(context.Background(), "spoke1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.DesiredVersion != "" {
		t.Errorf("DesiredVersion = %q, want empty when image set not found", status.DesiredVersion)
	}
}

func TestGetUpgradeStatusUnknownTypeFallbackToOCP(t *testing.T) {
	mci := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "internal.open-cluster-management.io/v1beta1",
			"kind":       "ManagedClusterInfo",
			"metadata": map[string]interface{}{
				"name":      "mystery",
				"namespace": "mystery",
			},
			"status": map[string]interface{}{
				"distributionInfo": map[string]interface{}{
					"type": "",
					"ocp": map[string]interface{}{
						"version": "4.16.5",
						"channel": "stable-4.16",
					},
				},
			},
		},
	}
	mgr := newManager(mci)

	status, err := mgr.GetUpgradeStatus(context.Background(), "mystery")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.CurrentVersion != "4.16.5" {
		t.Errorf("CurrentVersion = %q, want %q", status.CurrentVersion, "4.16.5")
	}
}

func TestGetUpgradeStatusUnknownTypeFallbackToK8s(t *testing.T) {
	mci := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "internal.open-cluster-management.io/v1beta1",
			"kind":       "ManagedClusterInfo",
			"metadata": map[string]interface{}{
				"name":      "mystery2",
				"namespace": "mystery2",
			},
			"status": map[string]interface{}{
				"distributionInfo": map[string]interface{}{
					"type": "",
					"k8s": map[string]interface{}{
						"gitVersion": "v1.29.0",
					},
				},
			},
		},
	}
	mgr := newManager(mci)

	status, err := mgr.GetUpgradeStatus(context.Background(), "mystery2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.CurrentVersion != "v1.29.0" {
		t.Errorf("CurrentVersion = %q, want %q", status.CurrentVersion, "v1.29.0")
	}
}

func TestSetChannelUpdateError(t *testing.T) {
	cd := clusterDeployment("spoke1")
	mci := managedClusterInfo("spoke1", "4.16.5", "stable-4.16", "", nil)
	c := fakeClient(cd, mci)
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.SetChannel(context.Background(), "spoke1", "fast-4.16")
	if err != nil {
		t.Fatalf("first SetChannel failed: %v", err)
	}

	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("update", "clustercurators", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("update denied")
	})

	err = mgr.SetChannel(context.Background(), "spoke1", "candidate-4.16")
	if err == nil {
		t.Fatal("expected error on update")
	}
	if !strings.Contains(err.Error(), "updating ClusterCurator") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStartUpgradeUpdateError(t *testing.T) {
	cd := clusterDeployment("spoke1")
	mci := managedClusterInfo("spoke1", "4.16.5", "stable-4.16", "", nil)
	c := fakeClient(cd, mci)
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.StartUpgrade(context.Background(), "spoke1", "4.16.6")
	if err != nil {
		t.Fatalf("first StartUpgrade failed: %v", err)
	}

	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("update", "clustercurators", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("update denied")
	})

	err = mgr.StartUpgrade(context.Background(), "spoke1", "4.16.7")
	if err == nil {
		t.Fatal("expected error on update")
	}
	if !strings.Contains(err.Error(), "updating ClusterCurator") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSetChannelGetError(t *testing.T) {
	cd := clusterDeployment("spoke1")
	mci := managedClusterInfo("spoke1", "4.16.5", "stable-4.16", "", nil)
	c := fakeClient(cd, mci)
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("get", "clustercurators", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("server error")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.SetChannel(context.Background(), "spoke1", "fast-4.16")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "checking ClusterCurator") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStartUpgradeGetError(t *testing.T) {
	cd := clusterDeployment("spoke1")
	mci := managedClusterInfo("spoke1", "4.16.5", "stable-4.16", "", nil)
	c := fakeClient(cd, mci)
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("get", "clustercurators", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("server error")
	})
	mgr := New(c, config.Config{}, discardLogger)

	err := mgr.StartUpgrade(context.Background(), "spoke1", "4.16.6")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "checking ClusterCurator") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuildUpgradeClusterCurator(t *testing.T) {
	cc := buildUpgradeClusterCurator("spoke1", "4.16.6", "stable-4.16")
	if cc.GetName() != "spoke1" {
		t.Errorf("Name = %q, want spoke1", cc.GetName())
	}
	desiredUpdate, _, _ := unstructured.NestedString(cc.Object, "spec", "upgrade", "desiredUpdate")
	if desiredUpdate != "4.16.6" {
		t.Errorf("desiredUpdate = %q, want 4.16.6", desiredUpdate)
	}
	channel, _, _ := unstructured.NestedString(cc.Object, "spec", "upgrade", "channel")
	if channel != "stable-4.16" {
		t.Errorf("channel = %q, want stable-4.16", channel)
	}
}

func TestBuildUpgradeClusterCuratorNoChannel(t *testing.T) {
	cc := buildUpgradeClusterCurator("spoke1", "4.16.6", "")
	upgrade, _, _ := unstructured.NestedMap(cc.Object, "spec", "upgrade")
	if _, exists := upgrade["channel"]; exists {
		t.Error("channel should not be set when empty")
	}
}

func TestBuildChannelClusterCurator(t *testing.T) {
	cc := buildChannelClusterCurator("spoke1", "fast-4.16")
	if cc.GetName() != "spoke1" {
		t.Errorf("Name = %q, want spoke1", cc.GetName())
	}
	channel, _, _ := unstructured.NestedString(cc.Object, "spec", "upgrade", "channel")
	if channel != "fast-4.16" {
		t.Errorf("channel = %q, want fast-4.16", channel)
	}
	curation, _, _ := unstructured.NestedString(cc.Object, "spec", "desiredCuration")
	if curation != "upgrade" {
		t.Errorf("desiredCuration = %q, want upgrade", curation)
	}
}

func TestFillK8sStatusEmpty(t *testing.T) {
	mci := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "internal.open-cluster-management.io/v1beta1",
			"kind":       "ManagedClusterInfo",
			"metadata": map[string]interface{}{
				"name":      "empty1",
				"namespace": "empty1",
			},
			"status": map[string]interface{}{
				"distributionInfo": map[string]interface{}{
					"type": "kubernetes",
					"k8s":  map[string]interface{}{},
				},
			},
		},
	}
	mgr := newManager(mci)

	status, err := mgr.GetUpgradeStatus(context.Background(), "empty1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.CurrentVersion != "" {
		t.Errorf("CurrentVersion = %q, want empty", status.CurrentVersion)
	}
}
