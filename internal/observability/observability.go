package observability

import (
	"context"
	"fmt"
	"log/slog"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

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

	PullSecretName      = "multiclusterhub-operator-pull-secret"
	PullSecretSourceNS  = "openshift-config"
	MetricsAllowlistCM  = "observability-metrics-custom-allowlist"
	CustomRulesCM       = "thanos-rule-custom-rules"
	DashboardLabelKey   = "grafana-custom-dashboard"
	DashboardLabelValue = "true"
	OBCName             = "observability-obc"
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

type StorageOpts struct {
	Type         string
	BucketName   string
	StorageClass string
	Endpoint     string
}

type CustomRuleOpts struct {
	Name  string
	Rules string
}

type DashboardOpts struct {
	Name string
	JSON string
}

type MetricsOpts struct {
	Metrics []string
}

type RetentionOpts struct {
	RetentionInLocal string
	BlockDuration    string
	DeleteDelay      string
}

type AddonHealth struct {
	Cluster   string `json:"cluster"`
	Available bool   `json:"available"`
	Degraded  bool   `json:"degraded"`
}

func (m *Manager) ConfigurePullSecret(ctx context.Context) error {
	m.logger.Info("observability.ConfigurePullSecret")
	src, err := m.client.Get(ctx, client.GVRSecret, PullSecretSourceNS, "pull-secret")
	if err != nil {
		return fmt.Errorf("getting source pull secret: %w", err)
	}
	dst := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      PullSecretName,
				"namespace": Namespace,
			},
			"type": src.Object["type"],
			"data": src.Object["data"],
		},
	}
	return m.client.CreateIfNotExists(ctx, client.GVRSecret, Namespace, dst)
}

func (m *Manager) ConfigureOBCStorage(ctx context.Context, opts StorageOpts) error {
	m.logger.Info("observability.ConfigureOBCStorage")
	if opts.BucketName == "" {
		opts.BucketName = MinioBucket
	}
	if opts.StorageClass == "" {
		opts.StorageClass = StorageClass
	}
	obc := buildOBC(OBCName, Namespace, opts.StorageClass, opts.BucketName)
	return m.client.CreateIfNotExists(ctx, client.GVRObjectBucketClaim, Namespace, obc)
}

func (m *Manager) DeployCustomRules(ctx context.Context, opts CustomRuleOpts) error {
	m.logger.Info("observability.DeployCustomRules")
	cm := buildCustomRulesConfigMap(Namespace, opts.Rules)
	return m.client.CreateIfNotExists(ctx, client.GVRConfigMap, Namespace, cm)
}

func (m *Manager) RemoveCustomRules(ctx context.Context) error {
	m.logger.Info("observability.RemoveCustomRules")
	return m.client.DeleteIfExists(ctx, client.GVRConfigMap, Namespace, CustomRulesCM)
}

func (m *Manager) DeployDashboard(ctx context.Context, opts DashboardOpts) error {
	m.logger.Info("observability.DeployDashboard", "name", opts.Name)
	cm := buildDashboardConfigMap(Namespace, opts.Name, opts.JSON)
	return m.client.CreateIfNotExists(ctx, client.GVRConfigMap, Namespace, cm)
}

func (m *Manager) RemoveDashboard(ctx context.Context, name string) error {
	m.logger.Info("observability.RemoveDashboard", "name", name)
	return m.client.DeleteIfExists(ctx, client.GVRConfigMap, Namespace, name)
}

func (m *Manager) ConfigureMetricsAllowlist(ctx context.Context, opts MetricsOpts) error {
	m.logger.Info("observability.ConfigureMetricsAllowlist")
	cm := buildMetricsAllowlistConfigMap(Namespace, opts.Metrics)
	return m.client.CreateIfNotExists(ctx, client.GVRConfigMap, Namespace, cm)
}

func (m *Manager) ListAddonHealth(ctx context.Context) ([]AddonHealth, error) {
	m.logger.Info("observability.ListAddonHealth")
	list, err := m.client.Dynamic.Resource(client.GVRManagedClusterAddOn).
		Namespace("").
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing addons: %w", err)
	}
	var result []AddonHealth
	for _, item := range list.Items {
		if item.GetName() != "observability-controller" {
			continue
		}
		h := AddonHealth{Cluster: item.GetNamespace()}
		conditions, _ := item.Object["status"].(map[string]interface{})["conditions"].([]interface{})
		for _, c := range conditions {
			cond, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			switch cond["type"] {
			case "Available":
				h.Available = cond["status"] == "True"
			case "Degraded":
				h.Degraded = cond["status"] == "True"
			}
		}
		result = append(result, h)
	}
	return result, nil
}

func (m *Manager) ConfigureRetention(ctx context.Context, opts RetentionOpts) error {
	m.logger.Info("observability.ConfigureRetention")
	obj, err := m.client.Get(ctx, client.GVRMultiClusterObservability, "", MCOName)
	if err != nil {
		return fmt.Errorf("getting MCO: %w", err)
	}
	retention := map[string]interface{}{}
	if opts.RetentionInLocal != "" {
		retention["retentionInLocal"] = opts.RetentionInLocal
	}
	if opts.BlockDuration != "" {
		retention["blockDuration"] = opts.BlockDuration
	}
	if opts.DeleteDelay != "" {
		retention["deleteDelay"] = opts.DeleteDelay
	}
	spec, _ := obj.Object["spec"].(map[string]interface{})
	if spec == nil {
		spec = map[string]interface{}{}
		obj.Object["spec"] = spec
	}
	spec["retentionConfig"] = retention
	_, err = m.client.Update(ctx, client.GVRMultiClusterObservability, "", obj)
	return err
}

func (m *Manager) ConfigureMetricsAllowlistFromYAML(ctx context.Context, metricsYAML string) error {
	m.logger.Info("observability.ConfigureMetricsAllowlistFromYAML")
	cm := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      MetricsAllowlistCM,
				"namespace": Namespace,
			},
			"data": map[string]interface{}{
				"metrics_list.yaml": metricsYAML,
			},
		},
	}
	return m.client.CreateIfNotExists(ctx, client.GVRConfigMap, Namespace, cm)
}


