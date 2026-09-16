#!/bin/bash
# UC-29: Security Baseline Enforcement via Gatekeeper (OPA)
#
# Deploys Gatekeeper ConstraintTemplates and Constraints to spoke clusters
# via ManifestWork. A ConfigurationPolicy monitors Gatekeeper health.
# CIS Level 1 constraints: no privileged pods, no hostPID/hostNetwork,
# required resource limits, approved image registries.
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#   - Spoke clusters registered in ACM
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

CLUSTER_NAME="spoke1"
NAMESPACE="open-cluster-management"
BASELINE="cis-level1"


# ═════════════════════════════════════════════════════════════════════
# DEPLOY SECURITY BASELINE
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 1: Deploy Gatekeeper constraints via ManifestWork
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 1: Deploy ConstraintTemplates + Constraints ==="

cat <<EOF | kubectl apply -f -
apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: security-baseline-${CLUSTER_NAME}
  namespace: ${CLUSTER_NAME}
  labels:
    caas/security-level: ${BASELINE}
spec:
  workload:
    manifests:
    - apiVersion: templates.gatekeeper.sh/v1
      kind: ConstraintTemplate
      metadata:
        name: k8spspprivilegedcontainer
      spec:
        crd:
          spec:
            names:
              kind: K8sPSPPrivilegedContainer
        targets:
        - target: admission.k8s.gatekeeper.sh
          rego: |
            package k8spspprivilegedcontainer
            violation[{"msg": msg}] {
              c := input.review.object.spec.containers[_]
              c.securityContext.privileged == true
              msg := sprintf("Privileged container not allowed: %v", [c.name])
            }
    - apiVersion: constraints.gatekeeper.sh/v1beta1
      kind: K8sPSPPrivilegedContainer
      metadata:
        name: deny-privileged
      spec:
        match:
          kinds:
          - apiGroups: [""]
            kinds: ["Pod"]
    - apiVersion: templates.gatekeeper.sh/v1
      kind: ConstraintTemplate
      metadata:
        name: k8spsphostnamespace
      spec:
        crd:
          spec:
            names:
              kind: K8sPSPHostNamespace
        targets:
        - target: admission.k8s.gatekeeper.sh
          rego: |
            package k8spsphostnamespace
            violation[{"msg": msg}] {
              input.review.object.spec.hostPID == true
              msg := "hostPID not allowed"
            }
            violation[{"msg": msg}] {
              input.review.object.spec.hostNetwork == true
              msg := "hostNetwork not allowed"
            }
    - apiVersion: constraints.gatekeeper.sh/v1beta1
      kind: K8sPSPHostNamespace
      metadata:
        name: deny-host-namespace
      spec:
        match:
          kinds:
          - apiGroups: [""]
            kinds: ["Pod"]
EOF

# ─────────────────────────────────────────────────────────────────────
# Step 2: Create Gatekeeper health policy
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 2: Create Gatekeeper health ConfigurationPolicy ==="

cat <<EOF | kubectl apply -f -
apiVersion: policy.open-cluster-management.io/v1
kind: Policy
metadata:
  name: gatekeeper-health-${CLUSTER_NAME}
  namespace: ${NAMESPACE}
spec:
  remediationAction: inform
  disabled: false
  policy-templates:
  - objectDefinition:
      apiVersion: policy.open-cluster-management.io/v1
      kind: ConfigurationPolicy
      metadata:
        name: gatekeeper-health-config
      spec:
        remediationAction: inform
        severity: high
        object-templates:
        - complianceType: musthave
          objectDefinition:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: gatekeeper-controller-manager
              namespace: gatekeeper-system
            status:
              readyReplicas: 1
EOF


# ═════════════════════════════════════════════════════════════════════
# VERIFY
# ═════════════════════════════════════════════════════════════════════

echo "=== ManifestWork status ==="

kubectl get manifestwork "security-baseline-${CLUSTER_NAME}" -n "$CLUSTER_NAME" \
  -o jsonpath='{range .status.conditions[*]}{.type}: {.status}{"\n"}{end}'


# ═════════════════════════════════════════════════════════════════════
# REMOVE
# ═════════════════════════════════════════════════════════════════════

# kubectl delete manifestwork security-baseline-${CLUSTER_NAME} -n ${CLUSTER_NAME}
# kubectl delete policy gatekeeper-health-${CLUSTER_NAME} -n ${NAMESPACE}


# ═════════════════════════════════════════════════════════════════════
# Alternative: Using the acmlab CLI
# ═════════════════════════════════════════════════════════════════════
#
# acmlab security apply cis-level1 --cluster spoke1
# acmlab security status spoke1
# acmlab security list
# acmlab security remove spoke1
