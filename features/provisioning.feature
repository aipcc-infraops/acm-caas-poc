@slow
Feature: Cluster provisioning via Hive ClusterDeployment
  As a platform operator
  I want to provision spoke clusters programmatically via the ACM API
  So that the ComputeRequest controller can automate this

  Background:
    Given the ACM hub is reachable
    And I have a dynamic client for the hub

  @slow
  Scenario: Create a ClusterDeployment and wait for provisioning
    Given cloud credentials exist as a Secret in namespace "spoke-test"
    And a ClusterImageSet for the target OCP version exists
    When I provision cluster "spoke-test" with default settings
    Then the ClusterDeployment "spoke-test" is accepted by Hive
    And the cluster "spoke-test" eventually reaches Provisioned = True

  @slow @aws
  Scenario: Provision spoke1 on AWS
    Given a ClusterImageSet for the target OCP version exists
    When I provision cluster "spoke1" on platform "aws"
    Then the ClusterDeployment "spoke1" is accepted by Hive
    And the cluster "spoke1" eventually reaches Provisioned = True

  @slow @ibm
  Scenario: Provision spoke2 on IBM Cloud using ACM credentials
    Given the ACM credential "ibm-caas-creds" exists in open-cluster-management namespace
    And a ClusterImageSet for the target OCP version exists
    When I provision cluster "spoke2" on platform "ibmcloud"
    Then the ClusterDeployment "spoke2" is accepted by Hive
    And the cluster "spoke2" eventually reaches Provisioned = True

  Scenario: List provisioned clusters
    When I list all provisioned clusters
    Then I receive a list of ClusterDeployments with status

  @slow
  Scenario: Delete a ClusterDeployment and verify cleanup
    Given a ClusterDeployment "spoke-test" exists with status Provisioned = True
    When I destroy cluster "spoke-test"
    Then the ClusterDeployment "spoke-test" is removed
    And the ManagedCluster "spoke-test" is removed from the hub

  Scenario: Create a cluster template in a namespace
    Given the Hive ClusterDeploymentCustomization CRD exists
    When I run "acmlab provision template-create acmlab-small-profile --namespace hive --patch replace:/compute/0/replicas:2"
    Then a ClusterDeploymentCustomization "acmlab-small-profile" is created in namespace "hive"

  Scenario: Template commands require namespace flag
    When I run "acmlab provision template-create mytemplate --patch replace:/p:v" without --namespace
    Then the command fails with "the --namespace flag is required"

  Scenario: List templates in a namespace
    Given ClusterDeploymentCustomizations exist in namespace "hive"
    When I run "acmlab provision template-list --namespace hive"
    Then I see the templates from that namespace

  Scenario: Apply a template to a ClusterDeployment
    Given ClusterDeploymentCustomization "small-profile" exists in namespace "spoke1"
    And ClusterDeployment "spoke1" exists
    When I run "acmlab provision template-apply spoke1 --template small-profile --template-namespace spoke1"
    Then the ClusterDeployment is annotated with the template name
