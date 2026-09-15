package observability

import (
	"context"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

const (
	Namespace      = "open-cluster-management-observability"
	MinIOName      = "minio"
	MinIOPort      = 9000
	ThanosCfgKey   = "thanos.yaml"
	SecretName     = "thanos-object-storage"
	MCOName        = "observability"
	StorageClass   = "ibmc-vpc-block-10iops-tier"
	MinIOPVCSize   = "20Gi"
	MinIOAccessKey = "minio"
	MinIOSecretKey = "minio123"
	MinioBucket    = "thanos"
)

type Manager struct {
	client *client.Client
	cfg    config.Config
	logger *slog.Logger
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger}
}

func (m *Manager) Setup(ctx context.Context) error {
	m.logger.Info("observability.Setup")
	steps := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"namespace", m.ensureNamespace},
		{"minio-pvc", m.ensureMinioPVC},
		{"minio-deployment", m.ensureMinioDeployment},
		{"minio-service", m.ensureMinioService},
		{"thanos-secret", m.ensureThanosSecret},
		{"multiclusterobservability", m.ensureMCO},
	}
	for _, s := range steps {
		if err := s.fn(ctx); err != nil {
			return fmt.Errorf("setup %s: %w", s.name, err)
		}
	}
	return nil
}

func (m *Manager) Teardown(ctx context.Context) error {
	m.logger.Info("observability.Teardown")
	steps := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"multiclusterobservability", m.deleteMCO},
		{"thanos-secret", m.deleteSecret},
		{"minio-service", m.deleteMinioService},
		{"minio-deployment", m.deleteMinioDeployment},
		{"minio-pvc", m.deleteMinioPVC},
		{"namespace", m.deleteNamespace},
	}
	for _, s := range steps {
		if err := s.fn(ctx); err != nil {
			return fmt.Errorf("teardown %s: %w", s.name, err)
		}
	}
	return nil
}

func (m *Manager) Status(ctx context.Context) (string, error) {
	m.logger.Info("observability.Status")
	obj, err := m.client.Get(ctx, client.GVRMultiClusterObservability, "", MCOName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "NotInstalled", nil
		}
		return "", fmt.Errorf("getting MCO: %w", err)
	}
	status, _ := obj.Object["status"].(map[string]interface{})
	if status == nil {
		return "Pending", nil
	}
	conditions, _ := status["conditions"].([]interface{})
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] == "Ready" && cond["status"] == "True" {
			return "Ready", nil
		}
	}
	return "Progressing", nil
}

