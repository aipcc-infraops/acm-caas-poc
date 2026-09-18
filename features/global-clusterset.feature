Feature: Global ManagedClusterSet (UC-54)

  As a platform operator
  I want a global ClusterSet that matches all clusters
  So that cross-team Placements can target the entire fleet without explicit membership

  Scenario: Enable the global ClusterSet
    When I run "acmlab clusterset global-enable"
    Then a ManagedClusterSet "global" is created with LabelSelector matching all clusters

  Scenario: Bind global ClusterSet to a namespace
    Given the global ClusterSet is enabled
    When I run "acmlab clusterset global-bind team-alpha"
    Then a ManagedClusterSetBinding "global" is created in namespace "team-alpha"

  Scenario: Unbind global ClusterSet from a namespace
    Given the global ClusterSet is bound to "team-alpha"
    When I run "acmlab clusterset global-unbind team-alpha"
    Then the binding is removed from namespace "team-alpha"

  Scenario: Show global ClusterSet status
    Given the global ClusterSet is enabled and bound to "team-alpha" and "team-beta"
    When I run "acmlab clusterset global-status"
    Then the output shows enabled status and all bound namespaces

  Scenario: Bind fails if global set not enabled
    When I run "acmlab clusterset global-bind team-alpha"
    Then the command returns an error about missing global set
