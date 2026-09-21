@core
Feature: Cluster worker node scaling via Hive MachinePool
  As a platform operator
  I want to scale worker nodes in managed clusters from the hub
  So that the CaaS platform can adjust capacity to tenant demand

  Background:
    Given the ACM hub is reachable
    And I have a dynamic client for the hub

  Scenario: Query current MachinePool configuration
    Given a Hive-provisioned cluster "spoke1" has a MachinePool
    When I get the MachinePool for "spoke1"
    Then I receive the replica count and platform type

  Scenario: Scale up workers by increasing replicas
    Given a Hive-provisioned cluster "spoke1" has a MachinePool
    When I set the MachinePool replicas to 5 for "spoke1"
    Then the MachinePool for "spoke1" has replicas set to 5

  Scenario: Enable autoscaling on a MachinePool
    Given a Hive-provisioned cluster "spoke1" has a MachinePool
    When I enable autoscaling with min 3 and max 10 for "spoke1"
    Then the MachinePool for "spoke1" has autoscaling enabled

  Scenario: List all MachinePools across the fleet
    When I list all MachinePools
    Then I receive MachinePool information for each Hive cluster

  Scenario: Reject scaling for imported clusters
    Given cluster "import-test" was imported without Hive
    When I try to get the MachinePool for "import-test"
    Then the operation returns an ErrNoMachinePool error
