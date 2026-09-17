Feature: GitOps fleet deployment via ApplicationSet
  As a platform operator
  I want to deploy applications across the fleet using GitOps
  So that spoke clusters receive consistent configurations from a Git repository

  Scenario: Create an ApplicationSet with placement generator
    Given ACM hub is connected
    And OpenShift GitOps is installed on the hub
    When I create an ApplicationSet "monitoring-stack" from repo "https://github.com/example/monitoring.git" path "manifests/base"
    Then an ApplicationSet resource is created in the openshift-gitops namespace
    And the generator targets clusters via ACM Placement

  Scenario: Create an ApplicationSet with cluster generator
    Given ACM hub is connected
    When I create an ApplicationSet "logging" with generator "cluster" and label "env=prod"
    Then the ApplicationSet uses the Argo CD cluster generator
    And only clusters with label env=prod are targeted

  Scenario: List ApplicationSets
    Given ApplicationSets "app1" and "app2" exist
    When I list all ApplicationSets
    Then I see both ApplicationSets with their sync status and app count

  Scenario: Delete an ApplicationSet
    Given ApplicationSet "monitoring-stack" exists
    When I delete the ApplicationSet "monitoring-stack"
    Then the ApplicationSet is removed
    And Argo CD cleans up generated Application resources

  Scenario: Trigger sync on an ApplicationSet
    Given ApplicationSet "monitoring-stack" exists
    When I trigger a sync on "monitoring-stack"
    Then a refresh annotation is added
    And Argo CD re-evaluates the generators

  Scenario: Idempotent creation
    Given ApplicationSet "monitoring-stack" already exists
    When I create the same ApplicationSet again
    Then no error occurs
    And the existing ApplicationSet is unchanged
