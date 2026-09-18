package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

func mockRunner(responses map[string][]byte) CmdRunner {
	return func(name string, args ...string) ([]byte, error) {
		key := name
		if len(args) > 0 {
			key = name + " " + args[0]
		}
		if data, ok := responses[key]; ok {
			return data, nil
		}
		return nil, fmt.Errorf("command not found: %s", key)
	}
}

func eksListJSON(names ...string) []byte {
	data, _ := json.Marshal(eksListOutput{Clusters: names})
	return data
}

func eksDescribeJSON(name, version, status string) []byte {
	out := eksDescribeOutput{}
	out.Cluster.Name = name
	out.Cluster.Version = version
	out.Cluster.Status = status
	data, _ := json.Marshal(out)
	return data
}

func rosaListJSON(clusters ...rosaCluster) []byte {
	data, _ := json.Marshal(clusters)
	return data
}

func ibmListJSON(clusters ...ibmCluster) []byte {
	data, _ := json.Marshal(clusters)
	return data
}

func TestScanAWSEKS(t *testing.T) {
	runner := mockRunner(map[string][]byte{
		"aws eks":  eksListJSON("my-eks-1", "my-eks-2"),
		"aws eks ": eksDescribeJSON("my-eks-1", "1.29", "ACTIVE"),
	})

	clusters, err := scanAWS(runner, "")
	if err != nil {
		t.Fatalf("scanAWS failed: %v", err)
	}
	if len(clusters) < 2 {
		t.Fatalf("expected at least 2 clusters, got %d", len(clusters))
	}

	found := false
	for _, c := range clusters {
		if c.Name == "my-eks-1" {
			found = true
			if c.Type != "eks" {
				t.Errorf("expected type eks, got %s", c.Type)
			}
			if c.Provider != "aws" {
				t.Errorf("expected provider aws, got %s", c.Provider)
			}
		}
	}
	if !found {
		t.Error("my-eks-1 not found in results")
	}
}

func TestScanAWSROSA(t *testing.T) {
	runner := mockRunner(map[string][]byte{
		"rosa list": rosaListJSON(
			rosaCluster{Name: "my-rosa", State: "ready", OpenshiftVersion: "4.14.5", AWS: rosaAWS{Region: "us-east-1"}},
		),
	})

	clusters, err := scanAWS(runner, "")
	if err != nil {
		t.Fatalf("scanAWS failed: %v", err)
	}
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	if clusters[0].Type != "rosa" {
		t.Errorf("expected type rosa, got %s", clusters[0].Type)
	}
	if clusters[0].Version != "4.14.5" {
		t.Errorf("expected version 4.14.5, got %s", clusters[0].Version)
	}
	if clusters[0].Region != "us-east-1" {
		t.Errorf("expected region us-east-1, got %s", clusters[0].Region)
	}
}

func TestScanIBMCloud(t *testing.T) {
	runner := mockRunner(map[string][]byte{
		"ibmcloud ks": ibmListJSON(
			ibmCluster{Name: "my-iks", Region: "us-south", State: "normal", Type: "kubernetes", MasterKubeVersion: "1.28.4"},
			ibmCluster{Name: "my-roks", Region: "eu-de", State: "normal", Type: "openshift", MasterKubeVersion: "4.14.5_openshift"},
		),
	})

	clusters, err := scanIBMCloud(runner, "")
	if err != nil {
		t.Fatalf("scanIBMCloud failed: %v", err)
	}
	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(clusters))
	}

	for _, c := range clusters {
		if c.Name == "my-iks" && c.Type != "iks" {
			t.Errorf("expected type iks, got %s", c.Type)
		}
		if c.Name == "my-roks" && c.Type != "roks" {
			t.Errorf("expected type roks, got %s", c.Type)
		}
	}
}

func TestScanIBMCloudRegionFilter(t *testing.T) {
	runner := mockRunner(map[string][]byte{
		"ibmcloud ks": ibmListJSON(
			ibmCluster{Name: "us-cluster", Region: "us-south", State: "normal", Type: "kubernetes", MasterKubeVersion: "1.28"},
			ibmCluster{Name: "eu-cluster", Region: "eu-de", State: "normal", Type: "kubernetes", MasterKubeVersion: "1.28"},
		),
	})

	clusters, err := scanIBMCloud(runner, "us-south")
	if err != nil {
		t.Fatalf("scanIBMCloud failed: %v", err)
	}
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster after filter, got %d", len(clusters))
	}
	if clusters[0].Name != "us-cluster" {
		t.Errorf("expected us-cluster, got %s", clusters[0].Name)
	}
}

func TestScanClustersMarkManaged(t *testing.T) {
	mc := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata":   map[string]interface{}{"name": "my-eks-1"},
		},
	}
	mgr := newManager(mc)
	mgr.cmdRunner = mockRunner(map[string][]byte{
		"aws eks":  eksListJSON("my-eks-1", "my-eks-2"),
		"aws eks ": eksDescribeJSON("my-eks-1", "1.29", "ACTIVE"),
	})

	clusters, err := mgr.ScanClusters(context.Background(), ScanOpts{Provider: "aws"})
	if err != nil {
		t.Fatalf("ScanClusters failed: %v", err)
	}

	for _, c := range clusters {
		if c.Name == "my-eks-1" && !c.Managed {
			t.Error("my-eks-1 should be marked as managed")
		}
		if c.Name == "my-eks-2" && c.Managed {
			t.Error("my-eks-2 should not be marked as managed")
		}
	}
}

func TestScanClustersMissingCLISkipsProvider(t *testing.T) {
	mgr := newManager()
	mgr.cmdRunner = func(name string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("command not found")
	}

	clusters, err := mgr.ScanClusters(context.Background(), ScanOpts{Provider: "aws"})
	if err != nil {
		t.Fatalf("expected no error when CLI missing, got: %v", err)
	}
	if len(clusters) != 0 {
		t.Errorf("expected 0 clusters when CLI missing, got %d", len(clusters))
	}
}

func TestScanClustersInvalidProvider(t *testing.T) {
	mgr := newManager()
	_, err := mgr.ScanClusters(context.Background(), ScanOpts{Provider: "gcp"})
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestAutoImport(t *testing.T) {
	mgr := newManager()
	mgr.cmdRunner = mockRunner(map[string][]byte{
		"aws eks":  eksListJSON("my-eks-1"),
		"aws eks ": eksDescribeJSON("my-eks-1", "1.29", "ACTIVE"),
	})

	preview, err := mgr.AutoImport(context.Background(), "my-eks-1", "aws", ImportOpts{})
	if err != nil {
		t.Fatalf("AutoImport failed: %v", err)
	}
	if preview.ClusterSet != "default" {
		t.Errorf("expected default cluster set, got %s", preview.ClusterSet)
	}

	mc, err := mgr.client.Get(context.Background(), client.GVRManagedCluster, "", "my-eks-1")
	if err != nil {
		t.Fatalf("ManagedCluster not created: %v", err)
	}
	labels := mc.GetLabels()
	if labels["acmlab.redhat.com/discovered-from"] != "aws" {
		t.Error("missing discovered-from label")
	}
	if labels["created-via"] != "cloud-discovery" {
		t.Error("missing created-via=cloud-discovery label")
	}
}

func TestAutoImportNotFound(t *testing.T) {
	mgr := newManager()
	mgr.cmdRunner = mockRunner(map[string][]byte{
		"aws eks": eksListJSON(),
	})

	_, err := mgr.AutoImport(context.Background(), "nonexistent", "aws", ImportOpts{})
	if err == nil {
		t.Fatal("expected error for nonexistent cluster")
	}
}

func TestAutoImportIBMCloud(t *testing.T) {
	mgr := newManager()
	mgr.cmdRunner = mockRunner(map[string][]byte{
		"ibmcloud ks": ibmListJSON(
			ibmCluster{Name: "my-roks", Region: "us-south", State: "normal", Type: "openshift", MasterKubeVersion: "4.14.5"},
		),
	})

	_, err := mgr.AutoImport(context.Background(), "my-roks", "ibmcloud", ImportOpts{})
	if err != nil {
		t.Fatalf("AutoImport failed: %v", err)
	}

	mc, err := mgr.client.Get(context.Background(), client.GVRManagedCluster, "", "my-roks")
	if err != nil {
		t.Fatalf("ManagedCluster not created: %v", err)
	}
	labels := mc.GetLabels()
	if labels["cluster-type"] != "roks" {
		t.Errorf("expected cluster-type roks, got %s", labels["cluster-type"])
	}
}

func TestAutoImportInvalidProvider(t *testing.T) {
	mgr := newManager()
	_, err := mgr.AutoImport(context.Background(), "cluster", "gcp", ImportOpts{})
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestBuildCloudManagedCluster(t *testing.T) {
	mc := buildCloudManagedCluster("test-eks", "aws", "eks", "us-east-1", "default")

	labels := mc.GetLabels()
	if labels["cloud"] != "aws" {
		t.Errorf("expected cloud=aws, got %s", labels["cloud"])
	}
	if labels["cluster-type"] != "eks" {
		t.Errorf("expected cluster-type=eks, got %s", labels["cluster-type"])
	}
	if labels["region"] != "us-east-1" {
		t.Errorf("expected region=us-east-1, got %s", labels["region"])
	}

	hubAccepts, _, _ := unstructured.NestedBool(mc.Object, "spec", "hubAcceptsClient")
	if !hubAccepts {
		t.Error("expected hubAcceptsClient=true")
	}
}

func writeKubeconfig(t *testing.T, dir, filename, clusterName, server string) string {
	t.Helper()
	content := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: %s
  name: %s
contexts:
- context:
    cluster: %s
    user: admin
  name: %s
users:
- name: admin
  user:
    token: test-token
current-context: %s
`, server, clusterName, clusterName, clusterName, clusterName)
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing kubeconfig: %v", err)
	}
	return path
}

func TestScanKubeconfigs(t *testing.T) {
	dir := t.TempDir()
	writeKubeconfig(t, dir, "cluster-a.kubeconfig", "cluster-a", "https://api.cluster-a.example.com:6443")
	writeKubeconfig(t, dir, "cluster-b.kubeconfig", "cluster-b", "https://api.cluster-b.example.com:6443")
	os.WriteFile(filepath.Join(dir, "not-a-kubeconfig.txt"), []byte("hello"), 0644)

	mgr := newManager()
	clusters, err := mgr.ScanKubeconfigs(context.Background(), dir)
	if err != nil {
		t.Fatalf("ScanKubeconfigs failed: %v", err)
	}
	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(clusters))
	}
	for _, c := range clusters {
		if c.Name != "cluster-a" && c.Name != "cluster-b" {
			t.Errorf("unexpected cluster name: %s", c.Name)
		}
		if c.Managed {
			t.Errorf("cluster %s should not be managed", c.Name)
		}
	}
}

func TestScanKubeconfigsManagedCluster(t *testing.T) {
	dir := t.TempDir()
	writeKubeconfig(t, dir, "managed.kubeconfig", "spoke1", "https://api.spoke1.example.com:6443")
	writeKubeconfig(t, dir, "unmanaged.kubeconfig", "new-cluster", "https://api.new.example.com:6443")

	mc := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata":   map[string]interface{}{"name": "spoke1"},
		},
	}
	mgr := newManager(mc)
	clusters, err := mgr.ScanKubeconfigs(context.Background(), dir)
	if err != nil {
		t.Fatalf("ScanKubeconfigs failed: %v", err)
	}
	for _, c := range clusters {
		if c.Name == "spoke1" && !c.Managed {
			t.Error("spoke1 should be marked as managed")
		}
		if c.Name == "new-cluster" && c.Managed {
			t.Error("new-cluster should not be marked as managed")
		}
	}
}

func TestAutoImportKubeconfig(t *testing.T) {
	dir := t.TempDir()
	path := writeKubeconfig(t, dir, "spoke.kubeconfig", "new-spoke", "https://api.new-spoke.example.com:6443")

	mgr := newManager()
	preview, err := mgr.AutoImportKubeconfig(context.Background(), "new-spoke", path, ImportOpts{})
	if err != nil {
		t.Fatalf("AutoImportKubeconfig failed: %v", err)
	}
	if preview.ClusterSet != "default" {
		t.Errorf("expected default cluster set, got %s", preview.ClusterSet)
	}

	mc, err := mgr.client.Get(context.Background(), client.GVRManagedCluster, "", "new-spoke")
	if err != nil {
		t.Fatalf("ManagedCluster not created: %v", err)
	}
	labels := mc.GetLabels()
	if labels["created-via"] != "kubeconfig-discovery" {
		t.Errorf("expected created-via=kubeconfig-discovery, got %s", labels["created-via"])
	}

	_, err = mgr.client.Get(context.Background(), client.GVRSecret, "new-spoke", "auto-import-secret")
	if err != nil {
		t.Fatal("auto-import secret not created")
	}
}

func TestAutoImportKubeconfigFileNotFound(t *testing.T) {
	mgr := newManager()
	_, err := mgr.AutoImportKubeconfig(context.Background(), "test", "/nonexistent/kubeconfig", ImportOpts{})
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestScanKubeconfigsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	mgr := newManager()
	clusters, err := mgr.ScanKubeconfigs(context.Background(), dir)
	if err != nil {
		t.Fatalf("ScanKubeconfigs failed: %v", err)
	}
	if len(clusters) != 0 {
		t.Errorf("expected 0 clusters, got %d", len(clusters))
	}
}

func TestAutoImportKubeconfigDryRun(t *testing.T) {
	dir := t.TempDir()
	path := writeKubeconfig(t, dir, "spoke.kubeconfig", "dry-spoke", "https://api.dry-spoke.example.com:6443")

	mgr := newManager()
	preview, err := mgr.AutoImportKubeconfig(context.Background(), "dry-spoke", path, ImportOpts{DryRun: true})
	if err != nil {
		t.Fatalf("AutoImportKubeconfig dry-run failed: %v", err)
	}
	if preview.ClusterName != "dry-spoke" {
		t.Errorf("expected cluster name dry-spoke, got %s", preview.ClusterName)
	}
	if len(preview.Resources) != 4 {
		t.Errorf("expected 4 preview resources, got %d", len(preview.Resources))
	}

	_, err = mgr.client.Get(context.Background(), client.GVRManagedCluster, "", "dry-spoke")
	if err == nil {
		t.Fatal("ManagedCluster should NOT be created in dry-run mode")
	}
}

func TestAutoImportKubeconfigClusterSet(t *testing.T) {
	dir := t.TempDir()
	path := writeKubeconfig(t, dir, "spoke.kubeconfig", "set-spoke", "https://api.set-spoke.example.com:6443")

	mgr := newManager()
	preview, err := mgr.AutoImportKubeconfig(context.Background(), "set-spoke", path, ImportOpts{ClusterSet: "production"})
	if err != nil {
		t.Fatalf("AutoImportKubeconfig with cluster-set failed: %v", err)
	}
	if preview.ClusterSet != "production" {
		t.Errorf("expected cluster set production, got %s", preview.ClusterSet)
	}

	mc, err := mgr.client.Get(context.Background(), client.GVRManagedCluster, "", "set-spoke")
	if err != nil {
		t.Fatalf("ManagedCluster not created: %v", err)
	}
	labels := mc.GetLabels()
	if labels["cluster.open-cluster-management.io/clusterset"] != "production" {
		t.Errorf("expected clusterset=production, got %s", labels["cluster.open-cluster-management.io/clusterset"])
	}
}

func TestAutoImportClusterSet(t *testing.T) {
	mgr := newManager()
	mgr.cmdRunner = mockRunner(map[string][]byte{
		"aws eks":  eksListJSON("my-eks-set"),
		"aws eks ": eksDescribeJSON("my-eks-set", "1.29", "ACTIVE"),
	})

	preview, err := mgr.AutoImport(context.Background(), "my-eks-set", "aws", ImportOpts{ClusterSet: "staging"})
	if err != nil {
		t.Fatalf("AutoImport with cluster-set failed: %v", err)
	}
	if preview.ClusterSet != "staging" {
		t.Errorf("expected cluster set staging, got %s", preview.ClusterSet)
	}

	mc, err := mgr.client.Get(context.Background(), client.GVRManagedCluster, "", "my-eks-set")
	if err != nil {
		t.Fatalf("ManagedCluster not created: %v", err)
	}
	labels := mc.GetLabels()
	if labels["cluster.open-cluster-management.io/clusterset"] != "staging" {
		t.Errorf("expected clusterset=staging, got %s", labels["cluster.open-cluster-management.io/clusterset"])
	}
}

func TestAutoImportDryRun(t *testing.T) {
	mgr := newManager()
	mgr.cmdRunner = mockRunner(map[string][]byte{
		"aws eks":  eksListJSON("dry-eks"),
		"aws eks ": eksDescribeJSON("dry-eks", "1.29", "ACTIVE"),
	})

	preview, err := mgr.AutoImport(context.Background(), "dry-eks", "aws", ImportOpts{DryRun: true, ClusterSet: "test-set"})
	if err != nil {
		t.Fatalf("AutoImport dry-run failed: %v", err)
	}
	if preview.ClusterName != "dry-eks" {
		t.Errorf("expected cluster name dry-eks, got %s", preview.ClusterName)
	}
	if preview.ClusterSet != "test-set" {
		t.Errorf("expected cluster set test-set, got %s", preview.ClusterSet)
	}
	if preview.Provider != "aws" {
		t.Errorf("expected provider aws, got %s", preview.Provider)
	}

	_, err = mgr.client.Get(context.Background(), client.GVRManagedCluster, "", "dry-eks")
	if err == nil {
		t.Fatal("ManagedCluster should NOT be created in dry-run mode")
	}
}

func TestListImports(t *testing.T) {
	mc1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "imported-a",
				"labels": map[string]interface{}{
					"acmlab.redhat.com/discovery":                   "true",
					"created-via":                                   "kubeconfig-discovery",
					"cluster.open-cluster-management.io/clusterset": "production",
				},
			},
		},
	}
	mc2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "imported-b",
				"labels": map[string]interface{}{
					"acmlab.redhat.com/discovery":                   "true",
					"created-via":                                   "cloud-discovery",
					"cluster.open-cluster-management.io/clusterset": "staging",
				},
			},
		},
	}
	mcNoDiscovery := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "manual-cluster",
			},
		},
	}
	mgr := newManager(mc1, mc2, mcNoDiscovery)

	imports, err := mgr.ListImports(context.Background())
	if err != nil {
		t.Fatalf("ListImports failed: %v", err)
	}
	if len(imports) != 2 {
		t.Fatalf("expected 2 imported clusters, got %d", len(imports))
	}

	found := map[string]bool{}
	for _, imp := range imports {
		found[imp.Name] = true
	}
	if !found["imported-a"] || !found["imported-b"] {
		t.Errorf("expected imported-a and imported-b, got %v", found)
	}
	if found["manual-cluster"] {
		t.Error("manual-cluster should not appear in discovery imports list")
	}
}

func TestListImportsEmpty(t *testing.T) {
	mgr := newManager()
	imports, err := mgr.ListImports(context.Background())
	if err != nil {
		t.Fatalf("ListImports failed: %v", err)
	}
	if len(imports) != 0 {
		t.Errorf("expected 0 imports, got %d", len(imports))
	}
}
