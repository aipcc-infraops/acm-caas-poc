@core
Feature: Cluster resource monitoring from the hub
  As a platform operator
  I want to query cluster resource usage and node status
  So that the ComputeRequest controller can make capacity-aware decisions

  Background:
    Given the ACM hub is reachable
    And I have a dynamic client for the hub

  Scenario: Read node count and resource capacity for a cluster
    Given a managed cluster "spoke1" is joined and available
    When I get the cluster resources for "spoke1"
    Then I receive node count, CPU capacity, and memory capacity

  Scenario: List resource usage across the fleet
    When I list cluster resources for all managed clusters
    Then I receive resource summaries for each cluster
    And each summary includes node count and capacity
