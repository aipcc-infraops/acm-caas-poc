Feature: UC-44 Agent GitOps for Disconnected Clusters
  As a platform engineer
  I want to deploy applications to disconnected clusters via agent mode
  So that edge environments receive GitOps without direct hub connectivity

  Scenario: Enable agent-mode GitOps on disconnected clusters
    Given the hub cluster is available
    When I run "acmlab gitops enable-agent edge-apps --repo https://github.com/example/edge-manifests.git --path manifests/edge --cluster edge-01 --cluster edge-02"
    Then an ApplicationSet "edge-apps" should be created with PullMode=true annotation
    And the ApplicationSet should use a list generator for the specified clusters

  Scenario: Check agent-mode status
    Given an agent-mode ApplicationSet "edge-apps" exists
    When I run "acmlab gitops agent-status edge-apps"
    Then the output should show mode "pull" and sync status

  Scenario: Disable agent-mode GitOps
    Given an agent-mode ApplicationSet "edge-apps" exists
    When I run "acmlab gitops disable-agent edge-apps"
    Then the ApplicationSet "edge-apps" should be removed

  Scenario: Reject disable on non-agent ApplicationSet
    Given a standard ApplicationSet "regular-app" exists
    When I run "acmlab gitops disable-agent regular-app"
    Then the command should fail with "not an agent-mode ApplicationSet"
