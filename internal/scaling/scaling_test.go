package scaling

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clienttesting "k8s.io/client-go/testing"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

// gvrKinds maps all GVRs we use to their list kind strings for the fake client.
var gvrKinds = map[schema.GroupVersionResource]string{
	client.GVRMachinePool:        "MachinePoolList",
	client.GVRClusterDeployment:  "ClusterDeploymentList",
	client.GVRManagedClusterInfo: "ManagedClusterInfoList",
}

func fakeClient(objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrKinds, objs...)
	return &client.Client{Dynamic: fake}
}

func newManager(objs ...runtime.Object) *Manager {
	return New(fakeClient(objs...), config.Config{})
}

// ----- Helper constructors -----

func machinePool(name, namespace string, replicas *int64, platform string) *unstructured.Unstructured {
	spec := map[string]interface{}{
		"clusterDeploymentRef": map[string]interface{}{"name": namespace},
		"name":                 "worker",
	}
	if replicas != nil {
		spec["replicas"] = *replicas
	}
	if platform != "" {
		spec["platform"] = map[string]interface{}{
			platform: map[string]interface{}{"type": "m5.xlarge"},
		}
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "MachinePool",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": spec,
		},
	}
	return obj
}

func machinePoolWithAutoscaling(name, namespace string, min, max int64) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "MachinePool",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"clusterDeploymentRef": map[string]interface{}{"name": namespace},
				"name":                 "worker",
				"autoscaling": map[string]interface{}{
					"minReplicas": min,
					"maxReplicas": max,
				},
			},
		},
	}
	return obj
}

func clusterDeployment(name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
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
	return obj
}

func managedClusterInfo(name string, nodes []interface{}, distType string) *unstructured.Unstructured {
	status := map[string]interface{}{}
	if nodes != nil {
		status["nodeList"] = nodes
	}
	if distType != "" {
		status["distributionInfo"] = map[string]interface{}{
			"type": distType,
		}
	}
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "internal.open-cluster-management.io/v1beta1",
			"kind":       "ManagedClusterInfo",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": name,
			},
			"status": status,
		},
	}
	return obj
}

func workerNode(instanceType string) map[string]interface{} {
	labels := map[string]interface{}{
		"node-role.kubernetes.io/worker": "",
	}
	if instanceType != "" {
		labels["node.kubernetes.io/instance-type"] = instanceType
	}
	return map[string]interface{}{
		"labels": labels,
	}
}

func masterNode() map[string]interface{} {
	return map[string]interface{}{
		"labels": map[string]interface{}{
			"node-role.kubernetes.io/master": "",
		},
	}
}

// ===== coalesce =====

func TestCoalesce(t *testing.T) {
	if got := coalesce("hello", "fallback"); got != "hello" {
		t.Errorf("coalesce(%q, %q) = %q, want %q", "hello", "fallback", got, "hello")
	}
	if got := coalesce("", "fallback"); got != "fallback" {
		t.Errorf("coalesce(%q, %q) = %q, want %q", "", "fallback", got, "fallback")
	}
	if got := coalesce("", ""); got != "" {
		t.Errorf("coalesce(%q, %q) = %q, want %q", "", "", got, "")
	}
}

// ===== ErrNoMachinePool.Error() =====

func TestErrNoMachinePoolErrorHive(t *testing.T) {
	e := &ErrNoMachinePool{
		ClusterName: "prod-1",
		IsHive:      true,
		WorkerCount: 3,
		WorkerType:  "m5.xlarge",
	}
	msg := e.Error()
	if !strings.Contains(msg, "prod-1") {
		t.Error("expected cluster name in error")
	}
	if !strings.Contains(msg, "no MachinePool found") {
		t.Error("expected 'no MachinePool found' in Hive error")
	}
	if !strings.Contains(msg, "3") {
		t.Error("expected worker count in error")
	}
	if !strings.Contains(msg, "m5.xlarge") {
		t.Error("expected worker type in error")
	}
	if !strings.Contains(msg, "acmlab scaling init") {
		t.Error("expected init command in error")
	}
}

func TestErrNoMachinePoolErrorNotHive(t *testing.T) {
	e := &ErrNoMachinePool{
		ClusterName: "imported-1",
		IsHive:      false,
		VendorType:  "EKS",
	}
	msg := e.Error()
	if !strings.Contains(msg, "imported-1") {
		t.Error("expected cluster name in error")
	}
	if !strings.Contains(msg, "not provisioned by Hive") {
		t.Error("expected Hive message in error")
	}
	if !strings.Contains(msg, "EKS") {
		t.Error("expected vendor type in error")
	}
}

func TestErrNoMachinePoolErrorNotHiveEmptyVendor(t *testing.T) {
	e := &ErrNoMachinePool{
		ClusterName: "imported-2",
		IsHive:      false,
		VendorType:  "",
	}
	msg := e.Error()
	if !strings.Contains(msg, "unknown type") {
		t.Error("expected 'unknown type' fallback when VendorType is empty")
	}
}

// ===== machinePoolInfoFromUnstructured =====

func TestMachinePoolInfoFromUnstructuredFixedReplicas(t *testing.T) {
	r := int64(3)
	mp := machinePool("c1-worker", "c1", &r, "aws")
	info := machinePoolInfoFromUnstructured(mp)

	if info.Name != "c1-worker" {
		t.Errorf("Name = %q, want %q", info.Name, "c1-worker")
	}
	if info.Namespace != "c1" {
		t.Errorf("Namespace = %q, want %q", info.Namespace, "c1")
	}
	if info.Replicas == nil || *info.Replicas != 3 {
		t.Errorf("Replicas = %v, want 3", info.Replicas)
	}
	if info.Platform != "aws" {
		t.Errorf("Platform = %q, want %q", info.Platform, "aws")
	}
	if info.MinSize != nil || info.MaxSize != nil {
		t.Error("MinSize/MaxSize should be nil for fixed replicas")
	}
}

func TestMachinePoolInfoFromUnstructuredAutoscaling(t *testing.T) {
	mp := machinePoolWithAutoscaling("c2-worker", "c2", 2, 10)
	info := machinePoolInfoFromUnstructured(mp)

	if info.Replicas != nil {
		t.Errorf("Replicas should be nil with autoscaling, got %v", *info.Replicas)
	}
	if info.MinSize == nil || *info.MinSize != 2 {
		t.Errorf("MinSize = %v, want 2", info.MinSize)
	}
	if info.MaxSize == nil || *info.MaxSize != 10 {
		t.Errorf("MaxSize = %v, want 10", info.MaxSize)
	}
}

func TestMachinePoolInfoFromUnstructuredPlatforms(t *testing.T) {
	platforms := []string{"ibmvpc", "aws", "gcp", "azure"}
	for _, p := range platforms {
		r := int64(1)
		mp := machinePool("w", "ns", &r, p)
		info := machinePoolInfoFromUnstructured(mp)
		if info.Platform != p {
			t.Errorf("Platform = %q, want %q", info.Platform, p)
		}
	}
}

func TestMachinePoolInfoFromUnstructuredNoPlatform(t *testing.T) {
	r := int64(1)
	mp := machinePool("w", "ns", &r, "")
	info := machinePoolInfoFromUnstructured(mp)
	if info.Platform != "" {
		t.Errorf("Platform = %q, want empty", info.Platform)
	}
}

func TestMachinePoolInfoFromUnstructuredUnknownPlatform(t *testing.T) {
	r := int64(1)
	mp := machinePool("w", "ns", &r, "")
	// Add an unrecognized platform
	mp.Object["spec"].(map[string]interface{})["platform"] = map[string]interface{}{
		"openstack": map[string]interface{}{"flavor": "m1.large"},
	}
	info := machinePoolInfoFromUnstructured(mp)
	if info.Platform != "" {
		t.Errorf("Platform = %q, want empty for unknown platform", info.Platform)
	}
}

func TestMachinePoolInfoFromUnstructuredNoSpec(t *testing.T) {
	mp := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "MachinePool",
			"metadata": map[string]interface{}{
				"name":      "bare",
				"namespace": "ns",
			},
		},
	}
	info := machinePoolInfoFromUnstructured(mp)
	if info.Name != "bare" {
		t.Errorf("Name = %q, want %q", info.Name, "bare")
	}
	if info.Replicas != nil {
		t.Error("Replicas should be nil when no spec")
	}
	if info.Platform != "" {
		t.Error("Platform should be empty when no spec")
	}
}

// ===== ClusterSupportsScaling =====

func TestClusterSupportsScalingHive(t *testing.T) {
	mgr := newManager(clusterDeployment("hive-cluster"))

	ok, err := mgr.ClusterSupportsScaling(context.Background(), "hive-cluster")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected Hive cluster to support scaling")
	}
}

func TestClusterSupportsScalingNotHive(t *testing.T) {
	mgr := newManager() // no ClusterDeployment

	ok, err := mgr.ClusterSupportsScaling(context.Background(), "imported-cluster")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected imported cluster to NOT support scaling")
	}
}

// ===== GetCurrentWorkerInfo =====

func TestGetCurrentWorkerInfoWithWorkers(t *testing.T) {
	nodes := []interface{}{
		workerNode("m5.xlarge"),
		workerNode("m5.xlarge"),
		masterNode(),
		workerNode(""), // worker without instance type
	}
	info := managedClusterInfo("c1", nodes, "OCP")
	mgr := newManager(info)

	count, wt, err := mgr.GetCurrentWorkerInfo(context.Background(), "c1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
	if wt != "m5.xlarge" {
		t.Errorf("workerType = %q, want %q", wt, "m5.xlarge")
	}
}

func TestGetCurrentWorkerInfoNoWorkers(t *testing.T) {
	nodes := []interface{}{masterNode()}
	info := managedClusterInfo("c1", nodes, "OCP")
	mgr := newManager(info)

	count, wt, err := mgr.GetCurrentWorkerInfo(context.Background(), "c1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
	if wt != "bx2-4x16" {
		t.Errorf("workerType = %q, want default %q", wt, "bx2-4x16")
	}
}

func TestGetCurrentWorkerInfoMissingCluster(t *testing.T) {
	mgr := newManager() // no ManagedClusterInfo

	count, wt, err := mgr.GetCurrentWorkerInfo(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Falls back to defaults when ManagedClusterInfo not found
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
	if wt != "bx2-4x16" {
		t.Errorf("workerType = %q, want default %q", wt, "bx2-4x16")
	}
}

func TestGetCurrentWorkerInfoEmptyNodeList(t *testing.T) {
	info := managedClusterInfo("c1", []interface{}{}, "OCP")
	mgr := newManager(info)

	count, wt, err := mgr.GetCurrentWorkerInfo(context.Background(), "c1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
	if wt != "bx2-4x16" {
		t.Errorf("workerType = %q, want default", wt)
	}
}

func TestGetCurrentWorkerInfoBadNodeEntry(t *testing.T) {
	// Nodes list with a non-map entry that should be skipped
	nodes := []interface{}{
		"not-a-map",
		workerNode("m5.xlarge"),
	}
	info := managedClusterInfo("c1", nodes, "")
	mgr := newManager(info)

	count, wt, err := mgr.GetCurrentWorkerInfo(context.Background(), "c1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
	if wt != "m5.xlarge" {
		t.Errorf("workerType = %q, want %q", wt, "m5.xlarge")
	}
}

// ===== GetMachinePool =====

func TestGetMachinePoolSuccess(t *testing.T) {
	r := int64(3)
	mp := machinePool("c1-worker", "c1", &r, "aws")
	mgr := newManager(mp)

	info, err := mgr.GetMachinePool(context.Background(), "c1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != "c1-worker" {
		t.Errorf("Name = %q, want %q", info.Name, "c1-worker")
	}
	if info.Replicas == nil || *info.Replicas != 3 {
		t.Errorf("Replicas = %v, want 3", info.Replicas)
	}
}

func TestGetMachinePoolNotFoundHive(t *testing.T) {
	// Hive cluster (ClusterDeployment exists) but no MachinePool
	cd := clusterDeployment("c1")
	nodes := []interface{}{workerNode("m5.xlarge"), workerNode("m5.xlarge")}
	mci := managedClusterInfo("c1", nodes, "OCP")
	mgr := newManager(cd, mci)

	_, err := mgr.GetMachinePool(context.Background(), "c1")
	if err == nil {
		t.Fatal("expected error for missing MachinePool")
	}
	var noMP *ErrNoMachinePool
	if !isErrNoMachinePool(err, &noMP) {
		t.Fatalf("expected ErrNoMachinePool, got %T: %v", err, err)
	}
	if !noMP.IsHive {
		t.Error("expected IsHive=true")
	}
	if noMP.WorkerCount != 2 {
		t.Errorf("WorkerCount = %d, want 2", noMP.WorkerCount)
	}
	if noMP.WorkerType != "m5.xlarge" {
		t.Errorf("WorkerType = %q, want %q", noMP.WorkerType, "m5.xlarge")
	}
}

func TestGetMachinePoolNotFoundImported(t *testing.T) {
	// No ClusterDeployment, no MachinePool
	mgr := newManager()

	_, err := mgr.GetMachinePool(context.Background(), "imported-1")
	if err == nil {
		t.Fatal("expected error for missing MachinePool on imported cluster")
	}
	var noMP *ErrNoMachinePool
	if !isErrNoMachinePool(err, &noMP) {
		t.Fatalf("expected ErrNoMachinePool, got %T: %v", err, err)
	}
	if noMP.IsHive {
		t.Error("expected IsHive=false for imported cluster")
	}
}

// ===== InitMachinePool =====

func TestInitMachinePoolSuccess(t *testing.T) {
	cd := clusterDeployment("c1")
	mgr := newManager(cd)

	info, err := mgr.InitMachinePool(context.Background(), "c1", "m5.xlarge", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != "c1-worker" {
		t.Errorf("Name = %q, want %q", info.Name, "c1-worker")
	}
	if info.Replicas == nil || *info.Replicas != 3 {
		t.Errorf("Replicas = %v, want 3", info.Replicas)
	}
}

func TestInitMachinePoolNotHive(t *testing.T) {
	mgr := newManager() // no ClusterDeployment

	_, err := mgr.InitMachinePool(context.Background(), "imported-1", "m5.xlarge", 3)
	if err == nil {
		t.Fatal("expected error for non-Hive cluster")
	}
	var noMP *ErrNoMachinePool
	if !isErrNoMachinePool(err, &noMP) {
		t.Fatalf("expected ErrNoMachinePool, got %T: %v", err, err)
	}
	if noMP.IsHive {
		t.Error("expected IsHive=false")
	}
}

// ===== SetReplicas =====

func TestSetReplicasSuccess(t *testing.T) {
	r := int64(3)
	mp := machinePool("c1-worker", "c1", &r, "aws")
	mgr := newManager(mp)

	err := mgr.SetReplicas(context.Background(), "c1", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetReplicasNoMachinePool(t *testing.T) {
	mgr := newManager()

	err := mgr.SetReplicas(context.Background(), "c1", 5)
	if err == nil {
		t.Fatal("expected error when no MachinePool exists")
	}
}

// ===== EnableAutoscaling =====

func TestEnableAutoscalingSuccess(t *testing.T) {
	r := int64(3)
	mp := machinePool("c1-worker", "c1", &r, "aws")
	mgr := newManager(mp)

	err := mgr.EnableAutoscaling(context.Background(), "c1", 2, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnableAutoscalingNoMachinePool(t *testing.T) {
	mgr := newManager()

	err := mgr.EnableAutoscaling(context.Background(), "c1", 2, 10)
	if err == nil {
		t.Fatal("expected error when no MachinePool exists")
	}
}

// ===== DisableAutoscaling =====

func TestDisableAutoscalingSuccess(t *testing.T) {
	mp := machinePoolWithAutoscaling("c1-worker", "c1", 2, 10)
	mgr := newManager(mp)

	err := mgr.DisableAutoscaling(context.Background(), "c1", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDisableAutoscalingNoMachinePool(t *testing.T) {
	mgr := newManager()

	err := mgr.DisableAutoscaling(context.Background(), "c1", 3)
	if err == nil {
		t.Fatal("expected error when no MachinePool exists")
	}
}

// ===== ListMachinePools =====

func TestListMachinePoolsMultiple(t *testing.T) {
	r1 := int64(3)
	r2 := int64(5)
	mp1 := machinePool("c1-worker", "c1", &r1, "aws")
	mp2 := machinePool("c2-worker", "c2", &r2, "gcp")
	mgr := newManager(mp1, mp2)

	infos, err := mgr.ListMachinePools(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("got %d MachinePools, want 2", len(infos))
	}
}

func TestListMachinePoolsEmpty(t *testing.T) {
	mgr := newManager()

	infos, err := mgr.ListMachinePools(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("got %d MachinePools, want 0", len(infos))
	}
}

// ===== noMachinePoolErr =====

func TestNoMachinePoolErrHive(t *testing.T) {
	cd := clusterDeployment("c1")
	nodes := []interface{}{workerNode("bx2-4x16"), workerNode("bx2-4x16"), workerNode("bx2-4x16")}
	mci := managedClusterInfo("c1", nodes, "OCP")
	mgr := newManager(cd, mci)

	err := mgr.noMachinePoolErr(context.Background(), "c1")
	var noMP *ErrNoMachinePool
	if !isErrNoMachinePool(err, &noMP) {
		t.Fatalf("expected ErrNoMachinePool, got %T", err)
	}
	if !noMP.IsHive {
		t.Error("expected IsHive=true for Hive cluster")
	}
	if noMP.WorkerCount != 3 {
		t.Errorf("WorkerCount = %d, want 3", noMP.WorkerCount)
	}
	if noMP.WorkerType != "bx2-4x16" {
		t.Errorf("WorkerType = %q, want %q", noMP.WorkerType, "bx2-4x16")
	}
}

func TestNoMachinePoolErrImported(t *testing.T) {
	// No ClusterDeployment, but ManagedClusterInfo with distribution type
	mci := managedClusterInfo("imp-1", nil, "EKS")
	mgr := newManager(mci)

	err := mgr.noMachinePoolErr(context.Background(), "imp-1")
	var noMP *ErrNoMachinePool
	if !isErrNoMachinePool(err, &noMP) {
		t.Fatalf("expected ErrNoMachinePool, got %T", err)
	}
	if noMP.IsHive {
		t.Error("expected IsHive=false for imported cluster")
	}
	if noMP.VendorType != "EKS" {
		t.Errorf("VendorType = %q, want %q", noMP.VendorType, "EKS")
	}
}

func TestNoMachinePoolErrImportedNoInfo(t *testing.T) {
	// No ClusterDeployment, no ManagedClusterInfo
	mgr := newManager()

	err := mgr.noMachinePoolErr(context.Background(), "gone")
	var noMP *ErrNoMachinePool
	if !isErrNoMachinePool(err, &noMP) {
		t.Fatalf("expected ErrNoMachinePool, got %T", err)
	}
	if noMP.IsHive {
		t.Error("expected IsHive=false")
	}
	// VendorType should be empty (GetClusterType fails on missing resource)
}

// ===== New =====

func TestNewReturnsManager(t *testing.T) {
	c := fakeClient()
	cfg := config.Config{}
	mgr := New(c, cfg)
	if mgr == nil {
		t.Fatal("New returned nil")
	}
	if mgr.client != c {
		t.Error("manager client mismatch")
	}
}

// ===== Error path tests using reactors =====

func fakeClientWithReactor(verb, resource string, err error, objs ...runtime.Object) *client.Client {
	scheme := runtime.NewScheme()
	fake := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrKinds, objs...)
	fake.PrependReactor(verb, resource, func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, err
	})
	return &client.Client{Dynamic: fake}
}

func TestClusterSupportsScalingAPIError(t *testing.T) {
	c := fakeClientWithReactor("get", "clusterdeployments", fmt.Errorf("api server down"))
	mgr := New(c, config.Config{})

	_, err := mgr.ClusterSupportsScaling(context.Background(), "c1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "checking ClusterDeployment") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGetMachinePoolListError(t *testing.T) {
	c := fakeClientWithReactor("list", "machinepools", fmt.Errorf("connection refused"))
	mgr := New(c, config.Config{})

	_, err := mgr.GetMachinePool(context.Background(), "c1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "listing MachinePools") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestListMachinePoolsNotFoundError(t *testing.T) {
	notFound := errors.NewNotFound(schema.GroupResource{Group: "hive.openshift.io", Resource: "machinepools"}, "")
	c := fakeClientWithReactor("list", "machinepools", notFound)
	mgr := New(c, config.Config{})

	infos, err := mgr.ListMachinePools(context.Background())
	if err != nil {
		t.Fatalf("expected nil error for NotFound, got: %v", err)
	}
	if infos != nil {
		t.Errorf("expected nil result for NotFound, got %v", infos)
	}
}

func TestListMachinePoolsAPIError(t *testing.T) {
	c := fakeClientWithReactor("list", "machinepools", fmt.Errorf("timeout"))
	mgr := New(c, config.Config{})

	_, err := mgr.ListMachinePools(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "listing MachinePools") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSetReplicasPatchError(t *testing.T) {
	r := int64(3)
	mp := machinePool("c1-worker", "c1", &r, "aws")
	c := fakeClient(mp)
	// Add reactor for patch failures
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("patch", "machinepools", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("patch denied")
	})
	mgr := New(c, config.Config{})

	err := mgr.SetReplicas(context.Background(), "c1", 5)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "patching MachinePool") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestEnableAutoscalingPatchError(t *testing.T) {
	r := int64(3)
	mp := machinePool("c1-worker", "c1", &r, "aws")
	c := fakeClient(mp)
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("patch", "machinepools", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("patch denied")
	})
	mgr := New(c, config.Config{})

	err := mgr.EnableAutoscaling(context.Background(), "c1", 2, 10)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "patching MachinePool") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestInitMachinePoolCreateError(t *testing.T) {
	cd := clusterDeployment("c1")
	c := fakeClient(cd)
	c.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("create", "machinepools", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("quota exceeded")
	})
	mgr := New(c, config.Config{})

	_, err := mgr.InitMachinePool(context.Background(), "c1", "m5.xlarge", 3)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "creating MachinePool") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ----- helpers -----

// isErrNoMachinePool checks if err is *ErrNoMachinePool and assigns to target.
func isErrNoMachinePool(err error, target **ErrNoMachinePool) bool {
	e, ok := err.(*ErrNoMachinePool)
	if ok && target != nil {
		*target = e
	}
	return ok
}
