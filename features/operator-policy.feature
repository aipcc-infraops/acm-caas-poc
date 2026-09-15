Feature: Operator version pinning via OperatorPolicy
  As a platform operator
  I want to pin operator versions across the fleet
  So that clusters run validated operator releases and do not auto-upgrade

  Background:
    Given the ACM hub is reachable
    And I have a dynamic client for the hub

  Scenario: Pin an operator to a specific version
    When I apply an OperatorPolicy "pin-gpu" for operator "gpu-sharing-operator" version "2.17.3" channel "stable-2.17"
    Then a Policy "pin-gpu" is created in the global-set namespace
    And the policy-template kind is "OperatorPolicy"
    And the OperatorPolicy subscription name is "gpu-sharing-operator"
    And the OperatorPolicy versions include "2.17.3"
    And the OperatorPolicy channel is "stable-2.17"

  Scenario: Pin operator without version constraint
    When I apply an OperatorPolicy "track-gpu" for operator "gpu-sharing-operator" with channel "stable-2.17" only
    Then a Policy "track-gpu" is created
    And the OperatorPolicy has no versions constraint
    And the OperatorPolicy channel is "stable-2.17"

  Scenario: Apply is idempotent
    Given an OperatorPolicy "pin-gpu" already exists
    When I apply the same OperatorPolicy again
    Then no error is returned
    And the policy remains unchanged

  Scenario: Remove operator policy
    Given an OperatorPolicy "pin-gpu" exists
    When I remove policy "pin-gpu"
    Then the Policy, Placement, and PlacementBinding are deleted
