Feature: Certificate expiry detection via CertificatePolicy
  As a platform operator
  I want to detect certificates approaching expiry across the fleet
  So that certificate-related outages are prevented

  Background:
    Given the ACM hub is reachable
    And I have a dynamic client for the hub

  Scenario: Deploy certificate expiry monitoring with 30-day threshold
    When I apply a CertificatePolicy "cert-check" with expiry threshold 30 days
    Then a Policy "cert-check" is created in the global-set namespace
    And the policy-template kind is "CertificatePolicy"
    And the minimumDuration is "720h"
    And the severity is "high"
    And the namespaceSelector includes "openshift-config" and "openshift-ingress"

  Scenario: Monitor custom namespaces
    When I apply a CertificatePolicy "cert-custom" with expiry 14 days and namespaces "kube-system,openshift-etcd"
    Then the namespaceSelector includes "kube-system" and "openshift-etcd"
    And the minimumDuration is "336h"

  Scenario: Default namespaces when none specified
    When I apply a CertificatePolicy "cert-default" with expiry 30 days and no namespace list
    Then the namespaceSelector includes "openshift-config" and "openshift-ingress"

  Scenario: Check fleet-wide compliance
    Given a CertificatePolicy "cert-check" is active on all clusters
    When I get the policy status for "cert-check"
    Then I see per-cluster compliance state
    And NonCompliant clusters have expiring certificates
