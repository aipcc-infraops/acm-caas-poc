Feature: UC-26 Multi-cluster networking via Submariner

  As a platform engineer
  I want to enable cross-cluster networking via Submariner
  So that pods across different managed clusters can communicate directly

  Background:
    Given a hub cluster running ACM 2.17
    And a ManagedClusterSet "prod-set" with clusters "spoke1" and "spoke2"

  Scenario: Enable Submariner for a ClusterSet
    When I enable Submariner for ClusterSet "prod-set"
    Then a ManagedClusterAddOn "submariner" is created in each cluster namespace
    And a SubmarinerConfig is created in each cluster namespace
    And each cluster is labelled "submariner=enabled"
    And the SubmarinerConfig uses cableDriver "libreswan"

  Scenario: Disable Submariner for a ClusterSet
    Given Submariner is enabled for ClusterSet "prod-set"
    When I disable Submariner for ClusterSet "prod-set"
    Then the ManagedClusterAddOn "submariner" is removed from each cluster namespace
    And the SubmarinerConfig is removed from each cluster namespace

  Scenario: Check Submariner connectivity status
    Given Submariner is enabled for ClusterSet "prod-set"
    And all clusters have gateway nodes labelled and agents running
    When I check the status of ClusterSet "prod-set"
    Then the status shows "connected" with all gateways ready
    And each cluster reports at least 1 active connection

  Scenario: Status shows disconnected when addon is missing
    Given Submariner has not been deployed to ClusterSet "prod-set"
    When I check the status of ClusterSet "prod-set"
    Then the status shows "disconnected"

  Scenario: List Submariner-enabled cluster sets
    Given Submariner is enabled for ClusterSet "prod-set"
    When I list all Submariner deployments
    Then the result includes ClusterSet "prod-set" with 2 clusters

  Scenario: Enable Submariner fails for empty ClusterSet
    Given ClusterSet "empty-set" has no clusters
    When I try to enable Submariner for ClusterSet "empty-set"
    Then the operation fails with "no clusters found"

  Scenario: Enable Submariner across three clusters
    Given a ManagedClusterSet "multi-set" with clusters "c1", "c2", and "c3"
    When I enable Submariner for ClusterSet "multi-set"
    Then a ManagedClusterAddOn "submariner" is created in each of the 3 cluster namespaces
    And a SubmarinerConfig is created in each of the 3 cluster namespaces

  Scenario: Status shows connection degraded details
    Given Submariner is enabled for ClusterSet "prod-set"
    And the add-on condition SubmarinerConnectionDegraded is True with reason "ConnectionsNotEstablished"
    When I run "acmlab submariner status prod-set"
    Then the status shows "DISCONNECTED"
    And each cluster shows connectionDegraded true with reason and message

  Scenario: Diagnose identifies missing gateway connections
    Given Submariner is enabled for ClusterSet "prod-set"
    And gateway nodes are labelled but no connections are established
    When I run "acmlab submariner diagnose prod-set"
    Then a check "addon/spoke1/connections" reports "fail"
    And the message includes "CIDR overlap, Globalnet, firewall"

  Scenario: Diagnose warns about local-cluster in set
    Given ClusterSet "default" contains "spoke1", "spoke2", and "local-cluster"
    When I run "acmlab submariner diagnose default"
    Then a check "cluster-set" reports "warn"
    And the message includes "local-cluster (hub) is in this set"

  Scenario: Test connectivity without spoke kubeconfigs
    Given Submariner add-ons exist on "spoke1" and "spoke2"
    When I run "acmlab submariner test-connectivity spoke1 spoke2"
    Then the phase is "SpokeAccessRequired"
    And the message includes "--kubeconfig-a" and "--kubeconfig-b"

  Scenario: Test connectivity reports harness failure on image pull error
    Given Submariner add-ons exist on "spoke1" and "spoke2"
    And the server pod fails with reason "SignatureValidationFailed"
    When I run "acmlab submariner test-connectivity spoke1 spoke2 --kubeconfig-a /tmp/a.kc --kubeconfig-b /tmp/b.kc"
    Then the phase is "HarnessFailed"
    And the message distinguishes harness failure from connectivity failure

  Scenario: Test connectivity creates ServiceExport
    Given Submariner add-ons exist on "spoke1" and "spoke2"
    When I run a connectivity test with spoke kubeconfigs
    Then a ServiceExport "submariner-test-svc" is created on cluster B
    And the client pod resolves the service via clusterset.local DNS

  Scenario: Test connectivity with cleanup disabled
    Given a connectivity test completes with "--cleanup=false"
    Then the output includes manual cleanup instructions
    And test resources remain in the namespace on both clusters

  Scenario: Create a dedicated test ClusterSet
    When I run "acmlab submariner create-test-set uc26-test --clusters caas-pool-1,caas-pool-2 --confirm"
    Then a ManagedClusterSet "uc26-test" is created
    And clusters "caas-pool-1" and "caas-pool-2" are relabelled to set "uc26-test"

  Scenario: Create test set requires confirmation
    When I run "acmlab submariner create-test-set uc26-test --clusters caas-pool-1,caas-pool-2"
    Then the operation fails with "pass --confirm to proceed"

  Scenario: Enable Submariner with wait for readiness
    Given a ManagedClusterSet "prod-set" with clusters "spoke1" and "spoke2"
    When I run "acmlab submariner enable prod-set --wait --timeout 10m"
    Then Submariner resources are created for all clusters in the set
    And the command polls until all clusters show connected status
    And the command reports "Submariner is connected and ready"

  Scenario: Enable Submariner with Globalnet for overlapping CIDRs
    Given a ManagedClusterSet "overlap-set" with clusters using identical Pod/Service CIDRs
    When I run "acmlab submariner enable overlap-set --globalnet"
    Then each SubmarinerConfig includes a unique globalCIDR from the 242.0.0.0/8 range
    And Submariner uses Globalnet to route traffic between clusters

  Scenario: Diagnose warns about default ClusterSet
    Given Submariner is enabled for ClusterSet "default"
    When I run "acmlab submariner diagnose default"
    Then a check "cluster-set/naming" reports "warn"
    And the message suggests creating a dedicated ClusterSet

  Scenario: Diagnose recommends Globalnet when connections fail
    Given Submariner is enabled for ClusterSet "prod-set"
    And connections are degraded with reason "ConnectionsNotEstablished"
    And no SubmarinerConfig has a globalCIDR set
    When I run "acmlab submariner diagnose prod-set"
    Then a check "globalnet" reports "warn"
    And the message recommends enabling Globalnet or reprovisioning with non-overlapping CIDRs

  Scenario: Test connectivity fails preflight when addon not available
    Given Submariner add-ons exist on "spoke1" and "spoke2"
    And the add-on on "spoke2" has Available=False
    When I run "acmlab submariner test-connectivity spoke1 spoke2"
    Then the phase is "PreflightFailed"
    And the message suggests running "acmlab submariner diagnose"

  Scenario: Test connectivity fails preflight when ServiceExport API missing
    Given Submariner add-ons exist on "spoke1" and "spoke2"
    And cluster "spoke2" does not have the ServiceExport CRD installed
    When I run a connectivity test with spoke kubeconfigs
    Then the phase is "PreflightFailed"
    And the message explains the ServiceExport API is not available
