Feature: Cost tracking and chargeback via node metadata estimation (UC-11)
  As a platform operator
  I want to estimate cluster costs from ManagedClusterInfo node metadata
  So that teams can be charged for their compute consumption

  Background:
    Given the ACM hub is reachable
    And I have a dynamic client for the hub

  Scenario: Get cost estimate for a single cluster
    Given a ManagedCluster "spoke-test" exists with node metadata
    When I request the cost estimate for "spoke-test"
    Then I receive an estimated hourly and monthly cost based on instance types
    And the estimate includes node count, CPU hours, and memory hours

  Scenario: Generate a fleet-wide cost report
    Given multiple ManagedClusters exist with node metadata
    When I generate a cost report for the fleet
    Then I receive a report with per-cluster cost estimates
    And the report defaults to human-readable text output
    And "--format json" exports the report as JSON
    And "--format csv" exports the report with header "name,nodes,cpuCores,memoryGiB,dailyEstimate,periodEstimate,days"
    And "--json" is equivalent to "--format json"
    And an unsupported "--format xml" returns a clear validation error

  Scenario: Deploy cost dashboard to Grafana via ACM observability
    Given ACM multicluster observability is installed on the hub
    When I run "acmlab cost dashboard"
    Then a custom metrics allowlist is configured for cost-related metrics
    And recording rules use kube_node_role to filter worker nodes only
    And Prometheus recording rules compute per-cluster daily and monthly estimates
    And a Grafana dashboard "acmlab-cost-tracking" is deployed as a ConfigMap
    And the Grafana URL is printed for direct access

  Scenario: Remove cost dashboard from Grafana
    Given the cost tracking dashboard has been deployed
    When I run "acmlab cost remove-dashboard"
    Then the Grafana dashboard ConfigMap is removed
    And only the acmlab cost recording rule group is removed
    And other custom rule groups are preserved

  Scenario: Identify clusters with no cost center
    Given a ManagedCluster "spoke-test" exists without a cost center label
    When I generate a cost report
    Then "spoke-test" appears with cost center "unassigned"
