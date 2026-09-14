Feature: External cluster import into ACM
  As a platform operator
  I want to import clusters not provisioned by ACM
  So that the ComputeRequest controller can manage heterogeneous fleets

  Background:
    Given the ACM hub is reachable
    And I have a dynamic client for the hub

  Scenario: Import a cluster with auto-import secret
    Given I have a kubeconfig for external cluster "import-test"
    When I import cluster "import-test" with labels "cloud=test,env=dev"
    Then a ManagedCluster "import-test" exists on the hub
    And a KlusterletAddonConfig exists in namespace "import-test"
    And an auto-import secret exists in namespace "import-test"

  Scenario: Check import status
    Given cluster "import-test" has been imported
    When I get the import status of "import-test"
    Then I receive availability and join state

  Scenario: List imported clusters
    Given cluster "import-test" has been imported
    When I list all imported clusters
    Then the list includes "import-test"

  Scenario: Detach an imported cluster
    Given cluster "import-test" has been imported
    When I detach cluster "import-test"
    Then the ManagedCluster "import-test" is removed from the hub
