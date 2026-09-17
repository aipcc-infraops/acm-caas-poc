package policy

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type PolicySetOpts struct {
	Name        string
	Namespace   string
	Description string
	Policies    []string
	ClusterSet  string
}

type PolicySetInfo struct {
	Name        string   `json:"name"`
	Namespace   string   `json:"namespace"`
	Description string   `json:"description"`
	Policies    []string `json:"policies"`
	Compliant   string   `json:"compliant"`
}

func (m *Manager) ApplyPolicySet(ctx context.Context, opts PolicySetOpts) error {
	m.logger.Info("policy.ApplyPolicySet", "name", opts.Name)
	ns := opts.Namespace
	if ns == "" {
		ns = DefaultNamespace
	}

	policySet := buildPolicySet(opts.Name, ns, opts.Description, opts.Policies)
	if err := m.client.CreateIfNotExists(ctx, client.GVRPolicySet, ns, policySet); err != nil {
		return fmt.Errorf("creating PolicySet: %w", err)
	}

	placement := buildPolicySetPlacement(opts.Name, ns, opts.ClusterSet)
	if err := m.client.CreateIfNotExists(ctx, client.GVRPlacement, ns, placement); err != nil {
		return fmt.Errorf("creating PolicySet placement: %w", err)
	}

	binding := buildPolicySetBinding(opts.Name, ns)
	if err := m.client.CreateIfNotExists(ctx, client.GVRPlacementBinding, ns, binding); err != nil {
		return fmt.Errorf("creating PolicySet placement binding: %w", err)
	}

	return nil
}

func (m *Manager) GetPolicySet(ctx context.Context, name, namespace string) (*PolicySetInfo, error) {
	m.logger.Info("policy.GetPolicySet", "name", name)
	if namespace == "" {
		namespace = DefaultNamespace
	}

	obj, err := m.client.Get(ctx, client.GVRPolicySet, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("getting PolicySet %s: %w", name, err)
	}

	return parsePolicySetInfo(obj.Object), nil
}

func (m *Manager) ListPolicySets(ctx context.Context, namespace string) ([]PolicySetInfo, error) {
	m.logger.Info("policy.ListPolicySets")
	if namespace == "" {
		namespace = DefaultNamespace
	}

	list, err := m.client.List(ctx, client.GVRPolicySet, namespace, "")
	if err != nil {
		return nil, fmt.Errorf("listing PolicySets: %w", err)
	}

	sets := make([]PolicySetInfo, 0, len(list.Items))
	for _, item := range list.Items {
		sets = append(sets, *parsePolicySetInfo(item.Object))
	}
	return sets, nil
}

func (m *Manager) RemovePolicySet(ctx context.Context, name, namespace string) (bool, error) {
	m.logger.Info("policy.RemovePolicySet", "name", name)
	if namespace == "" {
		namespace = DefaultNamespace
	}

	_, err := m.client.Get(ctx, client.GVRPolicySet, namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking PolicySet %s: %w", name, err)
	}

	if err := m.client.DeleteIfExists(ctx, client.GVRPlacementBinding, namespace, name+"-policyset-binding"); err != nil {
		return false, fmt.Errorf("removing PolicySet binding: %w", err)
	}
	if err := m.client.DeleteIfExists(ctx, client.GVRPlacement, namespace, name+"-policyset-placement"); err != nil {
		return false, fmt.Errorf("removing PolicySet placement: %w", err)
	}
	if err := m.client.DeleteIfExists(ctx, client.GVRPolicySet, namespace, name); err != nil {
		return false, fmt.Errorf("removing PolicySet: %w", err)
	}

	return true, nil
}

func parsePolicySetInfo(obj map[string]interface{}) *PolicySetInfo {
	info := &PolicySetInfo{}

	if meta, ok := obj["metadata"].(map[string]interface{}); ok {
		info.Name, _ = meta["name"].(string)
		info.Namespace, _ = meta["namespace"].(string)
	}

	spec, _ := obj["spec"].(map[string]interface{})
	if spec != nil {
		info.Description, _ = spec["description"].(string)
		if policies, ok := spec["policies"].([]interface{}); ok {
			for _, p := range policies {
				if name, ok := p.(string); ok {
					info.Policies = append(info.Policies, name)
				}
			}
		}
	}

	status, _ := obj["status"].(map[string]interface{})
	if status != nil {
		info.Compliant, _ = status["compliant"].(string)
	}

	return info
}
