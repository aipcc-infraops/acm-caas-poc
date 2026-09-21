Feature: ClusterPool and ClusterClaim for pre-warmed clusters (UC-25)

  As a developer or QA engineer
  I want to claim a pre-warmed cluster instantly
  So that I do not wait 10 minutes for provisioning every time I need a cluster

  Scenario: Create a cluster pool
    When I run "acmlab pool create amd64-419 --size 3 --image-set img4.19-multi --platform ibmcloud --region us-south --base-domain example.com"
    Then a ClusterPool "amd64-419" is created with size 3
    And Hive provisions 3 hibernated clusters

  Scenario: List cluster pools
    Given a pool "amd64-419" exists with 3 ready clusters
    When I run "acmlab pool list"
    Then the output shows pool name, size, ready, and claimed counts

  Scenario: Claim a cluster instantly
    Given the "amd64-419" pool has at least 1 ready cluster
    When I run "acmlab claim create amd64-419 --name my-test --ttl 48h"
    Then a ClusterClaim "my-test" is created and bound in seconds
    And Hive provisions a replacement cluster to maintain pool size

  Scenario: Release a claim and observe cluster destruction
    Given I have a claimed cluster "my-test"
    When I run "acmlab claim release my-test"
    Then the ClusterClaim is deleted
    And the claimed cluster is destroyed by Hive
    And Hive provisions a replacement to maintain pool size
