Feature: ManifestWorkReplicaSet progressive rollout (UC-33)

  As a platform operator
  I want to roll out fleet-wide updates progressively
  So that a bad manifest does not take down all clusters at once

  Scenario: Create a progressive rollout
    Given a Placement "gpu-placement" selects 20 clusters
    When I run "acmlab rollout create kueue-v12 --placement gpu-placement --strategy Progressive --max-concurrency 2 --max-failures 10%"
    Then a ManifestWorkReplicaSet "kueue-v12" is created
    And the rollout strategy is Progressive with maxConcurrency 2
    And maxFailures is set to 10%

  Scenario: Rollout stops on failure threshold
    Given a progressive rollout "kueue-v12" targets 20 clusters
    When more than 10% of clusters report Degraded
    Then the rollout stops automatically
    And remaining clusters are not updated

  Scenario: Check rollout status
    Given a rollout "kueue-v12" is in progress
    When I run "acmlab rollout get kueue-v12"
    Then the output shows applied, total, and failed counts
    And the status reflects the current rollout state

  Scenario: Update rollout strategy mid-flight
    Given a rollout "kueue-v12" uses the All strategy
    When I run "acmlab rollout update-strategy kueue-v12 --strategy Progressive --max-concurrency 3"
    Then the strategy is changed to Progressive
    And subsequent batches use maxConcurrency 3

  Scenario: Delete a rollout
    Given a rollout "kueue-v12" exists
    When I run "acmlab rollout delete kueue-v12"
    Then the ManifestWorkReplicaSet is removed
    And spoke clusters retain their current state
