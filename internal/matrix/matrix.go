package matrix

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

type Manager struct {
	client *client.Client
	cfg    config.Config
	logger *slog.Logger
	now    func() time.Time
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger, now: time.Now}
}

type MatrixSpec struct {
	OCPVersions      []string `json:"ocpVersions" yaml:"ocpVersions"`
	Architectures    []string `json:"architectures" yaml:"architectures"`
	OperatorVersions []string `json:"operatorVersions" yaml:"operatorVersions"`
	Platform         string   `json:"platform" yaml:"platform"`
	Region           string   `json:"region" yaml:"region"`
}

type MatrixCell struct {
	Name            string `json:"name"`
	OCPVersion      string `json:"ocpVersion"`
	Architecture    string `json:"architecture"`
	OperatorVersion string `json:"operatorVersion"`
	Status          string `json:"status"`
}

type MatrixResult struct {
	Cells  []MatrixCell `json:"cells"`
	Total  int          `json:"total"`
	Ready  int          `json:"ready"`
	Failed int          `json:"failed"`
}

func (m *Manager) ProvisionMatrix(ctx context.Context, spec MatrixSpec) (*MatrixResult, error) {
	m.logger.Info("matrix.ProvisionMatrix", "versions", len(spec.OCPVersions), "archs", len(spec.Architectures), "operators", len(spec.OperatorVersions))

	if len(spec.OCPVersions) == 0 || len(spec.Architectures) == 0 || len(spec.OperatorVersions) == 0 {
		return nil, fmt.Errorf("matrix spec must have at least one value for ocpVersions, architectures, and operatorVersions")
	}

	cells := generateCombinations(spec)
	matrixID := fmt.Sprintf("%d", m.now().Unix())
	result := &MatrixResult{Total: len(cells)}

	for i, cell := range cells {
		ns := buildNamespace(cell.Name)
		if err := m.client.CreateIfNotExists(ctx, client.GVRNamespace, "", ns); err != nil {
			return nil, fmt.Errorf("creating namespace %s: %w", cell.Name, err)
		}

		cd := buildMatrixClusterDeployment(cell, m.cfg)
		if err := m.client.CreateIfNotExists(ctx, client.GVRClusterDeployment, cell.Name, cd); err != nil {
			cells[i].Status = "Failed"
			result.Failed++
			continue
		}

		labelPatch := buildMatrixLabelPatch(cell, matrixID)
		if _, err := m.client.Patch(ctx, client.GVRManagedCluster, "", cell.Name, types.MergePatchType, labelPatch); err != nil {
			m.logger.Warn("matrix: failed to label cluster", "cluster", cell.Name, "error", err)
		}

		cells[i].Status = "Provisioning"
		result.Ready++
	}

	result.Cells = cells
	return result, nil
}

func (m *Manager) DestroyMatrix(ctx context.Context, matrixID string) (*MatrixResult, error) {
	m.logger.Info("matrix.DestroyMatrix", "matrixID", matrixID)

	clusters, err := m.client.List(ctx, client.GVRManagedCluster, "", "matrix-id="+matrixID)
	if err != nil {
		return nil, fmt.Errorf("listing matrix clusters: %w", err)
	}

	result := &MatrixResult{Total: len(clusters.Items)}
	for _, obj := range clusters.Items {
		name := obj.GetName()
		cell := parseMatrixCell(obj.Object)

		if err := m.client.DeleteIfExists(ctx, client.GVRClusterDeployment, name, name); err != nil {
			cell.Status = "DeleteFailed"
			result.Failed++
		} else {
			cell.Status = "Deleting"
			result.Ready++
		}
		result.Cells = append(result.Cells, cell)
	}
	return result, nil
}

func (m *Manager) StatusMatrix(ctx context.Context, matrixID string) (*MatrixResult, error) {
	m.logger.Info("matrix.StatusMatrix", "matrixID", matrixID)

	clusters, err := m.client.List(ctx, client.GVRManagedCluster, "", "matrix-id="+matrixID)
	if err != nil {
		return nil, fmt.Errorf("listing matrix clusters: %w", err)
	}

	result := &MatrixResult{Total: len(clusters.Items)}
	for _, obj := range clusters.Items {
		cell := parseMatrixCell(obj.Object)
		name := obj.GetName()

		cd, err := m.client.Get(ctx, client.GVRClusterDeployment, name, name)
		if err != nil {
			cell.Status = "Unknown"
		} else {
			installed, _, _ := unstructured.NestedBool(cd.Object, "spec", "installed")
			if installed {
				cell.Status = "Ready"
				result.Ready++
			} else {
				cell.Status = "Provisioning"
			}
		}
		result.Cells = append(result.Cells, cell)
	}
	return result, nil
}

func (m *Manager) ListMatrixClusters(ctx context.Context) ([]MatrixCell, error) {
	m.logger.Info("matrix.ListMatrixClusters")

	clusters, err := m.client.List(ctx, client.GVRManagedCluster, "", "matrix=true")
	if err != nil {
		return nil, fmt.Errorf("listing matrix clusters: %w", err)
	}

	var cells []MatrixCell
	for _, obj := range clusters.Items {
		cells = append(cells, parseMatrixCell(obj.Object))
	}
	return cells, nil
}
