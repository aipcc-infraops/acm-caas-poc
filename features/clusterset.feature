Feature: ClusterSet management for team-based fleet isolation
  As a platform operator
  I want to create and manage ClusterSets per team
  So that each team can only see and manage their own clusters

  Background:
    Given the ACM hub is reachable
    And I have a dynamic client for the hub

  Scenario: Create a team ClusterSet
    When I create ClusterSet "team-serving" with binding in namespace "serving-ns"
    Then a ManagedClusterSet "team-serving" exists
    And a ManagedClusterSetBinding "team-serving" exists in namespace "serving-ns"
    And the binding references ClusterSet "team-serving"

  Scenario: Create is idempotent
    Given ClusterSet "team-serving" already exists
    When I create ClusterSet "team-serving" with binding in namespace "serving-ns"
    Then no error is returned

  Scenario: Assign a cluster to a ClusterSet
    Given ClusterSet "team-serving" exists
    When I assign cluster "spoke1" to ClusterSet "team-serving"
    Then cluster "spoke1" has label "cluster.open-cluster-management.io/clusterset=team-serving"

  Scenario: List ClusterSets with member counts
    Given ClusterSet "team-serving" exists with members "spoke1" and "spoke2"
    When I list ClusterSets
    Then "team-serving" shows count 2

  Scenario: Remove a ClusterSet
    Given ClusterSet "team-serving" exists
    When I remove ClusterSet "team-serving" from namespace "serving-ns"
    Then the ManagedClusterSet and ManagedClusterSetBinding are deleted
