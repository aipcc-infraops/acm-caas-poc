Feature: ManifestWork ordering (UC-55)

  As a platform operator
  I want to deploy manifests in a specific order within a ManifestWork
  So that dependencies (namespace before deployment, CRD before CR) are respected

  Scenario: Create an ordered ManifestWork
    Given a cluster "spoke1" is registered in ACM
    When I run "acmlab workorder create-ordered app-stack --cluster spoke1 --manifests app-manifests.json"
    Then a ManifestWork "app-stack" is created in namespace "spoke1"
    And manifests are ordered by ordinal (namespace first, then config, then deployment)
    And manifestConfigs include resourceIdentifier with ordinal sequencing

  Scenario: Get ordered work status
    Given ordered ManifestWork "app-stack" exists on "spoke1"
    When I run "acmlab workorder get-ordered app-stack --cluster spoke1"
    Then the output shows manifest count and apply status

  Scenario: List ordered works on a cluster
    Given ordered ManifestWorks "app-stack" and "db-stack" exist on "spoke1"
    When I run "acmlab workorder list-ordered --cluster spoke1"
    Then the output shows all ordered works with manifest counts

  Scenario: Remove an ordered work
    Given ordered ManifestWork "app-stack" exists on "spoke1"
    When I run "acmlab workorder remove-ordered app-stack --cluster spoke1"
    Then the ManifestWork is deleted from namespace "spoke1"

  Scenario: Manifests are sorted by ordinal before deployment
    When creating an ordered work with ordinals [2, 0, 1]
    Then the ManifestWork contains manifests reordered as [0, 1, 2]
