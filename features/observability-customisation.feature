Feature: Observability stack customisation (UC-52)

  As a platform operator
  I want to customise the ACM observability stack
  So that I can configure production storage, custom metrics, rules, and dashboards

  Scenario: Configure pull secret for observability
    Given the observability namespace exists
    When I run "acmlab observability configure-pull-secret"
    Then the pull secret is copied from openshift-config to the observability namespace
    And the secret is named "multiclusterhub-operator-pull-secret"

  Scenario: Configure ObjectBucketClaim storage
    When I run "acmlab observability configure-storage --storage-class gp3-csi --bucket thanos"
    Then an ObjectBucketClaim "observability-obc" is created in the observability namespace
    And the OBC references the specified storage class

  Scenario: Deploy custom Prometheus rules
    Given a YAML file with custom recording and alerting rules
    When I run "acmlab observability deploy-rules --rules-file custom-rules.yaml"
    Then a ConfigMap "thanos-rule-custom-rules" is created in the observability namespace
    And the rules are stored under the "custom_rules.yaml" key

  Scenario: Remove custom Prometheus rules
    Given custom rules are deployed
    When I run "acmlab observability remove-rules"
    Then the ConfigMap "thanos-rule-custom-rules" is deleted

  Scenario: Deploy a custom Grafana dashboard
    Given a JSON file with a Grafana dashboard definition
    When I run "acmlab observability deploy-dashboard --name gpu-overview --dashboard-file gpu-dashboard.json"
    Then a ConfigMap "gpu-overview" is created with label "grafana-custom-dashboard=true"
    And the dashboard JSON is stored under the "gpu-overview.json" key

  Scenario: Remove a custom Grafana dashboard
    When I run "acmlab observability remove-dashboard gpu-overview"
    Then the ConfigMap "gpu-overview" is deleted

  Scenario: Configure custom metrics allowlist
    When I run "acmlab observability configure-metrics --metric node_cpu_seconds_total --metric container_memory_rss"
    Then a ConfigMap "observability-metrics-custom-allowlist" is created
    And the metrics are listed in the "metrics_list.yaml" key

  Scenario: Show observability addon health
    When I run "acmlab observability addon-health"
    Then the output shows cluster, availability, and degraded status for each spoke

  Scenario: Configure Thanos retention
    When I run "acmlab observability configure-retention --retention 24h --block-duration 2h --delete-delay 48h"
    Then the MCO spec.retentionConfig is updated with the specified values

  Scenario: Full observability stack setup with production storage
    Given I run "acmlab observability setup"
    And I run "acmlab observability configure-pull-secret"
    And I run "acmlab observability configure-storage --storage-class gp3-csi"
    When I run "acmlab observability status"
    Then the observability stack is ready with production OBC storage
