Feature: Cluster discovery via OpenShift Cluster Manager (UC-56)

  As a platform operator
  I want to automatically discover unmanaged OpenShift clusters
  So that I can import them into ACM without manually tracking inventory

  Scenario: Enable cluster discovery
    Given access to the ACM hub
    When I run "acmlab discovery enable --token <ocm-token> --last-active 7 --versions 4.14,4.15"
    Then a Secret "ocm-api-token" is created in "open-cluster-management"
    And a DiscoveryConfig "discovery" is created with the credential reference
    And the filter restricts to clusters active within 7 days

  Scenario: List discovered clusters
    Given discovery is enabled in "open-cluster-management"
    And the OCM API reports unmanaged clusters
    When I run "acmlab discovery list"
    Then the output shows name, cloud provider, version, region, and status

  Scenario: List discovered clusters as JSON
    When I run "acmlab discovery list --json"
    Then the output is valid JSON with discovered cluster details

  Scenario: Import a discovered cluster
    Given a DiscoveredCluster "rosa-prod" exists
    When I run "acmlab discovery import rosa-prod"
    Then a ManagedCluster "rosa-prod" is created with label "created-via=discovery"
    And a KlusterletAddonConfig is created in namespace "rosa-prod"

  Scenario: Check discovery status
    Given discovery is enabled in "open-cluster-management"
    When I run "acmlab discovery status"
    Then the output shows namespace, credential, last-active filter, and discovered count

  Scenario: Disable cluster discovery
    Given discovery is enabled in "open-cluster-management"
    When I run "acmlab discovery disable"
    Then the DiscoveryConfig is deleted
    And the credential Secret is deleted
