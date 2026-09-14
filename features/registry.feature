Feature: Registry mirror for restricted-registry clusters
  As a platform operator
  I want to configure image registry mirrors for clusters with restricted access
  So that ACM can manage clusters that cannot reach public registries

  Background:
    Given the ACM hub is reachable
    And I have a dynamic client for the hub

  Scenario: List required images for a cluster
    Given a managed cluster "spoke1" exists with ManifestWorks
    When I list required images for "spoke1"
    Then I receive a list of container images extracted from ManifestWorks

  Scenario: Generate a mirror script
    Given I have a list of required images for "spoke1"
    When I generate a mirror script targeting "mirror.example.com/acm"
    Then the script contains skopeo copy commands for each image

  Scenario: Configure a registry mirror on the hub
    Given images have been mirrored to "mirror.example.com/acm"
    When I configure a registry mirror for "spoke1" with target "mirror.example.com/acm"
    Then a ManagedClusterImageRegistry exists for "spoke1"
    And a Placement with tolerations exists for "spoke1"

  Scenario: Check registry mirror status
    Given a registry mirror is configured for "spoke1"
    When I get the mirror status for "spoke1"
    Then I receive the mirror configuration and registry mappings

  Scenario: Remove a registry mirror
    Given a registry mirror is configured for "spoke1"
    When I remove the registry mirror for "spoke1"
    Then the ManagedClusterImageRegistry for "spoke1" is removed
