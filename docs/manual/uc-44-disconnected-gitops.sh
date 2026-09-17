#!/usr/bin/env bash
# UC-44: Disconnected cluster GitOps — agent-mode (manual)
# Shows raw ACM resources: ApplicationSet with pull-mode annotation

set -euo pipefail

APP_NAME="edge-apps"
NAMESPACE="openshift-gitops"
CLUSTER1="edge-01"
CLUSTER2="edge-02"

echo "=== UC-44: Disconnected GitOps (manual) ==="

echo "1. Create agent-mode ApplicationSet with PullMode annotation"
cat <<EOF | oc apply -f -
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: ${APP_NAME}
  namespace: ${NAMESPACE}
  labels:
    acmlab.redhat.com/managed: "true"
    acmlab.redhat.com/agent-mode: "true"
  annotations:
    apps.open-cluster-management.io/ocm-managed-cluster: "true"
    apps.open-cluster-management.io/ocm-managed-cluster-app-namespace: ${NAMESPACE}
spec:
  generators:
    - list:
        elements:
          - cluster: ${CLUSTER1}
            url: ""
          - cluster: ${CLUSTER2}
            url: ""
  template:
    metadata:
      name: "${APP_NAME}-{{cluster}}"
      annotations:
        apps.open-cluster-management.io/ocm-managed-cluster: "{{cluster}}"
    spec:
      project: default
      source:
        repoURL: https://github.com/org/edge-configs
        path: manifests/edge
        targetRevision: main
      destination:
        server: "{{url}}"
        namespace: default
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
EOF

echo ""
echo "2. Verify ApplicationSet created"
oc get applicationset ${APP_NAME} -n ${NAMESPACE}

echo ""
echo "3. Check generated Applications"
oc get applications -n ${NAMESPACE} -l "app.kubernetes.io/managed-by=applicationset-controller"

echo ""
echo "=== Done ==="
