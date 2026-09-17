package matrix

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

var matrixGVRKinds = map[schema.GroupVersionResource]string{
	client.GVRClusterDeployment: "ClusterDeploymentList",
	client.GVRManagedCluster:    "ManagedClusterList",
	client.GVRNamespace:         "NamespaceList",
}

func newTestManager(t *testing.T, objects ...runtime.Object) *Manager {
	t.Helper()
	scheme := runtime.NewScheme()
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, matrixGVRKinds, objects...)
	c := &client.Client{Dynamic: dyn}
	cfg := config.Config{Platform: "ibmcloud", IBMCloudRegion: "us-south"}
	return New(c, cfg, discardLogger)
}

func TestGenerateCombinations(t *testing.T) {
	spec := MatrixSpec{
		OCPVersions:      []string{"4.18", "4.19"},
		Architectures:    []string{"amd64", "arm64"},
		OperatorVersions: []string{"2.5"},
	}
	cells := generateCombinations(spec)
	if len(cells) != 4 {
		t.Fatalf("expected 4 cells, got %d", len(cells))
	}
	if cells[0].Name != "matrix-418-amd64-25" {
		t.Errorf("unexpected cell name: %s", cells[0].Name)
	}
}

func TestGenerateCombinations_SingleValues(t *testing.T) {
	spec := MatrixSpec{
		OCPVersions:      []string{"4.19"},
		Architectures:    []string{"amd64"},
		OperatorVersions: []string{"3.0"},
	}
	cells := generateCombinations(spec)
	if len(cells) != 1 {
		t.Fatalf("expected 1 cell, got %d", len(cells))
	}
}

func TestCellName(t *testing.T) {
	got := cellName("4.18", "s390x", "2.5.1")
	if got != "matrix-418-s390x-251" {
		t.Errorf("unexpected cell name: %s", got)
	}
}

func TestBuildMatrixClusterDeployment(t *testing.T) {
	cell := MatrixCell{Name: "matrix-418-amd64-25", OCPVersion: "4.18", Architecture: "amd64", OperatorVersion: "2.5"}
	cfg := config.Config{Platform: "aws", IBMCloudRegion: "us-east-1"}
	cd := buildMatrixClusterDeployment(cell, cfg)
	if cd.GetName() != "matrix-418-amd64-25" {
		t.Errorf("unexpected name: %s", cd.GetName())
	}
	labels := cd.GetLabels()
	if labels["ocp-version"] != "4.18" {
		t.Errorf("expected ocp-version label 4.18, got %s", labels["ocp-version"])
	}
	if labels["arch"] != "amd64" {
		t.Errorf("expected arch label amd64, got %s", labels["arch"])
	}
}

func TestBuildMatrixClusterDeployment_DefaultPlatform(t *testing.T) {
	cell := MatrixCell{Name: "test", OCPVersion: "4.19", Architecture: "arm64", OperatorVersion: "3.0"}
	cfg := config.Config{}
	cd := buildMatrixClusterDeployment(cell, cfg)
	spec, _, _ := unstructured.NestedMap(cd.Object, "spec", "platform")
	if _, ok := spec["ibmcloud"]; !ok {
		t.Error("expected default platform ibmcloud")
	}
}

func TestBuildMatrixLabelPatch(t *testing.T) {
	cell := MatrixCell{OCPVersion: "4.18", Architecture: "amd64", OperatorVersion: "2.5"}
	patch := buildMatrixLabelPatch(cell, "12345")
	if len(patch) == 0 {
		t.Error("expected non-empty patch")
	}
}

func TestBuildNamespace(t *testing.T) {
	ns := buildNamespace("test-ns")
	if ns.GetName() != "test-ns" {
		t.Errorf("unexpected name: %s", ns.GetName())
	}
}

func TestParseMatrixCell(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "matrix-418-amd64-25",
			"labels": map[string]interface{}{
				"ocp-version":         "4.18",
				"arch":                "amd64",
				"ai-platform-version": "2.5",
			},
		},
	}
	cell := parseMatrixCell(obj)
	if cell.Name != "matrix-418-amd64-25" {
		t.Errorf("unexpected name: %s", cell.Name)
	}
	if cell.OCPVersion != "4.18" || cell.Architecture != "amd64" || cell.OperatorVersion != "2.5" {
		t.Error("unexpected cell values")
	}
}

func TestProvisionMatrix_EmptySpec(t *testing.T) {
	mgr := newTestManager(t)
	_, err := mgr.ProvisionMatrix(context.Background(), MatrixSpec{})
	if err == nil {
		t.Error("expected error for empty spec")
	}
}

func TestProvisionMatrix_Success(t *testing.T) {
	mgr := newTestManager(t)
	spec := MatrixSpec{
		OCPVersions:      []string{"4.18"},
		Architectures:    []string{"amd64"},
		OperatorVersions: []string{"2.5"},
	}
	result, err := mgr.ProvisionMatrix(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("expected 1 total, got %d", result.Total)
	}
}

func TestDestroyMatrix_NoClusters(t *testing.T) {
	mgr := newTestManager(t)
	result, err := mgr.DestroyMatrix(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("expected 0 total, got %d", result.Total)
	}
}

func TestStatusMatrix_NoClusters(t *testing.T) {
	mgr := newTestManager(t)
	result, err := mgr.StatusMatrix(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("expected 0 total, got %d", result.Total)
	}
}

func TestListMatrixClusters_Empty(t *testing.T) {
	mgr := newTestManager(t)
	cells, err := mgr.ListMatrixClusters(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cells) != 0 {
		t.Errorf("expected 0 cells, got %d", len(cells))
	}
}

func TestListMatrixClusters_WithClusters(t *testing.T) {
	mc := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "matrix-418-amd64-25",
				"labels": map[string]interface{}{
					"matrix":              "true",
					"ocp-version":         "4.18",
					"arch":                "amd64",
					"ai-platform-version": "2.5",
				},
			},
		},
	}
	mgr := newTestManager(t, mc)
	cells, err := mgr.ListMatrixClusters(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cells) != 1 {
		t.Errorf("expected 1 cell, got %d", len(cells))
	}
}

func TestProvisionMatrix_MultipleCells(t *testing.T) {
	mgr := newTestManager(t)
	spec := MatrixSpec{
		OCPVersions:      []string{"4.18", "4.19"},
		Architectures:    []string{"amd64", "arm64"},
		OperatorVersions: []string{"2.5"},
	}
	result, err := mgr.ProvisionMatrix(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 4 {
		t.Errorf("expected 4 total, got %d", result.Total)
	}
}
