Feature: UC-45 Resource Right-Sizing with Advise and Adjust Modes
  As a platform engineer
  I want to identify over-provisioned workloads and optionally adjust their resources
  So that clusters run efficiently without manual analysis

  Scenario: Enable right-sizing monitoring on a cluster
    Given the hub cluster is available
    When I run "acmlab rightsizing enable spoke1"
    Then a ManifestWork deploying MCOA PrometheusRules should be created on spoke1

  Scenario: Get right-sizing recommendations (advise mode)
    Given right-sizing is enabled on spoke1
    When I run "acmlab rightsizing advise spoke1"
    Then the output should show workload recommendations with current and recommended CPU/memory values

  Scenario: Apply right-sizing adjustments (adjust mode)
    Given right-sizing is enabled on spoke1
    When I run "acmlab rightsizing adjust spoke1"
    Then ManifestWorks should be created to patch workload resources on spoke1

  Scenario: Dry-run right-sizing adjustments
    Given right-sizing is enabled on spoke1
    When I run "acmlab rightsizing adjust spoke1 --dry-run"
    Then the output should show proposed changes without applying them

  Scenario: Disable right-sizing monitoring
    Given right-sizing is enabled on spoke1
    When I run "acmlab rightsizing disable spoke1"
    Then the right-sizing ManifestWork should be removed from spoke1

  Scenario: List clusters with right-sizing enabled
    Given right-sizing is enabled on spoke1 and spoke2
    When I run "acmlab rightsizing list"
    Then the output should show both clusters with their monitoring status
