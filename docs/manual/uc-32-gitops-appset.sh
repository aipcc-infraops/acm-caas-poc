#!/usr/bin/env bash
# UC-32: GitOps fleet deployment via ApplicationSet
# Manual oc/kubectl commands for reference

NAMESPACE="openshift-gitops"
APPSET_NAME="monitoring-stack"
REPO_URL="https://github.com/example/monitoring.git"
REPO_PATH="manifests/base"

echo "=== Create ApplicationSet (placement generator) ==="
cat <<EOF | oc apply -f -
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: ${APPSET_NAME}
  namespace: ${NAMESPACE}
  labels:
    acmlab.redhat.com/managed: "true"
    acmlab.redhat.com/gitops: "true"
spec:
  generators:
  - clusterDecisionResource:
      configMapRef: acm-placement
      labelSelector:
        matchLabels:
          env: prod
      requeueAfterSeconds: 180
  template:
    metadata:
      name: '{{name}}-${APPSET_NAME}'
    spec:
      project: default
      source:
        repoURL: ${REPO_URL}
        path: ${REPO_PATH}
        targetRevision: main
      destination:
        server: '{{server}}'
        namespace: default
      syncPolicy:
        automated:
          selfHeal: true
          prune: true
EOF

echo ""
echo "=== Create ApplicationSet (cluster generator) ==="
cat <<EOF | oc apply -f -
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: logging-stack
  namespace: ${NAMESPACE}
spec:
  generators:
  - clusters:
      selector:
        matchLabels:
          tier: infra
  template:
    metadata:
      name: '{{name}}-logging-stack'
    spec:
      project: default
      source:
        repoURL: https://github.com/example/logging.git
        path: k8s/overlays/prod
        targetRevision: main
      destination:
        server: '{{server}}'
        namespace: default
      syncPolicy:
        automated:
          selfHeal: true
          prune: true
EOF

echo ""
echo "=== List ApplicationSets ==="
oc get applicationsets -n ${NAMESPACE} -o wide

echo ""
echo "=== Get ApplicationSet details ==="
oc get applicationset ${APPSET_NAME} -n ${NAMESPACE} -o yaml

echo ""
echo "=== Check generated Applications ==="
oc get applications -n ${NAMESPACE} -l "app.kubernetes.io/managed-by=applicationset-controller"

echo ""
echo "=== Trigger sync (add refresh annotation) ==="
oc annotate applicationset ${APPSET_NAME} -n ${NAMESPACE} \
  argocd.argoproj.io/refresh="$(date -u +%Y-%m-%dT%H:%M:%SZ)" --overwrite

echo ""
echo "=== Delete ApplicationSet ==="
oc delete applicationset ${APPSET_NAME} -n ${NAMESPACE}
