Feature: Cloud-native cluster discovery (UC-57)

  As a platform operator
  I want to discover unmanaged Kubernetes and OpenShift clusters from cloud providers
  So that I can identify and import them into ACM without relying on OpenShift Cluster Manager

  Scenario: Scan AWS for EKS and ROSA clusters
    Given the AWS CLI is authenticated
    When I run "acmlab discovery scan --provider aws"
    Then the output lists all EKS and ROSA clusters with name, type, region, version, and status
    And clusters already managed by ACM are marked as "managed"

  Scenario: Scan IBM Cloud for IKS and ROKS clusters
    Given the IBM Cloud CLI is authenticated
    When I run "acmlab discovery scan --provider ibmcloud"
    Then the output lists all IKS and ROKS clusters
    And ROKS clusters are identified as type "roks" and IKS as type "iks"

  Scenario: Scan all providers at once
    When I run "acmlab discovery scan"
    Then clusters from all configured providers are listed together
    And providers whose CLI is not installed are skipped with a warning

  Scenario: Filter scan by region
    When I run "acmlab discovery scan --provider aws --region us-east-1"
    Then only clusters in us-east-1 are shown

  Scenario: Auto-import a discovered EKS cluster
    Given an EKS cluster "my-eks" exists in AWS
    When I run "acmlab discovery auto-import my-eks --provider aws"
    Then a ManagedCluster "my-eks" is created with label discovered-from=aws
    And a KlusterletAddonConfig is created in namespace "my-eks"

  Scenario: Auto-import a discovered ROKS cluster
    Given a ROKS cluster "my-roks" exists in IBM Cloud
    When I run "acmlab discovery auto-import my-roks --provider ibmcloud"
    Then a ManagedCluster "my-roks" is created with label cluster-type=roks

  Scenario: Auto-import fails for nonexistent cluster
    When I run "acmlab discovery auto-import nonexistent --provider aws"
    Then the command returns an error "cluster nonexistent not found in aws"

  Scenario: Unsupported provider returns error
    When I run "acmlab discovery scan --provider gcp"
    Then the command returns an error about unsupported provider
