Feature: Legacy cluster decommissioning lifecycle
  As a platform operator
  I want to audit, notify, and safely decommission inherited clusters
  So that the CaaS platform can reclaim resources and reduce cost

  Background:
    Given the ACM hub is reachable
    And I have a dynamic client for the hub

  Scenario: Start a decommission workflow for a legacy cluster
    Given a ManagedCluster "legacy-1" exists and is available
    When I start a decommission workflow for "legacy-1" with owner "team-platform" and deadline "2026-10-01"
    Then a decommission state ConfigMap is created in namespace "open-cluster-management"
    And the state shows phase "Audit"
    And the state records the owner and deadline

  Scenario: Audit a legacy cluster for usage and workloads
    Given a decommission workflow for "legacy-1" is in phase "Audit"
    When I advance the decommission workflow for "legacy-1"
    Then the audit collects node count, CPU capacity, and memory capacity
    And non-system namespaces and their workload counts are listed
    And the state transitions to phase "Notify"

  Scenario: Notify the cluster owner before decommissioning
    Given a decommission workflow for "legacy-1" is in phase "Notify"
    When I advance the decommission workflow for "legacy-1"
    Then a notification record is stored with cluster name, usage summary, and deadline
    And the state transitions to phase "Backup"

  Scenario: Backup cluster state before deletion
    Given a decommission workflow for "legacy-1" is in phase "Backup"
    When I advance the decommission workflow for "legacy-1"
    Then namespace resources, PersistentVolumes, and cluster-scoped resources are exported
    And the backup manifest lists all exported resources
    And the state transitions to phase "Drain"

  Scenario: Drain and delete the cluster
    Given a decommission workflow for "legacy-1" is in phase "Drain"
    When I advance the decommission workflow for "legacy-1"
    Then worker nodes are cordoned and drained
    And the state transitions to phase "Delete"

  Scenario: Clean up ACM resources after cluster deletion
    Given a decommission workflow for "legacy-1" is in phase "Cleanup"
    When I advance the decommission workflow for "legacy-1"
    Then the ManagedCluster "legacy-1" is removed from the hub
    And all ManifestWorks in namespace "legacy-1" are deleted
    And all policies targeting "legacy-1" are cleaned up
    And the state transitions to phase "Done"

  Scenario: Decommission is idempotent and handles partial state
    Given a decommission workflow for "legacy-1" is in phase "Cleanup"
    And the ManagedCluster "legacy-1" was already deleted
    When I advance the decommission workflow for "legacy-1"
    Then the cleanup completes without errors
    And no resources are left behind in ACM

  Scenario: Cancel a decommission workflow
    Given a decommission workflow for "legacy-1" is in phase "Notify"
    When I cancel the decommission workflow for "legacy-1"
    Then the decommission state ConfigMap is removed
    And the cluster remains in the fleet

  Scenario: List all active decommission workflows
    Given decommission workflows exist for "legacy-1" and "legacy-2"
    When I list all decommission workflows
    Then I receive a list with cluster name, phase, owner, and deadline for each
