Feature: Tenant RBAC isolation via ManifestWork
  As a platform operator
  I want to deploy tenant isolation resources to spoke clusters
  So that the ComputeRequest controller can isolate teams on shared clusters

  Background:
    Given the ACM hub is reachable
    And I have a dynamic client for the hub
    And a managed cluster "spoke1" exists

  Scenario: Deploy tenant isolation to a spoke
    When I deploy tenant "team-alpha" to cluster "spoke1" with cpu "8" and memory "16Gi"
    Then a ManifestWork "tenant-team-alpha" exists in namespace "spoke1"
    And the ManifestWork contains a Namespace, RoleBinding, NetworkPolicy, and ResourceQuota

  Scenario: List tenants on a spoke
    Given tenant "team-alpha" is deployed to cluster "spoke1"
    When I list tenants on cluster "spoke1"
    Then the list includes tenant "team-alpha"

  Scenario: Check tenant deployment status
    Given tenant "team-alpha" is deployed to cluster "spoke1"
    When I get the status of tenant "team-alpha" on cluster "spoke1"
    Then I receive the ManifestWork sync status

  Scenario: Remove tenant isolation from a spoke
    Given tenant "team-alpha" is deployed to cluster "spoke1"
    When I remove tenant "team-alpha" from cluster "spoke1"
    Then the ManifestWork "tenant-team-alpha" no longer exists in namespace "spoke1"
