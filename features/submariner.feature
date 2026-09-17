Feature: UC-26 Multi-cluster networking via Submariner

  As a platform engineer
  I want to enable cross-cluster networking via Submariner
  So that pods across different managed clusters can communicate directly

  Background:
    Given a hub cluster running ACM 2.17
    And a ManagedClusterSet "prod-set" with clusters "spoke1" and "spoke2"

  Scenario: Enable Submariner for a ClusterSet
    When I enable Submariner for ClusterSet "prod-set"
    Then a ManagedClusterAddOn "submariner" is created in each cluster namespace
    And a SubmarinerConfig is created in each cluster namespace
    And each cluster is labelled "submariner=enabled"
    And the SubmarinerConfig uses cableDriver "libreswan"

  Scenario: Disable Submariner for a ClusterSet
    Given Submariner is enabled for ClusterSet "prod-set"
    When I disable Submariner for ClusterSet "prod-set"
    Then the ManagedClusterAddOn "submariner" is removed from each cluster namespace
    And the SubmarinerConfig is removed from each cluster namespace

  Scenario: Check Submariner connectivity status
    Given Submariner is enabled for ClusterSet "prod-set"
    And all clusters have gateway nodes labelled and agents running
    When I check the status of ClusterSet "prod-set"
    Then the status shows "connected" with all gateways ready
    And each cluster reports at least 1 active connection

  Scenario: Status shows disconnected when addon is missing
    Given Submariner has not been deployed to ClusterSet "prod-set"
    When I check the status of ClusterSet "prod-set"
    Then the status shows "disconnected"

  Scenario: List Submariner-enabled cluster sets
    Given Submariner is enabled for ClusterSet "prod-set"
    When I list all Submariner deployments
    Then the result includes ClusterSet "prod-set" with 2 clusters

  Scenario: Enable Submariner fails for empty ClusterSet
    Given ClusterSet "empty-set" has no clusters
    When I try to enable Submariner for ClusterSet "empty-set"
    Then the operation fails with "no clusters found"

  Scenario: Enable Submariner across three clusters
    Given a ManagedClusterSet "multi-set" with clusters "c1", "c2", and "c3"
    When I enable Submariner for ClusterSet "multi-set"
    Then a ManagedClusterAddOn "submariner" is created in each of the 3 cluster namespaces
    And a SubmarinerConfig is created in each of the 3 cluster namespaces
