package idp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

type IdPType string

const (
	IdPGitHub   IdPType = "github"
	IdPGoogle   IdPType = "google"
	IdPHTPasswd IdPType = "htpasswd"
	IdPLDAP     IdPType = "ldap"
	IdPOIDC     IdPType = "oidc"

	idpLabel = "acmlab.redhat.com/idp"
)

type IdPOpts struct {
	Name          string
	Cluster       string
	Type          IdPType
	ClientID      string
	ClientSecret  string
	Organizations []string
	IssuerURL     string
	Users         map[string]string
	LDAPURL       string
	BindDN        string
	BindPassword  string
	Insecure      bool
}

type IdPInfo struct {
	Name    string `json:"name"`
	Cluster string `json:"cluster"`
	Type    string `json:"type"`
	Status  string `json:"status"`
}

type Manager struct {
	client *client.Client
	cfg    config.Config
	logger *slog.Logger
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger}
}

func (m *Manager) Configure(ctx context.Context, opts IdPOpts) error {
	m.logger.Info("idp.Configure", "name", opts.Name, "cluster", opts.Cluster, "type", string(opts.Type))

	manifests := []interface{}{
		buildSecretManifest(opts),
		buildOAuthManifest(opts),
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "work.open-cluster-management.io/v1",
			"kind":       "ManifestWork",
			"metadata": map[string]interface{}{
				"name":      manifestWorkName(opts.Name),
				"namespace": opts.Cluster,
				"labels": map[string]interface{}{
					idpLabel:                opts.Name,
					"acmlab.redhat.com/type": string(opts.Type),
				},
			},
			"spec": map[string]interface{}{
				"workload": map[string]interface{}{
					"manifests": manifests,
				},
			},
		},
	}
	return m.client.CreateIfNotExists(ctx, client.GVRManifestWork, opts.Cluster, obj)
}

func (m *Manager) Remove(ctx context.Context, idpName, cluster string) error {
	m.logger.Info("idp.Remove", "name", idpName, "cluster", cluster)
	return m.client.DeleteIfExists(ctx, client.GVRManifestWork, cluster, manifestWorkName(idpName))
}

func (m *Manager) List(ctx context.Context, cluster string) ([]IdPInfo, error) {
	m.logger.Info("idp.List", "cluster", cluster)
	list, err := m.client.List(ctx, client.GVRManifestWork, cluster, idpLabel)
	if err != nil {
		return nil, fmt.Errorf("listing idp manifestworks: %w", err)
	}
	infos := make([]IdPInfo, 0, len(list.Items))
	for _, item := range list.Items {
		infos = append(infos, parseIdPInfo(item.Object))
	}
	return infos, nil
}

func (m *Manager) Rotate(ctx context.Context, idpName, cluster string, newOpts IdPOpts) error {
	m.logger.Info("idp.Rotate", "name", idpName, "cluster", cluster)

	newOpts.Name = idpName
	newOpts.Cluster = cluster

	existing, err := m.client.Get(ctx, client.GVRManifestWork, cluster, manifestWorkName(idpName))
	if err != nil {
		return fmt.Errorf("getting idp manifestwork %s: %w", idpName, err)
	}

	labels := existing.GetLabels()
	if labels != nil {
		if t, ok := labels["acmlab.redhat.com/type"]; ok && newOpts.Type == "" {
			newOpts.Type = IdPType(t)
		}
	}

	manifests := []interface{}{
		buildSecretManifest(newOpts),
		buildOAuthManifest(newOpts),
	}

	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"workload": map[string]interface{}{
				"manifests": manifests,
			},
		},
	}
	data, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshaling patch: %w", err)
	}
	_, err = m.client.Patch(ctx, client.GVRManifestWork, cluster, manifestWorkName(idpName), types.MergePatchType, data)
	if err != nil {
		return fmt.Errorf("patching idp manifestwork %s: %w", idpName, err)
	}
	return nil
}

func manifestWorkName(idpName string) string {
	return "idp-" + idpName
}

func parseIdPInfo(obj map[string]interface{}) IdPInfo {
	info := IdPInfo{}
	if meta, ok := obj["metadata"].(map[string]interface{}); ok {
		info.Cluster, _ = meta["namespace"].(string)
		if labels, ok := meta["labels"].(map[string]interface{}); ok {
			info.Name, _ = labels[idpLabel].(string)
			info.Type, _ = labels["acmlab.redhat.com/type"].(string)
		}
	}
	info.Status = "Pending"
	status, _ := obj["status"].(map[string]interface{})
	if status == nil {
		return info
	}
	conditions, _ := status["conditions"].([]interface{})
	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] == "Applied" && cond["status"] == "True" {
			info.Status = "Applied"
		}
	}
	return info
}
