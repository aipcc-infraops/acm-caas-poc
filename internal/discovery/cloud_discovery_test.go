package discovery

import (
	"context"
	"encoding/json"
	"fmt"
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

	err := mgr.AutoImport(context.Background(), "my-eks-1", "aws")
	if err != nil {
		t.Fatalf("AutoImport failed: %v", err)
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

	err := mgr.AutoImport(context.Background(), "nonexistent", "aws")
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

	err := mgr.AutoImport(context.Background(), "my-roks", "ibmcloud")
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
	err := mgr.AutoImport(context.Background(), "cluster", "gcp")
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestBuildCloudManagedCluster(t *testing.T) {
	mc := buildCloudManagedCluster("test-eks", "aws", "eks", "us-east-1")

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

// Suppress unused import warning
var _ = client.GVRManagedCluster
