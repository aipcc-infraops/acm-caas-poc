package provisioning

import (
	"context"
	"encoding/json"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type TemplatePatch struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value string `json:"value"`
}

type TemplateInfo struct {
	Name    string `json:"name"`
	Patches int    `json:"patches"`
}

func (m *Manager) CreateTemplate(ctx context.Context, name string, patches []TemplatePatch) error {
	m.logger.Info("provisioning.CreateTemplate", "name", name)

	tmpl := buildClusterDeploymentCustomization(name, patches)
	if err := m.client.CreateIfNotExists(ctx, client.GVRClusterDeploymentCustomization, "", tmpl); err != nil {
		return fmt.Errorf("creating cluster template %s: %w", name, err)
	}
	return nil
}

func (m *Manager) GetTemplate(ctx context.Context, name string) (map[string]interface{}, error) {
	m.logger.Info("provisioning.GetTemplate", "name", name)

	obj, err := m.client.Get(ctx, client.GVRClusterDeploymentCustomization, "", name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("template %s not found", name)
		}
		return nil, fmt.Errorf("getting template %s: %w", name, err)
	}
	return obj.Object, nil
}

func (m *Manager) ListTemplates(ctx context.Context) ([]TemplateInfo, error) {
	m.logger.Info("provisioning.ListTemplates")

	list, err := m.client.List(ctx, client.GVRClusterDeploymentCustomization, "", "")
	if err != nil {
		return nil, fmt.Errorf("listing templates: %w", err)
	}

	result := make([]TemplateInfo, 0, len(list.Items))
	for _, item := range list.Items {
		patchCount := 0
		spec, _ := item.Object["spec"].(map[string]interface{})
		if spec != nil {
			if patches, ok := spec["installConfigPatches"].([]interface{}); ok {
				patchCount = len(patches)
			}
		}
		result = append(result, TemplateInfo{
			Name:    item.GetName(),
			Patches: patchCount,
		})
	}
	return result, nil
}

func (m *Manager) RemoveTemplate(ctx context.Context, name string) error {
	m.logger.Info("provisioning.RemoveTemplate", "name", name)

	if err := m.client.DeleteIfExists(ctx, client.GVRClusterDeploymentCustomization, "", name); err != nil {
		return fmt.Errorf("deleting template %s: %w", name, err)
	}
	return nil
}

func (m *Manager) ApplyTemplate(ctx context.Context, clusterDeployment, templateName string) error {
	m.logger.Info("provisioning.ApplyTemplate", "cluster", clusterDeployment, "template", templateName)

	_, err := m.client.Get(ctx, client.GVRClusterDeploymentCustomization, "", templateName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("template %s not found", templateName)
		}
		return fmt.Errorf("checking template %s: %w", templateName, err)
	}

	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": map[string]interface{}{
				"hive.openshift.io/cluster-deployment-customization": templateName,
			},
		},
	}
	data, _ := json.Marshal(patch)
	ns := clusterDeployment
	if _, err := m.client.Patch(ctx, client.GVRClusterDeployment, ns, clusterDeployment, types.MergePatchType, data); err != nil {
		return fmt.Errorf("applying template %s to %s: %w", templateName, clusterDeployment, err)
	}
	return nil
}
