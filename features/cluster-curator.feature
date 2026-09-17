Feature: ClusterCurator day-2 hooks (UC-50)

  As a platform operator
  I want to attach pre/post hooks to cluster lifecycle operations
  So that critical tasks run automatically during upgrades and maintenance

  Scenario: Apply ClusterCurator with pre-hook
    Given a cluster "spoke1" exists
    When I run "acmlab lifecycle curator-apply spoke1 --pre-hook backup-etcd --pre-hook-type Job"
    Then a ClusterCurator "spoke1" is created in namespace "spoke1"
    And the pre-hook "backup-etcd" of type "Job" is configured

  Scenario: Apply ClusterCurator with both hooks
    When I run "acmlab lifecycle curator-apply spoke1 --pre-hook backup --post-hook verify-health"
    Then both pre-hook and post-hook are configured

  Scenario: View ClusterCurator status
    Given a ClusterCurator exists for "spoke1"
    When I run "acmlab lifecycle curator-status spoke1"
    Then the output shows hook names, types, and execution status

  Scenario: Remove ClusterCurator
    Given a ClusterCurator exists for "spoke1"
    When I run "acmlab lifecycle curator-remove spoke1"
    Then the ClusterCurator is deleted

  Scenario: List all ClusterCurators
    Given ClusterCurators exist for "spoke1" and "spoke2"
    When I run "acmlab lifecycle curator-list"
    Then the output shows cluster, hooks, and status for each
