Feature: Cluster relocation via planned workload migration (UC-43)
  As a platform operator
  I want to plan and execute workload migration between clusters
  So that I can relocate workloads without downtime during maintenance or decommissioning

  Background:
    Given the ACM hub is reachable
    And I have a dynamic client for the hub

  Scenario: Plan a migration
    Given a ManagedCluster "prod-east" exists with ManifestWorks deployed
    And a ManagedCluster "prod-west" exists
    When I plan a migration from "prod-east" to "prod-west"
    Then a migration plan is created with workload count and namespaces
    And a ConfigMap "migration-plan-prod-east-to-prod-west" stores the plan

  Scenario: Execute a migration plan
    Given a migration plan "prod-east-to-prod-west" exists with phase "Planned"
    When I execute the migration plan
    Then the source cluster is cordoned with label "acmlab.redhat.com/migration=draining"
    And workloads are deployed to the target via ManifestWork
    And the plan phase transitions to "Verifying"

  Scenario: Check migration status
    Given a migration is in progress for plan "prod-east-to-prod-west"
    When I check migration status
    Then the phase, workload count, and migrated count are reported

  Scenario: Rollback a migration
    Given a migration plan "prod-east-to-prod-west" is in phase "Deploying"
    When I rollback the migration
    Then target ManifestWorks are removed
    And the source cluster cordon is removed
    And the plan phase is set to "RolledBack"

  Scenario: List all migrations
    Given at least one migration plan exists
    When I list migrations
    Then each migration shows plan ID, source, target, phase, and creation time
