Feature: Security baseline enforcement via Gatekeeper/OPA (UC-29)

  As a platform operator
  I want to enforce CIS Kubernetes security controls on GPU clusters
  So that workloads comply with pod security standards without manual auditing

  Scenario: Apply CIS Level 1 baseline to a cluster
    Given a cluster "spoke1" is registered in ACM
    When I run "acmlab security apply cis-level1 --cluster spoke1"
    Then a ManifestWork "security-baseline-spoke1" is created in namespace "spoke1"
    And Gatekeeper ConstraintTemplates are deployed to the spoke
    And a ConfigurationPolicy verifies Gatekeeper is healthy

  Scenario: Constraints enforce pod security
    Given CIS Level 1 baseline is applied to "spoke1"
    When a privileged pod is submitted
    Then Gatekeeper denies the admission request
    And ACM reports the constraint violation

  Scenario: List security baselines across the fleet
    Given baselines are applied to "spoke1" and "spoke2"
    When I run "acmlab security list"
    Then the output shows each cluster with level and status

  Scenario: Remove security baseline from a cluster
    Given CIS Level 1 baseline is applied to "spoke1"
    When I run "acmlab security remove spoke1"
    Then the ManifestWork is deleted
    And Gatekeeper constraints are removed from the spoke
    And the health policy resources are cleaned up
