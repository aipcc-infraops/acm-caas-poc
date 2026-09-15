Feature: Cluster version upgrade management via ACM
  As a platform operator
  I want to manage OCP version upgrades from the hub
  So that the CaaS platform can keep clusters up to date

  Background:
    Given the ACM hub is reachable
    And I have a dynamic client for the hub

  Scenario: Get upgrade status for a Hive-provisioned OCP cluster
    Given a Hive-provisioned cluster "spoke1" with OCP version "4.16.5"
    When I get the upgrade status for "spoke1"
    Then the upgrade method is "hive"
    And the current version is "4.16.5"
    And the cluster type is "OCP"

  Scenario: Get upgrade status for an imported OCP cluster
    Given an imported OCP cluster "imported1" with version "4.15.10"
    When I get the upgrade status for "imported1"
    Then the upgrade method is "manifestwork"
    And the current version is "4.15.10"

  Scenario: Get upgrade status for a vanilla Kubernetes cluster
    Given a vanilla Kubernetes cluster "eks1" with version "v1.29.3"
    When I get the upgrade status for "eks1"
    Then the upgrade method is "report-only"
    And the current version is "v1.29.3"

  Scenario: List clusters with available upgrades
    Given multiple managed clusters exist
    When I list upgradeable clusters
    Then only OCP clusters with available updates are returned
    And vanilla Kubernetes clusters are excluded
    And clusters at latest version are excluded

  Scenario: Set update channel via ManifestWork
    Given a Hive-provisioned cluster "spoke1" on channel "stable-4.16"
    When I set the channel to "fast-4.16" for "spoke1"
    Then a ManifestWork "spoke1-channel" is created in namespace "spoke1"
    And the ManifestWork patches ClusterVersion spec.channel to "fast-4.16"
    And the ManifestWork uses ServerSideApply update strategy

  Scenario: Start upgrade via ManifestWork
    Given a Hive-provisioned cluster "spoke1" with available update "4.16.6"
    When I start an upgrade to "4.16.6" for "spoke1"
    Then a ManifestWork "spoke1-upgrade" is created in namespace "spoke1"
    And the ManifestWork patches ClusterVersion spec.desiredUpdate.version to "4.16.6"

  Scenario: Reject channel change for vanilla Kubernetes cluster
    Given a vanilla Kubernetes cluster "eks1"
    When I try to set the channel for "eks1"
    Then the operation returns an error about unsupported channel changes

  Scenario: Reject upgrade for vanilla Kubernetes cluster
    Given a vanilla Kubernetes cluster "eks1"
    When I try to start an upgrade for "eks1"
    Then the operation returns an error about unsupported upgrades

  Scenario: Get version upgrade history
    Given a cluster "spoke1" with OCP version history
    When I get the upgrade history for "spoke1"
    Then I receive version entries with state and timestamps

  Scenario: Idempotent channel change
    Given a ManifestWork "spoke1-channel" already exists
    When I set the channel to "candidate-4.16" for "spoke1"
    Then the existing ManifestWork is updated without error
