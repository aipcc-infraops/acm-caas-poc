Feature: PolicySet compliance profiles (UC-49)

  As a platform operator
  I want to group related policies into compliance profiles
  So that I can manage compliance holistically across clusters

  Scenario: Create a PolicySet with multiple policies
    Given policies "require-labels" and "no-privileged" exist
    When I run "acmlab policy apply-set cis-baseline --policies require-labels,no-privileged --description 'CIS Level 1 compliance profile'"
    Then a PolicySet "cis-baseline" is created with both policies
    And a Placement and PlacementBinding are created for the PolicySet

  Scenario: Create a PolicySet scoped to a ClusterSet
    When I run "acmlab policy apply-set gpu-compliance --policies gpu-limits --cluster-set gpu-clusters"
    Then the PolicySet placement is scoped to ClusterSet "gpu-clusters"

  Scenario: View PolicySet details
    Given a PolicySet "cis-baseline" exists
    When I run "acmlab policy get-set cis-baseline"
    Then the output shows the PolicySet name, description, compliance, and member policies

  Scenario: List all PolicySets
    Given multiple PolicySets exist
    When I run "acmlab policy list-sets"
    Then the output shows each PolicySet with compliance status and policy count

  Scenario: Remove a PolicySet
    Given a PolicySet "cis-baseline" exists
    When I run "acmlab policy remove-set cis-baseline"
    Then the PolicySet, Placement, and PlacementBinding are all deleted
