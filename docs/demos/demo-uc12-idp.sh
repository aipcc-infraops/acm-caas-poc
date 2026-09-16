#!/usr/bin/env bash
# UC-12: Identity Provider management via ManifestWork
# Demo: configure multiple IdP types, list, rotate, remove
set -euo pipefail

CLUSTER="${1:-spoke1}"

echo "=== UC-12: Identity Provider Management ==="

echo "--- Step 1: Configure GitHub IdP ---"
acmlab idp configure corp-github \
  --cluster "$CLUSTER" \
  --type github \
  --client-id "placeholder-client-id" \
  --client-secret "placeholder-client-secret" \
  --organizations "example-org"

echo "--- Step 2: Configure Google IdP ---"
acmlab idp configure corp-google \
  --cluster "$CLUSTER" \
  --type google \
  --client-id "placeholder-google-id" \
  --client-secret "placeholder-google-secret"

echo "--- Step 3: Configure htpasswd IdP ---"
acmlab idp configure local-users \
  --cluster "$CLUSTER" \
  --type htpasswd \
  --users "admin:placeholder-pass,developer:placeholder-pass"

echo "--- Step 4: Configure LDAP IdP ---"
acmlab idp configure corp-ldap \
  --cluster "$CLUSTER" \
  --type ldap \
  --ldap-url "ldap://ldap.example.com:389/ou=users,dc=example,dc=com?uid" \
  --bind-dn "cn=admin,dc=example,dc=com" \
  --bind-password "placeholder-ldap-password" \
  --insecure

echo "--- Step 5: Configure OIDC (Keycloak) IdP ---"
acmlab idp configure keycloak \
  --cluster "$CLUSTER" \
  --type oidc \
  --client-id "placeholder-kc-client" \
  --client-secret "placeholder-kc-secret" \
  --issuer-url "https://keycloak.example.com/realms/myrealm"

echo "--- Step 6: List all IdPs ---"
acmlab idp list --cluster "$CLUSTER"

echo "--- Step 7: List as JSON ---"
acmlab idp list --cluster "$CLUSTER" --json

echo "--- Step 8: Rotate GitHub credentials ---"
acmlab idp rotate corp-github \
  --cluster "$CLUSTER" \
  --client-id "new-client-id" \
  --client-secret "new-client-secret"

echo "--- Step 9: Clean up ---"
echo "  acmlab idp remove corp-github --cluster $CLUSTER"
echo "  acmlab idp remove corp-google --cluster $CLUSTER"
echo "  acmlab idp remove local-users --cluster $CLUSTER"
echo "  acmlab idp remove corp-ldap --cluster $CLUSTER"
echo "  acmlab idp remove keycloak --cluster $CLUSTER"

echo "=== Demo complete ==="
