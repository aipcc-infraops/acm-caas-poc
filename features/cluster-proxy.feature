Feature: UC-46 Cluster Proxy for Secure Spoke Access
  As a platform engineer
  I want to establish reverse proxy tunnels to spoke clusters
  So that I can access spoke APIs securely from the hub

  Scenario: Enable cluster proxy on a spoke
    Given the hub cluster is available
    When I run "acmlab access enable-proxy spoke1"
    Then a cluster-proxy ManagedClusterAddOn should be created on spoke1

  Scenario: Check proxy status
    Given cluster proxy is enabled on spoke1
    When I run "acmlab access proxy-status spoke1"
    Then the output should show the proxy endpoint and health status

  Scenario: Disable cluster proxy
    Given cluster proxy is enabled on spoke1
    When I run "acmlab access disable-proxy spoke1"
    Then the cluster-proxy addon should be removed from spoke1

  Scenario: Proxy status when not enabled
    Given no proxy is configured on spoke2
    When I run "acmlab access proxy-status spoke2"
    Then the output should show enabled as false
