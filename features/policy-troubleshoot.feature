Feature: Policy troubleshooting

  As a platform operator
  I want an easy way to troubleshoot policy violations
  So that I can quickly identify and resolve compliance issues

  Scenario: View policy violations
    Given a policy "no-privileged" is NonCompliant on "spoke1"
    When I run "acmlab policy violations no-privileged"
    Then the output shows the violating cluster and violation message

  Scenario: Troubleshoot a policy with violations and events
    Given a policy "no-privileged" is NonCompliant on "spoke1"
    And events exist in the policy namespace
    When I run "acmlab policy troubleshoot no-privileged"
    Then the output shows violations per cluster
    And the output shows recent events from the policy namespace

  Scenario: Troubleshoot a compliant policy
    Given a policy "require-labels" is Compliant on all clusters
    When I run "acmlab policy violations require-labels"
    Then the output shows "No violations found"

  Scenario: JSON output for troubleshoot
    When I run "acmlab policy troubleshoot no-privileged --json"
    Then the output is valid JSON with violations and events arrays
