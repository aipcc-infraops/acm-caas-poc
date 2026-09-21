//go:build integration

package integration

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/policy"
)

func registerPolicySteps(sc *godog.ScenarioContext, s *suiteContext) {
	sc.Step(`^I apply policy "([^"]*)" with registries "([^"]*)"$`, s.iApplyPolicy)
	sc.Step(`^the policy "([^"]*)" exists on the hub$`, s.policyExistsOnHub)
	sc.Step(`^the policy has a Placement and PlacementBinding$`, s.policyHasBindings)
	sc.Step(`^policy "([^"]*)" exists$`, s.policyExists)
	sc.Step(`^I list all policies$`, s.iListAllPolicies)
	sc.Step(`^the policy list includes "([^"]*)"$`, s.listIncludesPolicy)
	sc.Step(`^I get the status of policy "([^"]*)"$`, s.iGetPolicyStatus)
	sc.Step(`^I receive compliance information per cluster$`, s.receiveComplianceInfo)
	sc.Step(`^I set remediation of "([^"]*)" to "([^"]*)"$`, s.iSetRemediation)
	sc.Step(`^the policy remediation is "([^"]*)"$`, s.policyRemediationIs)
	sc.Step(`^I remove policy "([^"]*)"$`, s.iRemovePolicy)
	sc.Step(`^the policy "([^"]*)" no longer exists$`, s.policyNoLongerExists)
	sc.Step(`^the Placement and PlacementBinding are removed$`, s.bindingsRemoved)
}

func (s *suiteContext) iApplyPolicy(ctx context.Context, name, registries string) error {
	regs := strings.Split(registries, ",")
	for i := range regs {
		regs[i] = strings.TrimSpace(regs[i])
	}
	return s.policy.Apply(ctx, policy.PolicyOpts{
		Name:              name,
		Namespace:         "default",
		AllowedRegistries: regs,
		RemediationAction: "inform",
	})
}

func (s *suiteContext) policyExistsOnHub(ctx context.Context, name string) error {
	info, err := s.policy.Get(ctx, name, "default")
	if err != nil {
		return fmt.Errorf("policy %s not found: %w", name, err)
	}
	s.policyInfo = info
	return nil
}

func (s *suiteContext) policyHasBindings(ctx context.Context) error {
	if s.policyInfo == nil {
		return fmt.Errorf("no policy info available")
	}
	name := s.policyInfo.Name
	ns := "default"
	placementName := name + "-placement"
	bindingName := name + "-placement-binding"
	if _, err := s.client.Get(ctx, client.GVRPlacement, ns, placementName); err != nil {
		return fmt.Errorf("Placement %s/%s not found: %w", ns, placementName, err)
	}
	if _, err := s.client.Get(ctx, client.GVRPlacementBinding, ns, bindingName); err != nil {
		return fmt.Errorf("PlacementBinding %s/%s not found: %w", ns, bindingName, err)
	}
	return nil
}

func (s *suiteContext) policyExists(ctx context.Context, name string) error {
	info, err := s.policy.Get(ctx, name, "default")
	if err != nil {
		if applyErr := s.iApplyPolicy(ctx, name, "registry.redhat.io"); applyErr != nil {
			return applyErr
		}
		info, err = s.policy.Get(ctx, name, "default")
		if err != nil {
			return err
		}
	}
	s.policyInfo = info
	return nil
}

func (s *suiteContext) iListAllPolicies(ctx context.Context) error {
	policies, err := s.policy.List(ctx, "default")
	if err != nil {
		return err
	}
	s.policies = policies
	return nil
}

func (s *suiteContext) listIncludesPolicy(name string) error {
	for _, p := range s.policies {
		if p.Name == name {
			return nil
		}
	}
	return fmt.Errorf("policy %s not found in list", name)
}

func (s *suiteContext) iGetPolicyStatus(ctx context.Context, name string) error {
	info, err := s.policy.Get(ctx, name, "default")
	if err != nil {
		return err
	}
	s.policyInfo = info
	return nil
}

func (s *suiteContext) receiveComplianceInfo() error {
	if s.policyInfo == nil {
		return fmt.Errorf("no policy info available")
	}
	return nil
}

func (s *suiteContext) iSetRemediation(ctx context.Context, name, action string) error {
	return s.policy.SetRemediation(ctx, name, "default", action)
}

func (s *suiteContext) policyRemediationIs(ctx context.Context, action string) error {
	if s.policyInfo == nil {
		return fmt.Errorf("no policy info")
	}
	info, err := s.policy.Get(ctx, s.policyInfo.Name, "default")
	if err != nil {
		return err
	}
	if info.RemediationAction != action {
		return fmt.Errorf("remediation = %s, want %s", info.RemediationAction, action)
	}
	return nil
}

func (s *suiteContext) iRemovePolicy(ctx context.Context, name string) error {
	_, err := s.policy.Remove(ctx, name, "default")
	return err
}

func (s *suiteContext) policyNoLongerExists(ctx context.Context, name string) error {
	_, err := s.policy.Get(ctx, name, "default")
	if err == nil {
		return fmt.Errorf("policy %s still exists", name)
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("unexpected error checking policy %s: %w", name, err)
	}
	return nil
}

func (s *suiteContext) bindingsRemoved(ctx context.Context) error {
	if s.policyInfo == nil {
		return fmt.Errorf("no policy info available")
	}
	name := s.policyInfo.Name
	ns := "default"
	placementName := name + "-placement"
	bindingName := name + "-placement-binding"
	if _, err := s.client.Get(ctx, client.GVRPlacement, ns, placementName); err == nil {
		return fmt.Errorf("Placement %s/%s still exists", ns, placementName)
	} else if !errors.IsNotFound(err) {
		return fmt.Errorf("unexpected error checking Placement %s: %w", placementName, err)
	}
	if _, err := s.client.Get(ctx, client.GVRPlacementBinding, ns, bindingName); err == nil {
		return fmt.Errorf("PlacementBinding %s/%s still exists", ns, bindingName)
	} else if !errors.IsNotFound(err) {
		return fmt.Errorf("unexpected error checking PlacementBinding %s: %w", bindingName, err)
	}
	return nil
}
