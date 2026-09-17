package reclamation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
	"github.com/pablofelix/acm-caas-poc/internal/lifecycle"
)

type ClusterTTL struct {
	Name       string    `json:"name"`
	TTLHours   int       `json:"ttlHours"`
	ExpiryDate time.Time `json:"expiryDate"`
	Expired    bool      `json:"expired"`
	Owner      string    `json:"owner,omitempty"`
}

type ReclaimResult struct {
	Cluster string `json:"cluster"`
	Action  string `json:"action"`
	Error   string `json:"error,omitempty"`
}

type Manager struct {
	client    *client.Client
	cfg       config.Config
	logger    *slog.Logger
	lifecycle *lifecycle.Manager
	now       func() time.Time
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{
		client:    c,
		cfg:       cfg,
		logger:    logger,
		lifecycle: lifecycle.New(c, cfg, logger),
		now:       time.Now,
	}
}

func (m *Manager) SetTTL(ctx context.Context, cluster string, ttlHours int) error {
	m.logger.Info("reclamation.SetTTL", "cluster", cluster, "ttlHours", ttlHours)
	if ttlHours <= 0 {
		return fmt.Errorf("TTL must be positive, got %d", ttlHours)
	}

	expiry := m.now().Add(time.Duration(ttlHours) * time.Hour)
	patch := buildTTLLabelPatch(ttlHours, expiry)
	data, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshaling TTL patch: %w", err)
	}

	_, err = m.client.Patch(ctx, client.GVRManagedCluster, "", cluster, types.MergePatchType, data)
	if err != nil {
		return fmt.Errorf("patching ManagedCluster %s TTL labels: %w", cluster, err)
	}
	return nil
}

func (m *Manager) CheckExpired(ctx context.Context) ([]ClusterTTL, error) {
	m.logger.Info("reclamation.CheckExpired")
	all, err := m.ListTTLs(ctx)
	if err != nil {
		return nil, err
	}
	return filterExpired(all), nil
}

func (m *Manager) ListTTLs(ctx context.Context) ([]ClusterTTL, error) {
	m.logger.Info("reclamation.ListTTLs")
	list, err := m.client.List(ctx, client.GVRManagedCluster, "", "")
	if err != nil {
		return nil, fmt.Errorf("listing ManagedClusters: %w", err)
	}

	now := m.now()
	var ttls []ClusterTTL
	for _, item := range list.Items {
		ttl := parseClusterTTL(item.Object, now)
		if ttl == nil {
			continue
		}
		ttls = append(ttls, *ttl)
	}
	return ttls, nil
}

func (m *Manager) ExtendTTL(ctx context.Context, cluster string, hours int, justification string) error {
	m.logger.Info("reclamation.ExtendTTL", "cluster", cluster, "hours", hours)
	if hours <= 0 {
		return fmt.Errorf("extension hours must be positive, got %d", hours)
	}
	if justification == "" {
		return fmt.Errorf("justification is required for TTL extension")
	}

	mc, err := m.client.Get(ctx, client.GVRManagedCluster, "", cluster)
	if err != nil {
		return fmt.Errorf("getting ManagedCluster %s: %w", cluster, err)
	}

	currentExpiry := parseExpiryFromLabels(mc.GetLabels())
	if currentExpiry.IsZero() {
		return fmt.Errorf("cluster %s has no TTL set", cluster)
	}

	newExpiry := currentExpiry.Add(time.Duration(hours) * time.Hour)
	patch := buildExtendPatch(newExpiry, justification)
	data, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshaling extend patch: %w", err)
	}

	_, err = m.client.Patch(ctx, client.GVRManagedCluster, "", cluster, types.MergePatchType, data)
	if err != nil {
		return fmt.Errorf("patching ManagedCluster %s extension: %w", cluster, err)
	}
	return nil
}

func (m *Manager) ReclaimCluster(ctx context.Context, cluster string) (*ReclaimResult, error) {
	m.logger.Info("reclamation.ReclaimCluster", "cluster", cluster)
	result := &ReclaimResult{Cluster: cluster}

	_, err := m.client.Get(ctx, client.GVRClusterDeployment, cluster, cluster)
	if err == nil {
		if hibErr := m.lifecycle.Hibernate(ctx, cluster, cluster); hibErr != nil {
			result.Action = "hibernate-failed"
			result.Error = hibErr.Error()
			return result, fmt.Errorf("hibernating cluster %s: %w", cluster, hibErr)
		}
		result.Action = "hibernated"
		return result, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("checking ClusterDeployment %s: %w", cluster, err)
	}

	if delErr := m.client.Delete(ctx, client.GVRManagedCluster, "", cluster); delErr != nil {
		result.Action = "detach-failed"
		result.Error = delErr.Error()
		return result, fmt.Errorf("detaching cluster %s: %w", cluster, delErr)
	}
	result.Action = "detached"
	return result, nil
}

func parseClusterTTL(obj map[string]interface{}, now time.Time) *ClusterTTL {
	meta, ok := obj["metadata"].(map[string]interface{})
	if !ok {
		return nil
	}
	labels, _ := meta["labels"].(map[string]interface{})
	ttlStr, hasTTL := labels["caas/ttl-hours"].(string)
	if !hasTTL {
		return nil
	}

	ttlHours, err := strconv.Atoi(ttlStr)
	if err != nil {
		return nil
	}

	name, _ := meta["name"].(string)
	owner, _ := labels["caas/owner"].(string)

	ttl := &ClusterTTL{
		Name:     name,
		TTLHours: ttlHours,
		Owner:    owner,
	}

	expiryStr, hasExpiry := labels["caas/expiry-date"].(string)
	if hasExpiry {
		expiry, parseErr := time.Parse(time.RFC3339, expiryStr)
		if parseErr == nil {
			ttl.ExpiryDate = expiry
			ttl.Expired = now.After(expiry)
		}
	}

	return ttl
}

func parseExpiryFromLabels(labels map[string]string) time.Time {
	expiryStr, ok := labels["caas/expiry-date"]
	if !ok {
		return time.Time{}
	}
	expiry, err := time.Parse(time.RFC3339, expiryStr)
	if err != nil {
		return time.Time{}
	}
	return expiry
}

func filterExpired(ttls []ClusterTTL) []ClusterTTL {
	var expired []ClusterTTL
	for _, ttl := range ttls {
		if ttl.Expired {
			expired = append(expired, ttl)
		}
	}
	return expired
}

func buildTTLLabelPatch(ttlHours int, expiry time.Time) map[string]interface{} {
	return map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				"caas/ttl-hours":   strconv.Itoa(ttlHours),
				"caas/expiry-date": expiry.UTC().Format(time.RFC3339),
			},
		},
	}
}

func buildExtendPatch(newExpiry time.Time, justification string) map[string]interface{} {
	return map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]interface{}{
				"caas/expiry-date": newExpiry.UTC().Format(time.RFC3339),
			},
			"annotations": map[string]interface{}{
				"caas/extend-justification": justification,
			},
		},
	}
}
