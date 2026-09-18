Feature: Placement tolerations and taints (UC-34)

  As a platform operator
  I want to taint clusters and create tolerant Placements
  So that workloads are only scheduled to appropriate clusters

  Scenario: Add a taint to a managed cluster
    Given a cluster "spoke1" is registered in ACM
    When I run "acmlab fleet add-taint spoke1 --key gpu-reserved --value true --effect NoSchedule"
    Then the ManagedCluster "spoke1" has taint "gpu-reserved=true:NoSchedule"

  Scenario: List taints on a cluster
    Given cluster "spoke1" has taint "gpu-reserved=true:NoSchedule"
    When I run "acmlab fleet list-taints spoke1"
    Then the output shows the taint key, value, and effect

  Scenario: Remove a taint from a cluster
    Given cluster "spoke1" has taint "gpu-reserved=true:NoSchedule"
    When I run "acmlab fleet remove-taint spoke1 --key gpu-reserved"
    Then the taint is removed from the ManagedCluster

  Scenario: Create a Placement with tolerations
    Given cluster "spoke1" is tainted with "gpu-reserved=true:NoSchedule"
    When I run "acmlab fleet create-tolerant-placement ml-workloads --tolerate gpu-reserved=true --cluster-set gpu-clusters"
    Then a Placement "ml-workloads" is created with toleration for "gpu-reserved"
    And only clusters tolerating the taint are eligible for scheduling

  Scenario: Duplicate taint returns error
    Given cluster "spoke1" already has taint "gpu-reserved"
    When I run "acmlab fleet add-taint spoke1 --key gpu-reserved --value true"
    Then the command returns an error about duplicate taint
