Feature: Credential-free hub-to-spoke access via ManagedServiceAccount (UC-35)

  As a platform operator
  I want the hub to access spoke clusters without static kubeconfigs
  So that credentials are auto-rotated and never stored manually

  Scenario: Enable managed access on a spoke cluster
    Given a cluster "spoke1" is registered in ACM
    When I run "acmlab access enable spoke1 --ttl 720h"
    Then a ManagedServiceAccount "acmlab-access" is created in namespace "spoke1"
    And a ManagedClusterAddOn "managed-serviceaccount" is created
    And a ManagedClusterAddOn "cluster-proxy" is created
    And the token rotation interval is set to 720h

  Scenario: Verify token availability
    Given managed access is enabled on "spoke1"
    When I run "acmlab access status spoke1"
    Then the output shows TokenAvailable as true
    And the output shows AddonHealthy as true

  Scenario: List clusters with managed access
    Given managed access is enabled on "spoke1" and "spoke2"
    When I run "acmlab access list"
    Then the output shows both clusters with their access status

  Scenario: Disable managed access
    Given managed access is enabled on "spoke1"
    When I run "acmlab access disable spoke1"
    Then the ManagedServiceAccount and addons are removed
    And no static credentials remain on the hub

  Scenario: Enable is idempotent
    Given managed access is already enabled on "spoke1"
    When I run "acmlab access enable spoke1"
    Then no error occurs and resources remain unchanged
