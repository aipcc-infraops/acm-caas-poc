Feature: Per-team compliance reporting via ClusterSet-scoped policies
  As a platform operator
  I want to see compliance status per team
  So that I know which team's clusters are violating policies

  Background:
    Given the ACM hub is reachable
    And I have a dynamic client for the hub

  Scenario: Apply a policy scoped to a team ClusterSet
    Given ClusterSet "team-serving" exists
    When I apply policy "gpu-check" with cluster-set "team-serving"
    Then the Placement has spec.clusterSets containing "team-serving"
    And compliance is reported only for clusters in "team-serving"

  Scenario: Generate fleet-wide per-team compliance report
    Given policies are active across multiple ClusterSets
    When I generate a compliance report
    Then I see per-ClusterSet: total evaluations, compliant, non-compliant, pending
    And clusters without a ClusterSet label appear under "default"

  Scenario: Combined ClusterSet and label scoping
    When I apply policy "narrow" with cluster-set "team-gpu" and labels "gpu=true"
    Then the Placement has both clusterSets and predicates set
