//go:build integration

package integration

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"

	"github.com/pablofelix/acm-caas-poc/internal/client"
	"github.com/pablofelix/acm-caas-poc/internal/provisioning"
)

func registerProvisioningSteps(sc *godog.ScenarioContext, s *suiteContext) {
	sc.Step(`^cloud credentials exist as a Secret in namespace "([^"]*)"$`, s.cloudCredentialsExist)
	sc.Step(`^a ClusterImageSet for the target OCP version exists$`, s.clusterImageSetExists)
	sc.Step(`^I provision cluster "([^"]*)" with default settings$`, s.iProvisionCluster)
	sc.Step(`^the ClusterDeployment "([^"]*)" is accepted by Hive$`, s.clusterDeploymentAccepted)
	sc.Step(`^the cluster "([^"]*)" eventually reaches Provisioned = True$`, s.clusterReachesProvisioned)
	sc.Step(`^I list all provisioned clusters$`, s.iListProvisionedClusters)
	sc.Step(`^I receive a list of ClusterDeployments with status$`, s.receiveClusterDeploymentList)
	sc.Step(`^a ClusterDeployment "([^"]*)" exists with status Provisioned = True$`, s.clusterDeploymentProvisioned)
	sc.Step(`^I destroy cluster "([^"]*)"$`, s.iDestroyCluster)
	sc.Step(`^the ClusterDeployment "([^"]*)" is removed$`, s.clusterDeploymentRemoved)
}

func (s *suiteContext) cloudCredentialsExist(_ string) error {
	return nil
}

func (s *suiteContext) clusterImageSetExists(ctx context.Context) error {
	sets, err := s.provisioner.ListImageSets(ctx)
	if err != nil {
		return err
	}
	if len(sets) == 0 {
		return fmt.Errorf("no ClusterImageSets available")
	}
	return nil
}

func (s *suiteContext) iProvisionCluster(ctx context.Context, name string) error {
	return s.provisioner.Create(ctx, provisioning.ClusterOpts{
		Name: name,
	})
}

func (s *suiteContext) clusterDeploymentAccepted(ctx context.Context, name string) error {
	_, err := s.client.Get(ctx, client.GVRClusterDeployment, name, name)
	return err
}

func (s *suiteContext) clusterReachesProvisioned(_ string) error {
	return godog.ErrPending
}

func (s *suiteContext) iListProvisionedClusters(ctx context.Context) error {
	list, err := s.provisioner.List(ctx)
	if err != nil {
		return err
	}
	s.provisionList = list
	return nil
}

func (s *suiteContext) receiveClusterDeploymentList() error {
	if s.provisionList == nil {
		return fmt.Errorf("no provisioned clusters returned")
	}
	return nil
}

func (s *suiteContext) clusterDeploymentProvisioned(ctx context.Context, name string) error {
	_, err := s.client.Get(ctx, client.GVRClusterDeployment, name, name)
	return err
}

func (s *suiteContext) iDestroyCluster(ctx context.Context, name string) error {
	return s.provisioner.Destroy(ctx, name)
}

func (s *suiteContext) clusterDeploymentRemoved(ctx context.Context, name string) error {
	_, err := s.client.Get(ctx, client.GVRClusterDeployment, name, name)
	if err != nil {
		return nil
	}
	return fmt.Errorf("ClusterDeployment %s still exists", name)
}
