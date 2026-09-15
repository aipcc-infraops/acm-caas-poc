package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

type Manager struct {
	client *client.Client
	cfg    config.Config
	logger *slog.Logger
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger}
}

type MirrorConfig struct {
	ClusterName    string
	MirrorRegistry string
	PullSecretPath string
	Registries     []RegistryMapping
}

type RegistryMapping struct {
	Source string `json:"source"`
	Mirror string `json:"mirror"`
}

type RequiredImage struct {
	Image        string `json:"image"`
	ManifestWork string `json:"manifestWork"`
}

type MirrorStatus struct {
	ClusterName string            `json:"clusterName"`
	Configured  bool              `json:"configured"`
	Registries  []RegistryMapping `json:"registries,omitempty"`
}

// ListRequiredImages extracts all container images from the ManifestWorks
// that ACM created for a cluster. These are the images the spoke needs
// to pull — if the spoke can't reach the source registry, they must be
// mirrored.
func (m *Manager) ListRequiredImages(ctx context.Context, clusterName string) ([]RequiredImage, error) {
	m.logger.Info("registry.ListRequiredImages", "cluster", clusterName)
	mwList, err := m.client.List(ctx, client.GVRManifestWork, clusterName, "")
	if err != nil {
		return nil, fmt.Errorf("listing ManifestWorks in %s: %w", clusterName, err)
	}

	var images []RequiredImage
	seen := make(map[string]bool)

	for _, mw := range mwList.Items {
		mwName := mw.GetName()
		manifests, _, _ := unstructured.NestedSlice(mw.Object, "spec", "workload", "manifests")
		for _, raw := range manifests {
			obj, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			for _, img := range extractImagesFromObject(obj) {
				if !seen[img] {
					seen[img] = true
					images = append(images, RequiredImage{
						Image:        img,
						ManifestWork: mwName,
					})
				}
			}
		}
	}

	return images, nil
}

// ConfigureMirror creates a Placement, pull secret, and ManagedClusterImageRegistry
// on the hub so that ACM rewrites image references in klusterlet manifests
// before applying them to the spoke.
func (m *Manager) ConfigureMirror(ctx context.Context, opts MirrorConfig) error {
	m.logger.Info("registry.ConfigureMirror", "cluster", opts.ClusterName)
	ns := opts.ClusterName
	placementName := fmt.Sprintf("%s-registry-placement", opts.ClusterName)

	if err := m.ensureClusterSetBinding(ctx, ns); err != nil {
		return fmt.Errorf("creating ManagedClusterSetBinding: %w", err)
	}

	if err := m.createPlacement(ctx, ns, placementName, opts.ClusterName); err != nil {
		return fmt.Errorf("creating Placement: %w", err)
	}

	pullSecretName := fmt.Sprintf("%s-registry-pull-secret", opts.ClusterName)
	if opts.PullSecretPath != "" {
		if err := m.createPullSecret(ctx, ns, pullSecretName, opts.PullSecretPath); err != nil {
			return fmt.Errorf("creating pull secret: %w", err)
		}
	}

	registries := opts.Registries
	if len(registries) == 0 && opts.MirrorRegistry != "" {
		registries = DefaultRegistryMappings(opts.MirrorRegistry)
	}

	if err := m.createImageRegistry(ctx, ns, opts.ClusterName, placementName, pullSecretName, registries); err != nil {
		return fmt.Errorf("creating ManagedClusterImageRegistry: %w", err)
	}

	return nil
}

// RemoveMirror deletes the ManagedClusterImageRegistry, Placement, and pull
// secret created by ConfigureMirror.
func (m *Manager) RemoveMirror(ctx context.Context, clusterName string) error {
	m.logger.Info("registry.RemoveMirror", "cluster", clusterName)
	ns := clusterName

	steps := []struct {
		label string
		gvr   schema.GroupVersionResource
		name  string
	}{
		{"ManagedClusterImageRegistry", client.GVRManagedClusterImageRegistry, fmt.Sprintf("%s-image-registry", clusterName)},
		{"Placement", client.GVRPlacement, fmt.Sprintf("%s-registry-placement", clusterName)},
		{"pull secret", client.GVRSecret, fmt.Sprintf("%s-registry-pull-secret", clusterName)},
		{"ManagedClusterSetBinding", client.GVRManagedClusterSetBinding, "default"},
	}
	for _, s := range steps {
		if err := m.client.DeleteIfExists(ctx, s.gvr, ns, s.name); err != nil {
			return fmt.Errorf("deleting %s: %w", s.label, err)
		}
	}
	return nil
}

// GetMirrorStatus checks whether a ManagedClusterImageRegistry is configured
// for a cluster.
func (m *Manager) GetMirrorStatus(ctx context.Context, clusterName string) (*MirrorStatus, error) {
	m.logger.Info("registry.GetMirrorStatus", "cluster", clusterName)
	name := fmt.Sprintf("%s-image-registry", clusterName)
	obj, err := m.client.Get(ctx, client.GVRManagedClusterImageRegistry, clusterName, name)
	if errors.IsNotFound(err) {
		return &MirrorStatus{ClusterName: clusterName, Configured: false}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting ManagedClusterImageRegistry: %w", err)
	}

	var registries []RegistryMapping
	regs, _, _ := unstructured.NestedSlice(obj.Object, "spec", "registries")
	for _, raw := range regs {
		reg, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		source, _ := reg["source"].(string)
		mirror, _ := reg["mirror"].(string)
		registries = append(registries, RegistryMapping{Source: source, Mirror: mirror})
	}

	return &MirrorStatus{
		ClusterName: clusterName,
		Configured:  true,
		Registries:  registries,
	}, nil
}

// GenerateMirrorScript produces a bash script with skopeo commands to copy
// images from the source registry to the mirror. The user runs this script
// before calling ConfigureMirror.
func GenerateMirrorScript(images []RequiredImage, targetRegistry string) string {
	var sb strings.Builder
	sb.WriteString("#!/bin/bash\n")
	sb.WriteString("# Mirror ACM/MCE images for restricted-registry clusters\n")
	sb.WriteString("# Run this before configuring the image registry in ACM\n")
	sb.WriteString("# Requires: skopeo, credentials for both source and target registries\n\n")
	sb.WriteString("set -euo pipefail\n\n")
	sb.WriteString(fmt.Sprintf("TARGET_REGISTRY=%q\n\n", targetRegistry))

	for _, img := range images {
		source := img.Image
		parts := strings.SplitN(source, "/", 2)
		if len(parts) != 2 {
			continue
		}
		target := fmt.Sprintf("${TARGET_REGISTRY}/%s", parts[1])
		sb.WriteString(fmt.Sprintf("echo \"Mirroring %s\"\n", source))
		sb.WriteString(fmt.Sprintf("skopeo copy --all docker://%s docker://%s\n\n", source, target))
	}

	sb.WriteString("echo \"Done. All images mirrored to ${TARGET_REGISTRY}\"\n")
	return sb.String()
}

// DefaultRegistryMappings returns the standard source→mirror mappings for
// ACM/MCE images. The mirror registry replaces registry.redhat.io paths.
func DefaultRegistryMappings(mirrorRegistry string) []RegistryMapping {
	return []RegistryMapping{
		{Source: "registry.redhat.io/multicluster-engine", Mirror: mirrorRegistry + "/multicluster-engine"},
		{Source: "registry.redhat.io/rhacm2", Mirror: mirrorRegistry + "/rhacm2"},
	}
}

// ensureClusterSetBinding creates a ManagedClusterSetBinding in the cluster
// namespace so that Placements in that namespace can select from the default
// ClusterSet. Without this binding, the Placement finds no clusters.
func (m *Manager) ensureClusterSetBinding(ctx context.Context, namespace string) error {
	binding := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta2",
			"kind":       "ManagedClusterSetBinding",
			"metadata": map[string]interface{}{
				"name":      "default",
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"clusterSet": "default",
			},
		},
	}
	return m.client.CreateIfNotExists(ctx, client.GVRManagedClusterSetBinding, namespace, binding)
}

func (m *Manager) createPlacement(ctx context.Context, namespace, name, clusterName string) error {
	placement := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta1",
			"kind":       "Placement",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"predicates": []interface{}{
					map[string]interface{}{
						"requiredClusterSelector": map[string]interface{}{
							"labelSelector": map[string]interface{}{
								"matchLabels": map[string]interface{}{
									"name": clusterName,
								},
							},
						},
					},
				},
				"clusterSets": []interface{}{"default"},
				// Tolerate unavailable/unreachable clusters so the mirror can
				// be configured before import completes (chicken-and-egg fix).
				"tolerations": []interface{}{
					map[string]interface{}{
						"key":      "cluster.open-cluster-management.io/unreachable",
						"operator": "Exists",
					},
					map[string]interface{}{
						"key":      "cluster.open-cluster-management.io/unavailable",
						"operator": "Exists",
					},
				},
			},
		},
	}

	return m.client.CreateIfNotExists(ctx, client.GVRPlacement, namespace, placement)
}

func (m *Manager) createPullSecret(ctx context.Context, namespace, name, pullSecretPath string) error {
	data, err := os.ReadFile(pullSecretPath)
	if err != nil {
		return fmt.Errorf("reading pull secret %s: %w", pullSecretPath, err)
	}

	var pullSecretData map[string]interface{}
	if err := json.Unmarshal(data, &pullSecretData); err != nil {
		return fmt.Errorf("parsing pull secret JSON: %w", err)
	}

	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"type": "kubernetes.io/dockerconfigjson",
			"stringData": map[string]interface{}{
				".dockerconfigjson": string(data),
			},
		},
	}

	return m.client.CreateIfNotExists(ctx, client.GVRSecret, namespace, secret)
}

func (m *Manager) createImageRegistry(ctx context.Context, namespace, clusterName, placementName, pullSecretName string, registries []RegistryMapping) error {
	name := fmt.Sprintf("%s-image-registry", clusterName)

	regs := make([]interface{}, len(registries))
	for i, r := range registries {
		regs[i] = map[string]interface{}{
			"source": r.Source,
			"mirror": r.Mirror,
		}
	}

	mcir := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "imageregistry.open-cluster-management.io/v1alpha1",
			"kind":       "ManagedClusterImageRegistry",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"placementRef": map[string]interface{}{
					"group":    "cluster.open-cluster-management.io",
					"resource": "placements",
					"name":     placementName,
				},
				"pullSecret": map[string]interface{}{
					"name": pullSecretName,
				},
				"registries": regs,
			},
		},
	}

	return m.client.CreateIfNotExists(ctx, client.GVRManagedClusterImageRegistry, namespace, mcir)
}

func extractImagesFromObject(obj map[string]interface{}) []string {
	var images []string

	if img, ok := obj["image"].(string); ok && strings.Contains(img, "/") {
		images = append(images, img)
	}

	for _, v := range obj {
		switch val := v.(type) {
		case map[string]interface{}:
			images = append(images, extractImagesFromObject(val)...)
		case []interface{}:
			for _, item := range val {
				if m, ok := item.(map[string]interface{}); ok {
					images = append(images, extractImagesFromObject(m)...)
				}
			}
		}
	}

	return images
}
