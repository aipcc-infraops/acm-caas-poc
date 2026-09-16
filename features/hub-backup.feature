Feature: Hub backup and restore (UC-36)
  As a platform engineer
  I want to back up and restore the ACM hub state
  So that I can recover from hub failures or migrate to a new hub

  Scenario: Enable hub backup schedule
    Given ACM hub is reachable
    When I enable hub backup with default schedule
    Then a BackupSchedule CR is created in the backup namespace
    And the schedule is set to every 6 hours

  Scenario: Get backup status
    Given hub backup is enabled
    When I check backup status
    Then I see the schedule, last backup time, and phase

  Scenario: Disable hub backup
    Given hub backup is enabled
    When I disable hub backup
    Then the BackupSchedule CR is removed

  Scenario: Restore hub from backup
    Given a completed backup exists
    When I trigger a hub restore
    Then a Restore CR is created with sync mode latest
    And ManagedClusters, credentials, and resources are restored

  Scenario: Enable is idempotent
    Given hub backup is already enabled
    When I enable hub backup again
    Then no error occurs and the existing schedule is preserved
