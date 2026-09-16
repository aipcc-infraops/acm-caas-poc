Feature: Resource quota gates via governance policy (UC-15)

  As a platform operator
  I want to detect and alert when clusters exceed approved resource limits
  So that no team can silently consume resources beyond their quota

  Scenario: Apply worker node quota to a cluster
    Given a cluster "spoke1" is registered in ACM
    When I run "acmlab policy apply-quota --cluster spoke1 --max-workers 5"
    Then the ManagedCluster "spoke1" has label "caas/max-workers=5"
    And a ConfigurationPolicy "spoke1-quota" is created
    And the policy monitors node count against the label

  Scenario: Detect non-compliant worker count
    Given cluster "spoke1" has label "caas/max-workers=3"
    And the cluster has 5 worker nodes
    When the governance policy evaluates
    Then the cluster is marked NonCompliant
    And the owner is notified

  Scenario: Check quota compliance status
    Given a quota policy is applied to "spoke1"
    When I run "acmlab policy quota-status spoke1"
    Then the output shows the quota limits and compliance state
