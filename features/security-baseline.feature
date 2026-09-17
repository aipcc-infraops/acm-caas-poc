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

  Scenario: Deploy a Gatekeeper policy from Rego file
    Given a cluster "spoke1" is registered in ACM
    And a file "deny-latest.rego" contains a valid Rego policy with package "deny_latest"
    When I run "acmlab security apply-policy --cluster spoke1 --policy-file deny-latest.rego"
    Then the engine is auto-detected as "gatekeeper" from the .rego extension
    And a ManifestWork "custom-rego-deny_latest-spoke1" is created in namespace "spoke1"
    And the ManifestWork contains a ConstraintTemplate with the Rego source
    And a Constraint instance matching Pods is created

  Scenario: Deploy Gatekeeper policy with explicit name and match kinds
    Given a cluster "spoke1" is registered in ACM
    When I run "acmlab security apply-policy --cluster spoke1 --policy-file no-hostpath.rego --name no-hostpath --match Pod,Deployment"
    Then the ManifestWork is named "custom-rego-no-hostpath-spoke1"
    And the Constraint matches both Pod and Deployment kinds

  Scenario: Deploy a Kyverno policy from YAML file
    Given a cluster "spoke1" is registered in ACM
    And a file "disallow-latest.yaml" contains a valid Kyverno ClusterPolicy
    When I run "acmlab security apply-policy --cluster spoke1 --policy-file disallow-latest.yaml"
    Then the engine is auto-detected as "kyverno" from the .yaml extension
    And a ManifestWork "kyverno-policy-disallow-latest-spoke1" is created in namespace "spoke1"
    And the ManifestWork wraps the Kyverno ClusterPolicy

  Scenario: Deploy Kyverno policy with explicit engine flag
    Given a cluster "spoke1" is registered in ACM
    When I run "acmlab security apply-policy --cluster spoke1 --policy-file gpu-limits.yaml --name gpu-limits --engine kyverno"
    Then the ManifestWork is named "kyverno-policy-gpu-limits-spoke1"

  Scenario: List all custom policies across engines
    Given Gatekeeper policies are deployed to "spoke1"
    And Kyverno policies are deployed to "spoke2"
    When I run "acmlab security list-policies"
    Then the output shows engine, cluster, policy name, and status for all policies

  Scenario: List policies filtered by engine
    When I run "acmlab security list-policies --engine kyverno"
    Then only Kyverno policies are shown

  Scenario: Remove a Gatekeeper policy
    Given a Gatekeeper policy "deny_latest" is deployed to "spoke1"
    When I run "acmlab security remove-policy deny_latest --cluster spoke1 --engine gatekeeper"
    Then the ManifestWork is deleted from namespace "spoke1"

  Scenario: Remove a Kyverno policy
    Given a Kyverno policy "disallow-latest" is deployed to "spoke1"
    When I run "acmlab security remove-policy disallow-latest --cluster spoke1 --engine kyverno"
    Then the ManifestWork is deleted from namespace "spoke1"

  Scenario: Reject invalid Rego without package declaration
    When I run "acmlab security apply-policy --cluster spoke1 --policy-file bad.rego"
    And the Rego file has no "package" declaration
    Then the command returns an error about missing package name

  Scenario: Gatekeeper and Kyverno coexist on the same cluster
    Given CIS Level 1 baseline is applied to "spoke1" via Gatekeeper
    And a Kyverno policy "gpu-resource-limits" is deployed to "spoke1"
    Then both admission webhooks operate independently
    And Gatekeeper handles CIS compliance while Kyverno handles GPU workload policies
