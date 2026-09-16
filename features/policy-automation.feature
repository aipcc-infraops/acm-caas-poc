Feature: Policy Automation via Ansible (UC-30)
  As a platform operator
  I want to link governance policies to Ansible job templates
  So that policy violations are automatically remediated

  Scenario: Create policy automation
    Given a managed cluster "spoke1" exists
    And a governance policy "image-policy" is applied
    When I create a PolicyAutomation "auto-remediate" for policy "image-policy"
    Then the PolicyAutomation should reference "image-policy"
    And the automation mode should be "scan"

  Scenario: List policy automations
    Given a PolicyAutomation "auto-remediate" exists
    When I list all PolicyAutomations
    Then the list should contain "auto-remediate"

  Scenario: Update automation mode
    Given a PolicyAutomation "auto-remediate" exists with mode "scan"
    When I set the mode to "disabled"
    Then the PolicyAutomation mode should be "disabled"

  Scenario: Delete policy automation
    Given a PolicyAutomation "auto-remediate" exists
    When I delete PolicyAutomation "auto-remediate"
    Then the PolicyAutomation should no longer exist

  Scenario: Idempotent creation
    Given a PolicyAutomation "auto-remediate" exists
    When I create the same PolicyAutomation again
    Then no error should occur
    And only one PolicyAutomation should exist
