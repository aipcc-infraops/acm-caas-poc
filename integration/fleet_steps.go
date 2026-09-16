//go:build integration

package integration

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"
)

func registerFleetSteps(sc *godog.ScenarioContext, s *suiteContext) {
	sc.Step(`^I list ManagedCluster resources$`, s.iListManagedClusterResources)
	sc.Step(`^each cluster has a name and condition list$`, s.eachClusterHasNameAndConditions)
	sc.Step(`^available clusters report Available = True$`, s.availableClustersReportTrue)
	sc.Step(`^a ManagedCluster "([^"]*)" exists$`, s.aManagedClusterExists)
	sc.Step(`^I get ManagedCluster "([^"]*)"$`, s.iGetManagedCluster)
	sc.Step(`^the cluster info includes name, labels, and conditions$`, s.clusterInfoIncludesDetails)
}

func (s *suiteContext) iListManagedClusterResources(ctx context.Context) error {
	clusters, err := s.fleet.ListClusters(ctx, "")
	if err != nil {
		return err
	}
	s.clusters = clusters
	return nil
}

func (s *suiteContext) eachClusterHasNameAndConditions() error {
	for _, c := range s.clusters {
		if c.Name == "" {
			return fmt.Errorf("found cluster with empty name")
		}
		if len(c.Conditions) == 0 {
			return fmt.Errorf("cluster %s has no conditions", c.Name)
		}
	}
	return nil
}

func (s *suiteContext) availableClustersReportTrue() error {
	for _, c := range s.clusters {
		if c.Available && c.Name != "" {
			return nil
		}
	}
	return fmt.Errorf("no available clusters found")
}

func (s *suiteContext) aManagedClusterExists(ctx context.Context, name string) error {
	cluster, err := s.fleet.GetCluster(ctx, name)
	if err != nil {
		return fmt.Errorf("ManagedCluster %s does not exist: %w", name, err)
	}
	s.cluster = cluster
	return nil
}

func (s *suiteContext) iGetManagedCluster(ctx context.Context, name string) error {
	cluster, err := s.fleet.GetCluster(ctx, name)
	if err != nil {
		return err
	}
	s.cluster = cluster
	return nil
}

func (s *suiteContext) clusterInfoIncludesDetails() error {
	if s.cluster == nil {
		return fmt.Errorf("no cluster info available")
	}
	if s.cluster.Name == "" {
		return fmt.Errorf("cluster name is empty")
	}
	if len(s.cluster.Conditions) == 0 {
		return fmt.Errorf("cluster has no conditions")
	}
	return nil
}
