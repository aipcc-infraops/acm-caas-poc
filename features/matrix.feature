Feature: UC-23 Multi-Architecture Cluster Matrix
  As a QA engineer
  I want to provision a matrix of clusters covering all OCP version, architecture, and operator version combinations
  So that I can validate compatibility across the entire support matrix

  Background:
    Given an ACM 2.17 hub cluster is available
    And ClusterImageSets exist for the target OCP versions

  Scenario: Provision a full matrix
    Given a matrix spec with versions "4.18,4.19", architectures "amd64,arm64", and operator versions "2.5"
    When I run "acmlab matrix provision --versions 4.18,4.19 --archs amd64,arm64 --operators 2.5"
    Then 4 ClusterDeployments should be created
    And each ClusterDeployment should have labels "ocp-version", "arch", and "ai-platform-version"
    And each ClusterDeployment should have label "matrix=true"
    And a namespace should exist for each matrix cell

  Scenario: Check matrix status
    Given a matrix with ID "1726574400" has been provisioned
    When I run "acmlab matrix status 1726574400"
    Then the output should show total, ready, and failed counts
    And each cluster cell should display its provisioning status

  Scenario: Destroy a matrix
    Given a matrix with ID "1726574400" exists with 4 clusters
    When I run "acmlab matrix destroy 1726574400"
    Then all 4 ClusterDeployments should be marked for deletion

  Scenario: List matrix clusters
    Given matrix clusters exist with label "matrix=true"
    When I run "acmlab matrix list"
    Then the output should show name, OCP version, architecture, and operator version for each cell

  Scenario: Provision with three-dimensional matrix
    Given a matrix spec with versions "4.18,4.19", architectures "amd64,arm64,s390x", and operator versions "2.5,3.0"
    When I run "acmlab matrix provision --versions 4.18,4.19 --archs amd64,arm64,s390x --operators 2.5,3.0"
    Then 12 ClusterDeployments should be created

  Scenario: Empty matrix spec rejected
    When I run "acmlab matrix provision --versions '' --archs amd64 --operators 2.5"
    Then the command should fail with an error about empty spec values
