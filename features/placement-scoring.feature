Feature: Placement scoring for cluster selection (UC-48)

  As a platform operator
  I want to rank clusters by resource availability
  So that workloads land on the most suitable cluster

  Scenario: Configure scoring with default prioritizers
    Given a hub cluster with managed clusters
    When I run "acmlab fleet scoring-configure gpu-scoring"
    Then a Placement "gpu-scoring" is created with ResourceAllocatableCPU and ResourceAllocatableMemory prioritizers
    And the Placement has label "acmlab.redhat.com/scoring=true"

  Scenario: Configure scoring with custom prioritizers and ClusterSet
    When I run "acmlab fleet scoring-configure custom --prioritizers ResourceAllocatableCPU --cluster-set gpu-clusters"
    Then the Placement is scoped to ClusterSet "gpu-clusters"

  Scenario: View scoring decisions
    Given a scoring placement "gpu-scoring" exists
    When I run "acmlab fleet scoring-status gpu-scoring"
    Then the output shows cluster names and placement decisions

  Scenario: Remove a scoring placement
    Given a scoring placement "gpu-scoring" exists
    When I run "acmlab fleet scoring-remove gpu-scoring"
    Then the Placement is deleted

  Scenario: List all scoring placements
    Given multiple scoring placements exist
    When I run "acmlab fleet scoring-list"
    Then the output shows name and namespace for each scoring placement
