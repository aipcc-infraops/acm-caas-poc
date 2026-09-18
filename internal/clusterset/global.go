package clusterset

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

const globalSetName = "global"

type GlobalBinding struct {
	Namespace string `json:"namespace"`
}

type GlobalStatus struct {
	Enabled  bool            `json:"enabled"`
	Bindings []GlobalBinding `json:"bindings"`
}

func (m *Manager) EnableGlobal(ctx context.Context) error {
	m.logger.Info("clusterset.EnableGlobal")

	cs := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta2",
			"kind":       "ManagedClusterSet",
			"metadata": map[string]interface{}{
				"name": globalSetName,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/global": "true",
				},
			},
			"spec": map[string]interface{}{
				"clusterSelector": map[string]interface{}{
					"selectorType": "LabelSelector",
					"labelSelector": map[string]interface{}{
						"matchLabels": map[string]interface{}{},
					},
				},
			},
		},
	}
	if err := m.client.CreateIfNotExists(ctx, client.GVRManagedClusterSet, "", cs); err != nil {
		return fmt.Errorf("creating global ManagedClusterSet: %w", err)
	}
	return nil
}

func (m *Manager) BindGlobal(ctx context.Context, namespace string) error {
	m.logger.Info("clusterset.BindGlobal", "namespace", namespace)

	_, err := m.client.Get(ctx, client.GVRManagedClusterSet, "", globalSetName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("global ManagedClusterSet not found - run enable-global first")
		}
		return fmt.Errorf("checking global set: %w", err)
	}

	binding := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1beta2",
			"kind":       "ManagedClusterSetBinding",
			"metadata": map[string]interface{}{
				"name":      globalSetName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"acmlab.redhat.com/global": "true",
				},
			},
			"spec": map[string]interface{}{
				"clusterSet": globalSetName,
			},
		},
	}
	if err := m.client.CreateIfNotExists(ctx, client.GVRManagedClusterSetBinding, namespace, binding); err != nil {
		return fmt.Errorf("creating global binding in %s: %w", namespace, err)
	}
	return nil
}

func (m *Manager) UnbindGlobal(ctx context.Context, namespace string) error {
	m.logger.Info("clusterset.UnbindGlobal", "namespace", namespace)

	if err := m.client.DeleteIfExists(ctx, client.GVRManagedClusterSetBinding, namespace, globalSetName); err != nil {
		return fmt.Errorf("removing global binding from %s: %w", namespace, err)
	}
	return nil
}

func (m *Manager) GlobalStatus(ctx context.Context) (*GlobalStatus, error) {
	m.logger.Info("clusterset.GlobalStatus")

	_, err := m.client.Get(ctx, client.GVRManagedClusterSet, "", globalSetName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return &GlobalStatus{Enabled: false}, nil
		}
		return nil, fmt.Errorf("checking global set: %w", err)
	}

	bindings, err := m.client.List(ctx, client.GVRManagedClusterSetBinding, "", "acmlab.redhat.com/global=true")
	if err != nil {
		return nil, fmt.Errorf("listing global bindings: %w", err)
	}

	result := &GlobalStatus{Enabled: true, Bindings: make([]GlobalBinding, 0, len(bindings.Items))}
	for _, b := range bindings.Items {
		result.Bindings = append(result.Bindings, GlobalBinding{Namespace: b.GetNamespace()})
	}
	return result, nil
}
