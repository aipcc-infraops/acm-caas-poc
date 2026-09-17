Feature: Workload disaster recovery via Velero + GitOps (UC-42)
  As a platform operator
  I want automated disaster recovery with Velero backups and GitOps failover
  So that workloads survive cluster failures with minimal downtime

  Background:
    Given the ACM hub is reachable
    And I have a dynamic client for the hub

  Scenario: Enable DR for a cluster pair
    Given a ManagedCluster "prod-east" exists
    And a ManagedCluster "prod-west" exists
    When I enable DR with source "prod-east" and target "prod-west"
    Then a ManifestWork "dr-labels-prod-east-prod-west" exists in namespace "prod-east"
    And a ManifestWork "dr-labels-prod-east-prod-west" exists in namespace "prod-west"
    And a ManifestWork "dr-velero-prod-east-prod-west" exists in namespace "prod-east"
    And a ConfigurationPolicy "dr-velero-policy-prod-east-prod-west" enforces Velero schedules

  Scenario: Trigger failover to standby cluster
    Given DR is enabled for pair "prod-east" to "prod-west"
    When I trigger failover from "prod-east" to "prod-west"
    Then a ManifestWork "dr-restore-prod-east-prod-west" exists in namespace "prod-west"
    And an ApplicationSet "dr-failover-prod-east-prod-west" re-routes GitOps to "prod-west"

  Scenario: Check failover status
    Given a failover is in progress from "prod-east" to "prod-west"
    When I check failover status
    Then the phase is "InProgress" or "Completed"
    And restore phase and GitOps status are reported

  Scenario: List DR pairs
    Given DR is enabled for at least one cluster pair
    When I list DR pairs
    Then each pair shows source, target, and status

  @slow
  Scenario: Disable DR for a cluster
    Given DR is enabled for pair "prod-east" to "prod-west"
    When I disable DR for "prod-east"
    Then all DR ManifestWorks, policies, and ApplicationSets are removed
