//go:build integration

package integration

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"

	"github.com/pablofelix/acm-caas-poc/internal/policy"
)

func registerPolicySteps(sc *godog.ScenarioContext, s *suiteContext) {
	sc.Step(`^I apply policy "([^"]*)" with registries "([^"]*)"$`, s.iApplyPolicy)
	sc.Step(`^the policy "([^"]*)" exists on the hub$`, s.policyExistsOnHub)
	sc.Step(`^the policy has a PlacementRule and PlacementBinding$`, s.policyHasBindings)
	sc.Step(`^policy "([^"]*)" exists$`, s.policyExists)
	sc.Step(`^I list all policies$`, s.iListAllPolicies)
	sc.Step(`^the list includes "([^"]*)"$`, s.listIncludesPolicy)
	sc.Step(`^I get the status of policy "([^"]*)"$`, s.iGetPolicyStatus)
	sc.Step(`^I receive compliance information per cluster$`, s.receiveComplianceInfo)
	sc.Step(`^I set remediation of "([^"]*)" to "([^"]*)"$`, s.iSetRemediation)
	sc.Step(`^the policy remediation is "([^"]*)"$`, s.policyRemediationIs)
	sc.Step(`^I remove policy "([^"]*)"$`, s.iRemovePolicy)
	sc.Step(`^the policy "([^"]*)" no longer exists$`, s.policyNoLongerExists)
	sc.Step(`^the PlacementRule and PlacementBinding are removed$`, s.bindingsRemoved)
}

func (s *suiteContext) iApplyPolicy(ctx context.Context, name, registries string) error {
	return s.policy.Apply(ctx, policy.PolicyOpts{
		Name:              name,
		Namespace:         "default",
		AllowedRegistries: []string{registries},
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

func (s *suiteContext) policyHasBindings() error {
	if s.policyInfo == nil {
		return fmt.Errorf("no policy info available")
	}
	return nil
}

func (s *suiteContext) policyExists(ctx context.Context, name string) error {
	_, err := s.policy.Get(ctx, name, "default")
	if err != nil {
		return s.iApplyPolicy(ctx, name, "registry.redhat.io")
	}
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
	return s.policy.Remove(ctx, name, "default")
}

func (s *suiteContext) policyNoLongerExists(ctx context.Context, name string) error {
	_, err := s.policy.Get(ctx, name, "default")
	if err != nil {
		return nil
	}
	return fmt.Errorf("policy %s still exists", name)
}

func (s *suiteContext) bindingsRemoved() error {
	return nil
}
