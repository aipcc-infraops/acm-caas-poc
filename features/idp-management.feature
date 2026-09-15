Feature: Identity Provider management across clusters via ManifestWork
  As a platform operator
  I want to configure and rotate identity providers on managed clusters
  So that authentication policies are consistent and credentials can be rotated after incidents

  Background:
    Given the ACM hub is reachable
    And I have a dynamic client for the hub

  Scenario: Configure GitHub Identity Provider
    When I configure IdP "corp-github" on cluster "spoke1" with type "github"
    Then a ManifestWork "idp-corp-github" exists in namespace "spoke1"
    And it contains an OAuth CR with a GitHub identity provider
    And it contains a Secret in "openshift-config" with the client credentials

  Scenario: Configure Google Identity Provider
    When I configure IdP "corp-google" on cluster "spoke1" with type "google"
    Then the OAuth CR has a Google identity provider entry

  Scenario: Configure htpasswd Identity Provider
    When I configure IdP "local-users" on cluster "spoke1" with type "htpasswd"
    Then the Secret contains htpasswd-formatted user entries

  Scenario: Configure LDAP Identity Provider
    When I configure IdP "corp-ldap" on cluster "spoke1" with type "ldap"
    Then the OAuth CR has an LDAP identity provider with attribute mappings
    And the Secret contains the LDAP bind password

  Scenario: Configure OIDC (Keycloak) Identity Provider
    When I configure IdP "keycloak" on cluster "spoke1" with type "oidc"
    Then the OAuth CR has an OpenID identity provider with issuer and claims

  Scenario: Configure is idempotent
    Given IdP "corp-github" is already configured on cluster "spoke1"
    When I configure IdP "corp-github" again on cluster "spoke1"
    Then no error is returned

  Scenario: Rotate credentials after a leak
    Given IdP "corp-github" is configured on cluster "spoke1"
    When I rotate credentials for "corp-github" on "spoke1" with a new client secret
    Then the ManifestWork Secret is updated with the new credentials

  Scenario: List Identity Providers on a cluster
    Given IdP "corp-github" and "corp-ldap" are configured on cluster "spoke1"
    When I list IdPs on cluster "spoke1"
    Then I see both providers with their type and status

  Scenario: Remove an Identity Provider
    Given IdP "corp-github" is configured on cluster "spoke1"
    When I remove IdP "corp-github" from cluster "spoke1"
    Then the ManifestWork is deleted
