# Feature: Cluster lifecycle management (hibernate/resume)
#
# As a platform operator
# I want to hibernate and resume clusters programmatically
# So that the ComputeRequest controller can implement idle reclamation

@core
Feature: Cluster power management via Hive Go API

  Background:
    Given the ACM hub is reachable
    And I have a dynamic client for hive.openshift.io/v1

  Scenario: Hibernate a Hive-provisioned cluster by patching powerState
    Given a ClusterDeployment "spoke2" exists in namespace "spoke2"
    And the ClusterDeployment has spec.powerState = "Running"
    When I patch spec.powerState to "Hibernating" via the Go API
    Then the ClusterDeployment status shows powerState = "Hibernating"
    And the ManagedCluster condition "Available" transitions to "Unknown" or "False"

  Scenario: Resume a hibernated cluster
    Given a ClusterDeployment "spoke2" exists in namespace "spoke2"
    And the ClusterDeployment has spec.powerState = "Hibernating"
    When I patch spec.powerState to "Running" via the Go API
    Then the ClusterDeployment status shows powerState = "Running"
    And eventually the ManagedCluster "spoke2" becomes Available = True

  Scenario: Verify lifecycle limitations on imported clusters
    Given a managed cluster "external-cluster" exists for lifecycle check
    And no ClusterDeployment exists for "external-cluster"
    When I attempt to get the power state for "external-cluster"
    Then the operation returns an error
    And the error message indicates "no ClusterDeployment found"

  Scenario: Check current power state of a cluster
    Given a ClusterDeployment "spoke2" exists in namespace "spoke2"
    When I get the power state for "spoke2"
    Then I receive the current powerState value
    And the value is one of: "Running", "Hibernating", "Stopping", "Resuming"

  Scenario: Wait for power state transition to complete
    Given a ClusterDeployment "spoke2" exists in namespace "spoke2"
    And the ClusterDeployment has spec.powerState = "Running"
    When I patch spec.powerState to "Hibernating"
    And I wait for the power state to become "Hibernating" with timeout 10m
    Then the wait completes successfully
    And the ClusterDeployment status.powerState = "Hibernating"

  Scenario: Handle power state transition timeout gracefully
    Given a ClusterDeployment "spoke2" exists in namespace "spoke2"
    And the ClusterDeployment has spec.powerState = "Running"
    When I wait for the power state to become "Hibernating" with timeout 1s
    Then the operation returns a timeout error

  Scenario: Idempotent hibernate operation
    Given a ClusterDeployment "spoke2" exists in namespace "spoke2"
    And the ClusterDeployment already has spec.powerState = "Hibernating"
    When I hibernate the cluster again
    Then the wait completes successfully
    And the powerState remains "Hibernating"
    And no unnecessary API calls are made

  Scenario: Idempotent resume operation
    Given a ClusterDeployment "spoke2" exists in namespace "spoke2"
    And the ClusterDeployment already has spec.powerState = "Running"
    When I resume the cluster again
    Then the wait completes successfully
    And the powerState remains "Running"
    And no unnecessary API calls are made
