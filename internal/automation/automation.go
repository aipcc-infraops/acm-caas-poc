package automation

import (
	"context"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/config"
)

const DefaultNamespace = "open-cluster-management-policies"

type AutomationOpts struct {
	Name        string
	Namespace   string
	PolicyName  string
	Mode        string
	TowerURL    string
	TowerSecret string
	JobTemplate string
	ExtraVars   map[string]string
}

type AutomationInfo struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	PolicyName string `json:"policyName"`
	Mode       string `json:"mode"`
	Status     string `json:"status"`
	LastRun    string `json:"lastRun,omitempty"`
}

type Manager struct {
	client *client.Client
	cfg    config.Config
	logger *slog.Logger
}

func New(c *client.Client, cfg config.Config, logger *slog.Logger) *Manager {
	return &Manager{client: c, cfg: cfg, logger: logger}
}

func (m *Manager) Create(ctx context.Context, opts AutomationOpts) error {
	m.logger.Info("automation.Create", "name", opts.Name, "policy", opts.PolicyName)
	if opts.Namespace == "" {
		opts.Namespace = DefaultNamespace
	}
	if opts.Mode == "" {
		opts.Mode = "scan"
	}

	pa := buildPolicyAutomation(opts)
	return m.client.CreateIfNotExists(ctx, client.GVRPolicyAutomation, opts.Namespace, pa)
}

func (m *Manager) Get(ctx context.Context, name, namespace string) (*AutomationInfo, error) {
	m.logger.Info("automation.Get", "name", name)
	if namespace == "" {
		namespace = DefaultNamespace
	}

	obj, err := m.client.Get(ctx, client.GVRPolicyAutomation, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("getting PolicyAutomation %s/%s: %w", namespace, name, err)
	}
	return parseAutomationInfo(obj), nil
}

func (m *Manager) List(ctx context.Context, namespace string) ([]AutomationInfo, error) {
	m.logger.Info("automation.List")
	if namespace == "" {
		namespace = DefaultNamespace
	}

	list, err := m.client.List(ctx, client.GVRPolicyAutomation, namespace, "acmlab.redhat.com/automation")
	if err != nil {
		return nil, fmt.Errorf("listing PolicyAutomations: %w", err)
	}

	infos := make([]AutomationInfo, 0, len(list.Items))
	for i := range list.Items {
		infos = append(infos, *parseAutomationInfo(&list.Items[i]))
	}
	return infos, nil
}

func (m *Manager) Delete(ctx context.Context, name, namespace string) (bool, error) {
	m.logger.Info("automation.Delete", "name", name)
	if namespace == "" {
		namespace = DefaultNamespace
	}

	_, err := m.client.Get(ctx, client.GVRPolicyAutomation, namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking PolicyAutomation %s: %w", name, err)
	}

	if err := m.client.DeleteIfExists(ctx, client.GVRPolicyAutomation, namespace, name); err != nil {
		return false, fmt.Errorf("deleting PolicyAutomation %s: %w", name, err)
	}
	return true, nil
}

func (m *Manager) UpdateMode(ctx context.Context, name, namespace, mode string) error {
	m.logger.Info("automation.UpdateMode", "name", name, "mode", mode)
	if namespace == "" {
		namespace = DefaultNamespace
	}

	obj, err := m.client.Get(ctx, client.GVRPolicyAutomation, namespace, name)
	if err != nil {
		return fmt.Errorf("getting PolicyAutomation %s: %w", name, err)
	}

	spec, _ := obj.Object["spec"].(map[string]interface{})
	if spec == nil {
		spec = map[string]interface{}{}
		obj.Object["spec"] = spec
	}
	spec["mode"] = mode

	_, err = m.client.Update(ctx, client.GVRPolicyAutomation, namespace, obj)
	if err != nil {
		return fmt.Errorf("updating PolicyAutomation mode: %w", err)
	}
	return nil
}

func parseAutomationInfo(obj *unstructured.Unstructured) *AutomationInfo {
	info := &AutomationInfo{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}

	spec, _ := obj.Object["spec"].(map[string]interface{})
	if spec != nil {
		info.PolicyName, _ = spec["policyRef"].(string)
		info.Mode, _ = spec["mode"].(string)
	}

	info.Status = "Pending"
	status, _ := obj.Object["status"].(map[string]interface{})
	if status != nil {
		conditions, _ := status["conditions"].([]interface{})
		for _, raw := range conditions {
			cond, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if cond["type"] == "Ready" && cond["status"] == "True" {
				info.Status = "Ready"
			}
		}
		if lr, ok := status["lastRun"].(string); ok {
			info.LastRun = lr
		}
	}

	return info
}
