@templating
Feature: Cluster templating via ClusterDeploymentCustomization (UC-53)

  As a platform operator
  I want to define reusable cluster profiles (small, medium, large)
  So that teams can provision standardised clusters without specifying low-level config

  Background:
    Given a hub cluster is available
    And the Hive API is accessible

  # --- Create ---

  Scenario: Create a cluster template with a single patch
    When I run "acmlab provision template-create small-profile --namespace hive --patch replace:/compute/0/replicas:2"
    Then a ClusterDeploymentCustomization "small-profile" is created in namespace "hive"
    And it contains 1 installConfigPatch
    And the patch has op "replace", path "/compute/0/replicas", value "2"
    And the resource is labelled "acmlab.redhat.com/cluster-template=true"

  Scenario: Create a template with multiple patches for a full profile
    When I run "acmlab provision template-create large-profile --namespace hive --patch replace:/compute/0/replicas:6 --patch-json replace:/compute/0/platform/aws/type:\"m5.4xlarge\" --patch replace:/networking/machineCIDR:10.0.0.0/16"
    Then a ClusterDeploymentCustomization "large-profile" is created in namespace "hive"
    And it contains 3 installConfigPatches

  Scenario: Create a template with typed JSON values
    When I run "acmlab provision template-create gpu-profile --namespace hive --patch-json replace:/compute/0/replicas:4 --patch-json replace:/networking/clusterNetwork/0:{\"cidr\":\"10.132.0.0/14\",\"hostPrefix\":23}"
    Then a ClusterDeploymentCustomization "gpu-profile" is created in namespace "hive"
    And the replicas patch value is numeric 4
    And the networking patch value is a map with cidr "10.132.0.0/14"

  Scenario: Template creation is idempotent
    Given template "small-profile" exists in namespace "hive"
    When I run "acmlab provision template-create small-profile --namespace hive --patch replace:/compute/0/replicas:2"
    Then the command succeeds without error
    And only one ClusterDeploymentCustomization "small-profile" exists in namespace "hive"

  Scenario: Template creation requires a namespace
    When I run "acmlab provision template-create my-template --patch replace:/p:v"
    Then the command fails with "the --namespace flag is required"

  Scenario: Template creation rejects malformed patches
    When I run "acmlab provision template-create bad --namespace hive --patch invalid-format"
    Then the command fails with "invalid patch format"

  # --- List ---

  Scenario: List cluster templates
    Given templates "small", "medium", and "large" exist in namespace "hive"
    When I run "acmlab provision template-list --namespace hive"
    Then the output shows 3 templates with their patch counts

  Scenario: List templates as JSON
    Given template "small" exists in namespace "hive"
    When I run "acmlab provision template-list --namespace hive --json"
    Then the output is valid JSON containing template name and patch count

  Scenario: List templates is namespace-scoped
    Given template "t1" exists in namespace "ns-a"
    And template "t2" exists in namespace "ns-b"
    When I run "acmlab provision template-list --namespace ns-a"
    Then the output shows 1 template
    And template "t2" is not listed

  Scenario: List templates when none exist
    When I run "acmlab provision template-list --namespace empty-ns"
    Then the output shows "No templates found"

  # --- Get ---

  Scenario: Get template details
    Given template "gpu-large" exists in namespace "hive" with 2 patches
    When I run "acmlab provision template-get gpu-large --namespace hive"
    Then the output shows the full ClusterDeploymentCustomization spec
    And the installConfigPatches array has 2 entries

  Scenario: Get a nonexistent template returns error
    When I run "acmlab provision template-get nonexistent --namespace hive"
    Then the command fails with "template nonexistent not found"

  # --- Apply ---

  Scenario: Apply a template to a ClusterDeployment
    Given template "gpu-large" exists in namespace "spoke3"
    And ClusterDeployment "spoke3" exists
    When I run "acmlab provision template-apply spoke3 --template gpu-large"
    Then the ClusterDeployment "spoke3" is annotated with "hive.openshift.io/cluster-deployment-customization=gpu-large"

  Scenario: Apply template with explicit namespace
    Given template "shared-profile" exists in namespace "hive-templates"
    And ClusterDeployment "spoke3" exists
    When I run "acmlab provision template-apply spoke3 --template shared-profile --template-namespace hive-templates"
    Then the ClusterDeployment "spoke3" is annotated with "hive.openshift.io/cluster-deployment-customization=shared-profile"

  Scenario: Apply template defaults to cluster namespace
    Given template "gpu-large" exists in namespace "spoke1"
    And ClusterDeployment "spoke1" exists
    When I run "acmlab provision template-apply spoke1 --template gpu-large"
    Then the template is resolved from namespace "spoke1"
    And the ClusterDeployment is annotated with the template reference

  Scenario: Apply a nonexistent template fails
    Given ClusterDeployment "spoke3" exists
    When I run "acmlab provision template-apply spoke3 --template nonexistent"
    Then the command fails with "template nonexistent not found"

  Scenario: Apply template requires --template flag
    When I run "acmlab provision template-apply spoke3"
    Then the command fails with "--template is required"

  # --- Remove ---

  Scenario: Remove a cluster template
    Given template "old-template" exists in namespace "hive"
    When I run "acmlab provision template-remove old-template --namespace hive"
    Then the ClusterDeploymentCustomization "old-template" is deleted from namespace "hive"

  Scenario: Remove a nonexistent template succeeds silently
    When I run "acmlab provision template-remove nonexistent --namespace hive"
    Then the command succeeds without error
