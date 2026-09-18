Feature: Cluster templating via ClusterDeploymentCustomization (UC-53)

  As a platform operator
  I want to define reusable cluster profiles
  So that teams can provision standardised clusters without specifying low-level config

  Scenario: Create a cluster template
    When I run "acmlab provision template-create gpu-large --patch replace:/networking/clusterNetwork/0/cidr:10.128.0.0/14"
    Then a ClusterDeploymentCustomization "gpu-large" is created
    And it contains the specified install config patches

  Scenario: List cluster templates
    Given templates "small", "medium", and "gpu-large" exist
    When I run "acmlab provision template-list"
    Then the output shows all templates with their patch counts

  Scenario: Get template details
    Given template "gpu-large" exists
    When I run "acmlab provision template-get gpu-large"
    Then the output shows the full template specification

  Scenario: Apply a template to a ClusterDeployment
    Given template "gpu-large" exists
    And ClusterDeployment "spoke3" exists
    When I run "acmlab provision template-apply spoke3 --template gpu-large"
    Then the ClusterDeployment is annotated with the template reference

  Scenario: Remove a cluster template
    Given template "old-template" exists
    When I run "acmlab provision template-remove old-template"
    Then the ClusterDeploymentCustomization is deleted
