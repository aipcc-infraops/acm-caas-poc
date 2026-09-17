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

  Scenario: Deploy a custom Rego policy from file
    Given a cluster "spoke1" is registered in ACM
    And a file "deny-latest.rego" contains a valid Rego policy with package "deny_latest"
    When I run "acmlab security apply-rego --cluster spoke1 --rego-file deny-latest.rego"
    Then a ManifestWork "custom-rego-deny_latest-spoke1" is created in namespace "spoke1"
    And the ManifestWork contains a ConstraintTemplate with the Rego source
    And a Constraint instance matching Pods is created

  Scenario: Deploy custom Rego with explicit name and match kinds
    Given a cluster "spoke1" is registered in ACM
    When I run "acmlab security apply-rego --cluster spoke1 --rego-file no-hostpath.rego --name no-hostpath --match Pod,Deployment"
    Then the ManifestWork is named "custom-rego-no-hostpath-spoke1"
    And the Constraint matches both Pod and Deployment kinds

  Scenario: List custom Rego policies
    Given custom Rego policies are deployed to "spoke1" and "spoke2"
    When I run "acmlab security list-rego"
    Then the output shows each cluster with policy name and status

  Scenario: Remove a custom Rego policy
    Given a custom Rego policy "deny_latest" is deployed to "spoke1"
    When I run "acmlab security remove-rego deny_latest --cluster spoke1"
    Then the ManifestWork is deleted from namespace "spoke1"

  Scenario: Reject invalid Rego without package declaration
    When I run "acmlab security apply-rego --cluster spoke1 --rego-file bad.rego"
    And the Rego file has no "package" declaration
    Then the command returns an error about missing package name

  Scenario: Deploy a Kyverno ClusterPolicy from file
    Given a cluster "spoke1" is registered in ACM
    And a file "disallow-latest.yaml" contains a valid Kyverno ClusterPolicy
    When I run "acmlab security apply-kyverno --cluster spoke1 --policy-file disallow-latest.yaml"
    Then a ManifestWork "kyverno-policy-disallow-latest-spoke1" is created in namespace "spoke1"
    And the ManifestWork wraps the Kyverno ClusterPolicy

  Scenario: Deploy Kyverno policy with explicit name
    Given a cluster "spoke1" is registered in ACM
    When I run "acmlab security apply-kyverno --cluster spoke1 --policy-file gpu-limits.yaml --name gpu-limits"
    Then the ManifestWork is named "kyverno-policy-gpu-limits-spoke1"

  Scenario: List Kyverno policies
    Given Kyverno policies are deployed to "spoke1" and "spoke2"
    When I run "acmlab security list-kyverno"
    Then the output shows each cluster with policy name and status

  Scenario: Remove a Kyverno policy
    Given a Kyverno policy "disallow-latest" is deployed to "spoke1"
    When I run "acmlab security remove-kyverno disallow-latest --cluster spoke1"
    Then the ManifestWork is deleted from namespace "spoke1"

  Scenario: Gatekeeper and Kyverno coexist on the same cluster
    Given CIS Level 1 baseline is applied to "spoke1" via Gatekeeper
    And a Kyverno policy "gpu-resource-limits" is deployed to "spoke1"
    Then both admission webhooks operate independently
    And Gatekeeper handles CIS compliance while Kyverno handles GPU workload policies
