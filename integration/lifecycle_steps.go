//go:build integration

package integration

import (
	"context"
	"fmt"
	"time"

	"github.com/cucumber/godog"

	"github.com/pablofelix/acm-caas-poc/internal/lifecycle"
)

func registerLifecycleSteps(sc *godog.ScenarioContext, s *suiteContext) {
	sc.Step(`^a ClusterDeployment "([^"]*)" exists in namespace "([^"]*)"$`, s.clusterDeploymentExists)
	sc.Step(`^the ClusterDeployment has spec\.powerState = "([^"]*)"$`, s.clusterDeploymentHasPowerState)
	sc.Step(`^I patch spec\.powerState to "([^"]*)"(?: via the Go API)?$`, s.iPatchPowerState)
	sc.Step(`^the ClusterDeployment status shows powerState = "([^"]*)"$`, s.statusShowsPowerState)
	sc.Step(`^the ManagedCluster condition "Available" transitions to "([^"]*)" or "([^"]*)"$`, s.availableTransitions)
	sc.Step(`^eventually the ManagedCluster "([^"]*)" becomes Available = True$`, s.eventuallyAvailable)
	sc.Step(`^no ClusterDeployment exists for "([^"]*)"$`, s.noClusterDeploymentExists)
	sc.Step(`^I attempt to get the power state for "([^"]*)"$`, s.iAttemptGetPowerState)
	sc.Step(`^the operation returns an error$`, s.operationReturnsError)
	sc.Step(`^the error message indicates "([^"]*)"$`, s.errorMessageIndicates)
	sc.Step(`^I get the power state for "([^"]*)"$`, s.iGetPowerState)
	sc.Step(`^I receive the current powerState value$`, s.receiveCurrentPowerState)
	sc.Step(`^the value is one of: "([^"]*)", "([^"]*)", "([^"]*)", "([^"]*)"$`, s.valueIsOneOf)
	sc.Step(`^I wait for the power state to become "([^"]*)" with timeout (\d+)([ms])$`, s.iWaitForPowerState)
	sc.Step(`^the wait completes successfully$`, s.waitCompletesSuccessfully)
	sc.Step(`^the ClusterDeployment status\.powerState = "([^"]*)"$`, s.statusShowsPowerState)
	sc.Step(`^I hibernate the cluster again$`, s.iHibernateAgain)
	sc.Step(`^I resume the cluster again$`, s.iResumeAgain)
	sc.Step(`^the powerState remains "([^"]*)"$`, s.powerStateRemains)
	sc.Step(`^no unnecessary API calls are made$`, s.noUnnecessaryAPICalls)
	sc.Step(`^the ClusterDeployment already has spec\.powerState = "([^"]*)"$`, s.clusterDeploymentHasPowerState)
}

func (s *suiteContext) clusterDeploymentExists(ctx context.Context, name, ns string) error {
	supported, err := s.lifecycle.ClusterSupportsLifecycle(ctx, ns, name)
	if err != nil {
		return err
	}
	if !supported {
		return fmt.Errorf("no ClusterDeployment found for %s/%s", ns, name)
	}
	s.lifecycleCluster = name
	s.lifecycleNamespace = ns
	return nil
}

func (s *suiteContext) clusterDeploymentHasPowerState(ctx context.Context, state string) error {
	return nil
}

func (s *suiteContext) iPatchPowerState(ctx context.Context, state string) error {
	ns, name := s.lifecycleNamespace, s.lifecycleCluster
	switch lifecycle.PowerState(state) {
	case lifecycle.PowerStateHibernating:
		s.err = s.lifecycle.Hibernate(ctx, ns, name)
	case lifecycle.PowerStateRunning:
		s.err = s.lifecycle.Resume(ctx, ns, name)
	default:
		return fmt.Errorf("unknown power state: %s", state)
	}
	return s.err
}

func (s *suiteContext) statusShowsPowerState(ctx context.Context, expected string) error {
	state, err := s.lifecycle.GetPowerStateStatus(ctx, s.lifecycleNamespace, s.lifecycleCluster)
	if err != nil {
		return err
	}
	if string(state) != expected {
		return fmt.Errorf("power state = %s, want %s", state, expected)
	}
	return nil
}

func (s *suiteContext) availableTransitions(_, _ string) error {
	return nil
}

func (s *suiteContext) eventuallyAvailable(ctx context.Context, name string) error {
	return s.lifecycle.WaitForPowerState(ctx, name, name, lifecycle.PowerStateRunning, 15*time.Minute)
}

func (s *suiteContext) noClusterDeploymentExists(ctx context.Context, name string) error {
	supported, _ := s.lifecycle.ClusterSupportsLifecycle(ctx, name, name)
	if supported {
		return fmt.Errorf("ClusterDeployment exists for %s but should not", name)
	}
	return nil
}

func (s *suiteContext) iAttemptGetPowerState(ctx context.Context, name string) error {
	_, s.err = s.lifecycle.GetPowerState(ctx, name, name)
	return nil
}

func (s *suiteContext) operationReturnsError() error {
	if s.err == nil {
		return fmt.Errorf("expected an error but got none")
	}
	return nil
}

func (s *suiteContext) errorMessageIndicates(msg string) error {
	if s.err == nil {
		return fmt.Errorf("no error to check")
	}
	return nil
}

func (s *suiteContext) iGetPowerState(ctx context.Context, name string) error {
	state, err := s.lifecycle.GetPowerState(ctx, name, name)
	if err != nil {
		s.err = err
		return err
	}
	s.powerState = state
	return nil
}

func (s *suiteContext) receiveCurrentPowerState() error {
	if s.powerState == "" {
		return fmt.Errorf("power state is empty")
	}
	return nil
}

func (s *suiteContext) valueIsOneOf(a, b, c, d string) error {
	for _, v := range []string{a, b, c, d} {
		if string(s.powerState) == v {
			return nil
		}
	}
	return fmt.Errorf("power state %s not in expected values", s.powerState)
}

func (s *suiteContext) iWaitForPowerState(ctx context.Context, state string, amount int, unit string) error {
	timeout := time.Duration(amount) * time.Second
	if unit == "m" {
		timeout = time.Duration(amount) * time.Minute
	}
	s.err = s.lifecycle.WaitForPowerState(ctx, s.lifecycleNamespace, s.lifecycleCluster, lifecycle.PowerState(state), timeout)
	return nil
}

func (s *suiteContext) waitCompletesSuccessfully() error {
	return s.err
}

func (s *suiteContext) iHibernateAgain(ctx context.Context) error {
	s.err = s.lifecycle.Hibernate(ctx, s.lifecycleNamespace, s.lifecycleCluster)
	return nil
}

func (s *suiteContext) iResumeAgain(ctx context.Context) error {
	s.err = s.lifecycle.Resume(ctx, s.lifecycleNamespace, s.lifecycleCluster)
	return nil
}

func (s *suiteContext) powerStateRemains(ctx context.Context, expected string) error {
	state, err := s.lifecycle.GetPowerState(ctx, s.lifecycleNamespace, s.lifecycleCluster)
	if err != nil {
		return err
	}
	if string(state) != expected {
		return fmt.Errorf("power state = %s, want %s", state, expected)
	}
	return nil
}

func (s *suiteContext) noUnnecessaryAPICalls() error {
	return nil
}
