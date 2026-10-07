package gitops

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

const DefaultNamespace = "openshift-gitops"

type AppSetOpts struct {
	Name          string
	Namespace     string
	RepoURL       string
	Path          string
	Revision      string
	Generator     string
	LabelSelector map[string]string
	Project       string
}

type AppSetInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	RepoURL   string `json:"repoURL"`
	Path      string `json:"path"`
	Generator string `json:"generator"`
	Status    string `json:"status"`
	AppCount  int    `json:"appCount"`
}

type Manager struct {
	client *client.Client
	cfg    config.Config
	logger *slog.Logger
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger}
}

type DiagnoseCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type DiagnoseResult struct {
	Namespace string          `json:"namespace"`
	Healthy   bool            `json:"healthy"`
	Checks    []DiagnoseCheck `json:"checks"`
}

func (r *DiagnoseResult) addCheck(name, status, message string) {
	r.Checks = append(r.Checks, DiagnoseCheck{Name: name, Status: status, Message: message})
	if status == "fail" {
		r.Healthy = false
	}
}

func (m *Manager) Diagnose(ctx context.Context, namespace string) (*DiagnoseResult, error) {
	if namespace == "" {
		namespace = DefaultNamespace
	}
	result := &DiagnoseResult{Namespace: namespace, Healthy: true}

	_, err := m.client.Get(ctx, client.GVRCustomResourceDefinition, "", "applicationsets.argoproj.io")
	if err != nil {
		result.addCheck("applicationset-crd", "fail",
			"ApplicationSet CRD (applicationsets.argoproj.io) is not installed. Install OpenShift GitOps or Argo CD.")
	} else {
		result.addCheck("applicationset-crd", "pass", "ApplicationSet CRD is available.")
	}

	_, err = m.client.Get(ctx, client.GVRNamespace, "", namespace)
	if err != nil {
		result.addCheck("namespace", "fail",
			fmt.Sprintf("Namespace %q does not exist. Create it or choose a namespace where Argo CD is installed.", namespace))
	} else {
		result.addCheck("namespace", "pass", fmt.Sprintf("Namespace %q exists.", namespace))
	}

	m.checkGitOpsCSV(ctx, namespace, result)

	return result, nil
}

func (m *Manager) checkGitOpsCSV(ctx context.Context, namespace string, result *DiagnoseResult) {
	csvList, err := m.client.List(ctx, client.GVRClusterServiceVersion, namespace, "")
	if err != nil {
		result.addCheck("gitops-operator", "warn",
			"Could not query ClusterServiceVersions. GitOps may still work with a standalone Argo CD installation.")
		return
	}
	for _, csv := range csvList.Items {
		name := csv.GetName()
		if strings.Contains(name, "openshift-gitops") || strings.Contains(name, "gitops-operator") {
			phase, _, _ := unstructuredNestedString(csv.Object, "status", "phase")
			if phase == "Succeeded" {
				result.addCheck("gitops-operator", "pass",
					fmt.Sprintf("OpenShift GitOps operator %q is installed and healthy.", name))
				return
			}
			result.addCheck("gitops-operator", "warn",
				fmt.Sprintf("OpenShift GitOps operator %q found but phase is %q.", name, phase))
			return
		}
	}
	result.addCheck("gitops-operator", "warn",
		"No OpenShift GitOps operator CSV found. GitOps may still work with a standalone Argo CD installation.")
}

func unstructuredNestedString(obj map[string]interface{}, fields ...string) (string, bool, error) {
	current := obj
	for i, field := range fields {
		if i == len(fields)-1 {
			val, ok := current[field].(string)
			return val, ok, nil
		}
		next, ok := current[field].(map[string]interface{})
		if !ok {
			return "", false, nil
		}
		current = next
	}
	return "", false, nil
}

func (m *Manager) CheckPrerequisites(ctx context.Context, namespace string) error {
	if namespace == "" {
		namespace = DefaultNamespace
	}
	_, err := m.client.Get(ctx, client.GVRCustomResourceDefinition, "", "applicationsets.argoproj.io")
	if err != nil {
		return fmt.Errorf("GitOps prerequisites not met: ApplicationSet CRD is not installed. Run 'acmlab gitops diagnose --namespace %s' for details", namespace)
	}
	_, err = m.client.Get(ctx, client.GVRNamespace, "", namespace)
	if err != nil {
		return fmt.Errorf("GitOps prerequisites not met: namespace %q does not exist. Run 'acmlab gitops diagnose --namespace %s' for details", namespace, namespace)
	}
	return nil
}

func isResourceNotFound(err error) bool {
	if apierrors.IsNotFound(err) {
		return true
	}
	return strings.Contains(err.Error(), "the server could not find the requested resource")
}

func (m *Manager) Create(ctx context.Context, opts AppSetOpts) error {
	m.logger.Info("gitops.Create", "name", opts.Name)
	applyDefaults(&opts)

	if err := m.CheckPrerequisites(ctx, opts.Namespace); err != nil {
		return err
	}

	appSet := buildApplicationSet(opts)
	if err := m.client.CreateIfNotExists(ctx, client.GVRApplicationSet, opts.Namespace, appSet); err != nil {
		return fmt.Errorf("creating ApplicationSet: %w", err)
	}
	return nil
}

func (m *Manager) Get(ctx context.Context, name, namespace string) (*AppSetInfo, error) {
	m.logger.Info("gitops.Get", "name", name, "namespace", namespace)
	if namespace == "" {
		namespace = DefaultNamespace
	}

	if err := m.CheckPrerequisites(ctx, namespace); err != nil {
		return nil, err
	}

	obj, err := m.client.Get(ctx, client.GVRApplicationSet, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("getting ApplicationSet %s: %w", name, err)
	}
	return parseAppSetInfo(obj.Object), nil
}

func (m *Manager) List(ctx context.Context, namespace string) ([]AppSetInfo, error) {
	m.logger.Info("gitops.List", "namespace", namespace)
	if namespace == "" {
		namespace = DefaultNamespace
	}

	if err := m.CheckPrerequisites(ctx, namespace); err != nil {
		return nil, err
	}

	list, err := m.client.List(ctx, client.GVRApplicationSet, namespace, "acmlab.redhat.com/gitops")
	if err != nil {
		return nil, fmt.Errorf("listing ApplicationSets: %w", err)
	}
	infos := make([]AppSetInfo, 0, len(list.Items))
	for _, item := range list.Items {
		infos = append(infos, *parseAppSetInfo(item.Object))
	}
	return infos, nil
}

func (m *Manager) Delete(ctx context.Context, name, namespace string) (bool, error) {
	m.logger.Info("gitops.Delete", "name", name, "namespace", namespace)
	if namespace == "" {
		namespace = DefaultNamespace
	}

	_, err := m.client.Get(ctx, client.GVRApplicationSet, namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) || isResourceNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking ApplicationSet %s: %w", name, err)
	}

	if err := m.client.DeleteIfExists(ctx, client.GVRApplicationSet, namespace, name); err != nil {
		return false, fmt.Errorf("deleting ApplicationSet %s: %w", name, err)
	}
	return true, nil
}

func (m *Manager) Sync(ctx context.Context, name, namespace string) error {
	m.logger.Info("gitops.Sync", "name", name, "namespace", namespace)
	if namespace == "" {
		namespace = DefaultNamespace
	}

	if err := m.CheckPrerequisites(ctx, namespace); err != nil {
		return err
	}

	_, err := m.client.Get(ctx, client.GVRApplicationSet, namespace, name)
	if err != nil {
		return fmt.Errorf("getting ApplicationSet %s for sync: %w", name, err)
	}

	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": map[string]interface{}{
				"argocd.argoproj.io/refresh": time.Now().UTC().Format(time.RFC3339),
			},
		},
	}
	patchData, _ := json.Marshal(patch)
	if _, err := m.client.Patch(ctx, client.GVRApplicationSet, namespace, name, types.MergePatchType, patchData); err != nil {
		return fmt.Errorf("patching ApplicationSet %s for sync: %w", name, err)
	}
	return nil
}

type AgentModeOpts struct {
	Name      string
	Namespace string
	RepoURL   string
	Path      string
	Revision  string
	Clusters  []string
}

type AgentModeInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	RepoURL   string `json:"repoURL"`
	Path      string `json:"path"`
	Mode      string `json:"mode"`
	Status    string `json:"status"`
}

func (m *Manager) EnableAgentMode(ctx context.Context, opts AgentModeOpts) error {
	m.logger.Info("gitops.EnableAgentMode", "name", opts.Name)
	if opts.Namespace == "" {
		opts.Namespace = DefaultNamespace
	}
	if opts.Revision == "" {
		opts.Revision = "main"
	}

	if err := m.CheckPrerequisites(ctx, opts.Namespace); err != nil {
		return err
	}

	appSet := buildAgentModeApplicationSet(opts)
	if err := m.client.CreateIfNotExists(ctx, client.GVRApplicationSet, opts.Namespace, appSet); err != nil {
		return fmt.Errorf("creating agent-mode ApplicationSet: %w", err)
	}
	return nil
}

func (m *Manager) DisableAgentMode(ctx context.Context, name, namespace string) (bool, error) {
	m.logger.Info("gitops.DisableAgentMode", "name", name)
	if namespace == "" {
		namespace = DefaultNamespace
	}

	obj, err := m.client.Get(ctx, client.GVRApplicationSet, namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) || isResourceNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking agent-mode ApplicationSet %s: %w", name, err)
	}

	labels := obj.GetLabels()
	if labels["acmlab.redhat.com/agent-mode"] != "true" {
		return false, fmt.Errorf("%s is not an agent-mode ApplicationSet", name)
	}

	if err := m.client.DeleteIfExists(ctx, client.GVRApplicationSet, namespace, name); err != nil {
		return false, fmt.Errorf("deleting agent-mode ApplicationSet %s: %w", name, err)
	}
	return true, nil
}

func (m *Manager) AgentModeStatus(ctx context.Context, name, namespace string) (*AgentModeInfo, error) {
	m.logger.Info("gitops.AgentModeStatus", "name", name)
	if namespace == "" {
		namespace = DefaultNamespace
	}

	if err := m.CheckPrerequisites(ctx, namespace); err != nil {
		return nil, err
	}

	obj, err := m.client.Get(ctx, client.GVRApplicationSet, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("getting agent-mode ApplicationSet %s: %w", name, err)
	}
	return parseAgentModeInfo(obj.Object), nil
}

func parseAgentModeInfo(obj map[string]interface{}) *AgentModeInfo {
	info := &AgentModeInfo{Status: "Pending", Mode: "pull"}

	meta, _ := obj["metadata"].(map[string]interface{})
	if meta != nil {
		info.Name, _ = meta["name"].(string)
		info.Namespace, _ = meta["namespace"].(string)
	}

	spec, _ := obj["spec"].(map[string]interface{})
	if spec != nil {
		tmpl, _ := spec["template"].(map[string]interface{})
		if tmpl != nil {
			tmplSpec, _ := tmpl["spec"].(map[string]interface{})
			if tmplSpec != nil {
				source, _ := tmplSpec["source"].(map[string]interface{})
				if source != nil {
					info.RepoURL, _ = source["repoURL"].(string)
					info.Path, _ = source["path"].(string)
				}
			}
		}
	}

	status, _ := obj["status"].(map[string]interface{})
	if status != nil {
		conditions, _ := status["conditions"].([]interface{})
		for _, raw := range conditions {
			cond, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if cond["type"] == "ResourcesUpToDate" && cond["status"] == "True" {
				info.Status = "Synced"
			}
		}
	}

	return info
}

func applyDefaults(opts *AppSetOpts) {
	if opts.Namespace == "" {
		opts.Namespace = DefaultNamespace
	}
	if opts.Revision == "" {
		opts.Revision = "main"
	}
	if opts.Generator == "" {
		opts.Generator = "placement"
	}
	if opts.Project == "" {
		opts.Project = "default"
	}
}

func parseAppSetInfo(obj map[string]interface{}) *AppSetInfo {
	info := &AppSetInfo{Status: "Pending"}

	meta, _ := obj["metadata"].(map[string]interface{})
	if meta != nil {
		info.Name, _ = meta["name"].(string)
		info.Namespace, _ = meta["namespace"].(string)
		if labels, ok := meta["labels"].(map[string]interface{}); ok {
			info.Generator, _ = labels["acmlab.redhat.com/generator"].(string)
		}
	}

	spec, _ := obj["spec"].(map[string]interface{})
	if spec != nil {
		tmpl, _ := spec["template"].(map[string]interface{})
		if tmpl != nil {
			tmplSpec, _ := tmpl["spec"].(map[string]interface{})
			if tmplSpec != nil {
				source, _ := tmplSpec["source"].(map[string]interface{})
				if source != nil {
					info.RepoURL, _ = source["repoURL"].(string)
					info.Path, _ = source["path"].(string)
				}
			}
		}
	}

	status, _ := obj["status"].(map[string]interface{})
	if status != nil {
		conditions, _ := status["conditions"].([]interface{})
		for _, raw := range conditions {
			cond, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if cond["type"] == "ResourcesUpToDate" && cond["status"] == "True" {
				info.Status = "Synced"
			}
		}
		if resources, ok := status["resources"].([]interface{}); ok {
			info.AppCount = len(resources)
		}
	}

	return info
}
