Feature: Unique identity provider per cluster (UC-16)

  As a platform operator
  I want each cluster to have unique credentials
  So that a single credential leak does not require rotating the entire fleet

  Scenario: Deploy unique emergency credentials to a cluster
    Given a cluster "spoke1" is registered in ACM
    When I run "acmlab idp configure-unique --cluster spoke1 --admin-user cluster-admin"
    Then a ManifestWork "idp-emergency-spoke1" is created in namespace "spoke1"
    And the htpasswd secret contains bcrypt-hashed credentials
    And the generated password is unique to spoke1

  Scenario: Credentials differ between clusters
    Given clusters "spoke1" and "spoke2" are registered
    When I deploy unique credentials to both clusters
    Then spoke1 and spoke2 have different passwords
    And a credential leak on spoke1 does not affect spoke2

  Scenario: Enforce SSO compliance fleet-wide
    When I run "acmlab idp enforce-sso"
    Then a Policy "sso-enforcement" is created
    And clusters without an OpenID IdP are marked NonCompliant

  Scenario: Rotate unique credentials on a single cluster
    Given unique credentials are deployed to "spoke1"
    When I run "acmlab idp configure-unique --cluster spoke1 --admin-user cluster-admin"
    Then new credentials are generated for spoke1
    And the rotation timestamp is updated
