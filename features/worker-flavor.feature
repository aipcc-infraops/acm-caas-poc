Feature: Worker node flavor change via MachinePool rolling replacement
  As a platform operator
  I want to change the worker node instance type of a running cluster
  So that I can right-size compute without reprovisioning the cluster

  Background:
    Given the ACM hub is reachable
    And I have a dynamic client for the hub

  Scenario: Change worker instance type
    Given cluster "spoke1" has a MachinePool with worker type "bx2-4x16"
    When I set flavor on "spoke1" to "cx2-8x16"
    Then the MachinePool platform type is updated to "cx2-8x16"
    And Hive performs a rolling replacement of worker nodes

  Scenario: Change flavor on AWS cluster
    Given cluster "spoke2" has a MachinePool with worker type "m5.xlarge"
    And the ClusterDeployment platform is "aws"
    When I set flavor on "spoke2" to "m6i.2xlarge"
    Then the MachinePool spec.platform.aws.type is "m6i.2xlarge"

  Scenario: Flavor change fails without MachinePool
    Given cluster "imported-1" has no MachinePool
    When I attempt to set flavor on "imported-1"
    Then an error indicates no MachinePool exists

  Scenario: Flavor change fails without detectable platform
    Given cluster "spoke3" has a MachinePool but an empty platform spec
    When I attempt to set flavor on "spoke3"
    Then an error indicates the platform cannot be detected
