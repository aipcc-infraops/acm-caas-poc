Feature: UC-47 Add-On Lifecycle Management
  As a platform engineer
  I want to manage ACM add-on configurations across my fleet
  So that add-ons are consistently configured and maintainable

  Scenario: List available add-ons
    Given the hub cluster is available
    When I run "acmlab addon list"
    Then the output should show registered ClusterManagementAddOns

  Scenario: Get add-on details
    Given ClusterManagementAddOn "work-manager" exists
    When I run "acmlab addon get work-manager"
    Then the output should show the add-on display name, description, and status

  Scenario: Configure an add-on with custom values
    Given the hub cluster is available
    When I run "acmlab addon configure observability-config --set replica-count=3 --set log-level=debug"
    Then an AddOnDeploymentConfig "observability-config" should be created with the specified values

  Scenario: Remove an add-on configuration
    Given AddOnDeploymentConfig "observability-config" exists
    When I run "acmlab addon remove-config observability-config"
    Then the configuration should be deleted

  Scenario: List add-on configurations
    Given AddOnDeploymentConfig resources exist
    When I run "acmlab addon list-configs"
    Then the output should show all configurations with their namespaces
