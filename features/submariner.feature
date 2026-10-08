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

  Scenario: Test connectivity fails preflight when broker config not applied
    Given Submariner add-ons exist on "spoke1" and "spoke2"
    And the add-on on "spoke1" has SubmarinerBrokerConfigApplied=False
    When I run "acmlab submariner test-connectivity spoke1 spoke2"
    Then the phase is "PreflightFailed"
    And the message mentions broker config and cloud credentials

  Scenario: Test connectivity fails preflight when agent degraded
    Given Submariner add-ons exist on "spoke1" and "spoke2"
    And the add-on on "spoke1" has SubmarinerAgentDegraded=True
    When I run "acmlab submariner test-connectivity spoke1 spoke2"
    Then the phase is "PreflightFailed"
    And the message mentions degraded agent

  Scenario: Enable Submariner looks up cloud credentials from ClusterDeployment
    Given a ClusterDeployment exists for "spoke1" with IBM Cloud credentials secret "spoke1-ibm-creds"
    When I enable Submariner for ClusterSet "prod-set"
    Then the SubmarinerConfig credentialsSecret references "spoke1-ibm-creds"
    And the add-on controller can provision gateway nodes and configure the broker

  Scenario: Re-enable with Globalnet updates existing SubmarinerConfig
    Given Submariner is enabled for ClusterSet "prod-set" without Globalnet
    When I run "acmlab submariner enable prod-set --globalnet"
    Then the SubmarinerConfig is updated with a globalCIDR
    And the existing SubmarinerConfig is not silently skipped

  Scenario: Disable waits for addon cleanup before returning
    Given Submariner is enabled for ClusterSet "prod-set"
    When I disable Submariner for ClusterSet "prod-set"
    Then the command waits until all ManagedClusterAddOn resources are deleted
    And re-enabling immediately after disable does not encounter stale state

  Scenario: Disable warns when addon cleanup times out
    Given Submariner is enabled for ClusterSet "prod-set"
    And the ManagedClusterAddOn resources are slow to delete
    When I disable Submariner for ClusterSet "prod-set"
    Then the command returns an error about cleanup timeout
    And the message advises waiting before re-enabling

  Scenario: Diagnose checks credentials secret existence
    Given Submariner is enabled for ClusterSet "prod-set"
    And the SubmarinerConfig on "spoke1" references a credentials secret that does not exist
    When I run "acmlab submariner diagnose prod-set"
    Then a check "config/spoke1/credentials" reports "fail"
    And the message explains the add-on controller needs cloud credentials

  Scenario: Enable creates Broker CR in broker namespace
    Given a ManagedClusterSet "prod-set" with clusters "spoke1" and "spoke2"
    When I enable Submariner for ClusterSet "prod-set"
    Then a Broker CR "submariner-broker" exists in namespace "prod-set-broker"
    And the Broker CR has components "service-discovery" and "connectivity"

  Scenario: Create test set creates Broker CR
    When I run "acmlab submariner create-test-set uc26-test --clusters spoke1,spoke2 --confirm"
    Then a ManagedClusterSet "uc26-test" is created
    And a Broker CR "submariner-broker" exists in namespace "uc26-test-broker"

  Scenario: Diagnose detects missing Broker CR
    Given Submariner is enabled for ClusterSet "prod-set"
    And no Broker CR exists in namespace "prod-set-broker"
    When I run "acmlab submariner diagnose prod-set"
    Then a check "broker-cr" reports "fail"
    And the message suggests running "acmlab submariner repair"

  Scenario: Diagnose warns about IBM Cloud UDP LoadBalancer
    Given Submariner is enabled for ClusterSet "ibm-set"
    And the SubmarinerConfig on "ibm1" has loadBalancerEnable=true
    And the ClusterDeployment on "ibm1" uses platform "ibmcloud"
    When I run "acmlab submariner diagnose ibm-set"
    Then a check "config/ibm1/ibm-udp" reports "warn"
    And the message warns about IBM Cloud UDP LoadBalancer rejection

  Scenario: Enable with force-udp-encaps
    Given a ManagedClusterSet "nat-set" with clusters "spoke1" and "spoke2"
    When I run "acmlab submariner enable nat-set --force-udp-encaps"
    Then each SubmarinerConfig includes forceUDPEncaps=true

  Scenario: Enable with load-balancer
    Given a ManagedClusterSet "lb-set" with clusters "spoke1" and "spoke2"
    When I run "acmlab submariner enable lb-set --load-balancer"
    Then each SubmarinerConfig includes loadBalancerEnable=true

  Scenario: Repair detects and fixes stuck addon finalizers
    Given Submariner was disabled for ClusterSet "stuck-set"
    And ManagedClusterAddOn/submariner on "spoke1" has a deletionTimestamp with stuck finalizers
    When I run "acmlab submariner repair stuck-set"
    Then the stuck finalizers are removed from the addon
    And the action status is "fixed"

  Scenario: Repair detects and creates missing Broker CR
    Given no Broker CR exists in namespace "missing-broker-broker"
    When I run "acmlab submariner repair missing-broker"
    Then a Broker CR "submariner-broker" is created in namespace "missing-broker-broker"
    And the action status is "fixed"

  Scenario: Repair detects duplicate ManagedClusterSet finalizers
    Given ManagedClusterSet "dup-set" has duplicate "submariner-cleanup" finalizers
    When I run "acmlab submariner repair dup-set"
    Then the duplicate finalizers are deduplicated
    And the action status is "fixed"

  Scenario: Repair dry-run previews without changes
    Given ManagedClusterAddOn/submariner on "spoke1" has stuck finalizers
    When I run "acmlab submariner repair stuck-set --dry-run"
    Then the action status is "would-fix"
    And the addon finalizers are not modified

  Scenario: Disable reports stuck clusters in error message
    Given Submariner is enabled for ClusterSet "prod-set"
    And the ManagedClusterAddOn resources have stuck finalizers
    When I disable Submariner for ClusterSet "prod-set"
    Then the error message lists the stuck clusters
    And the message suggests running "acmlab submariner repair"

  Scenario: Enable auto-detects IBM Cloud and sets forceUDPEncaps
    Given a ManagedClusterSet "ibm-set" with IBM Cloud clusters "ibm1" and "ibm2"
    When I run "acmlab submariner enable ibm-set"
    Then each SubmarinerConfig includes forceUDPEncaps=true
    And the log includes "IBM Cloud detected"

  Scenario: Enable does not auto-set forceUDPEncaps when load-balancer requested
    Given a ManagedClusterSet "ibm-set" with IBM Cloud clusters "ibm1" and "ibm2"
    When I run "acmlab submariner enable ibm-set --load-balancer"
    Then each SubmarinerConfig includes loadBalancerEnable=true
    And forceUDPEncaps is not auto-set

  Scenario: SubmarinerConfig includes NATTDiscoveryPort
    When I enable Submariner for ClusterSet "prod-set"
    Then each SubmarinerConfig includes NATTDiscoveryPort=4490

  Scenario: Diagnose shows gateway endpoints when connections fail
    Given Submariner is enabled for ClusterSet "prod-set"
    And connections are degraded with reason "ConnectionsNotEstablished"
    And gateway endpoints exist in the broker namespace
    When I run "acmlab submariner diagnose prod-set"
    Then a check "endpoint/spoke1" reports gateway IP and port information
    And a check "endpoint/spoke2" reports gateway IP and port information

  Scenario: Diagnose hides endpoints when connections are healthy
    Given Submariner is enabled for ClusterSet "prod-set"
    And all connections are healthy
    When I run "acmlab submariner diagnose prod-set"
    Then no "endpoint/*" checks appear in the output

  Scenario: Diagnose shows IBM Cloud firewall requirements on connection failure
    Given Submariner is enabled for ClusterSet "ibm-set"
    And connections are degraded on IBM Cloud clusters
    When I run "acmlab submariner diagnose ibm-set"
    Then a check "firewall/ibm1" reports "fail"
    And the message includes UDP ports 4500, 4490, and 500
    And the message includes ibmcloud CLI command for security group rules

  Scenario: Diagnose no firewall check when connections are healthy
    Given Submariner is enabled for ClusterSet "ibm-set"
    And all connections are healthy on IBM Cloud clusters
    When I run "acmlab submariner diagnose ibm-set"
    Then no "firewall/*" checks appear in the output
