#!/bin/bash
# UC-06: Cluster Resource Monitoring and Observability (Thanos)
#
# Read cluster resource info from ManagedClusterInfo and deploy the
# ACM observability stack (MultiClusterObservability + MinIO for storage).
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#   - For observability setup: S3-compatible storage (or MinIO deployed here)
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

CLUSTER="spoke2"
OBS_NAMESPACE="open-cluster-management-observability"


# ═════════════════════════════════════════════════════════════════════
# READ CLUSTER RESOURCES
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 1: List cluster resource summaries
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 1: Cluster resource summaries ==="

echo "All clusters:"
kubectl get managedclusterinfo -A \
  -o custom-columns='NAME:.metadata.name,NODES:.status.nodeList[*].name'

# ─────────────────────────────────────────────────────────────────────
# Step 2: Detailed node info for a cluster
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 2: Node details for $CLUSTER ==="

echo "Node list:"
kubectl get managedclusterinfo -n "$CLUSTER" "$CLUSTER" \
  -o jsonpath='{range .status.nodeList[*]}{.name}: capacity={.capacity.cpu}cpu,{.capacity.memory} labels={.labels.node-role\.kubernetes\.io/worker}{"\n"}{end}'

echo ""
echo "Distribution info:"
kubectl get managedclusterinfo -n "$CLUSTER" "$CLUSTER" \
  -o jsonpath='{.status.distributionInfo}'  | python3 -m json.tool 2>/dev/null || \
kubectl get managedclusterinfo -n "$CLUSTER" "$CLUSTER" \
  -o jsonpath='type={.status.distributionInfo.type} version={.status.distributionInfo.ocp.version}'
echo ""


# ═════════════════════════════════════════════════════════════════════
# DEPLOY OBSERVABILITY STACK
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 3: Create observability namespace
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 3: Create namespace ==="

kubectl create namespace "$OBS_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

# ─────────────────────────────────────────────────────────────────────
# Step 4: Deploy MinIO for Thanos object storage
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 4: Deploy MinIO ==="

cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: minio
  namespace: ${OBS_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: minio
  template:
    metadata:
      labels:
        app: minio
    spec:
      containers:
      - name: minio
        image: quay.io/minio/minio:latest
        args: ["server", "/data"]
        env:
        - name: MINIO_ROOT_USER
          value: "<minio-access-key>"
        - name: MINIO_ROOT_PASSWORD
          value: "<minio-secret-key>"
        ports:
        - containerPort: 9000
---
apiVersion: v1
kind: Service
metadata:
  name: minio
  namespace: ${OBS_NAMESPACE}
spec:
  selector:
    app: minio
  ports:
  - port: 9000
    targetPort: 9000
EOF

# ─────────────────────────────────────────────────────────────────────
# Step 5: Create Thanos object storage secret
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 5: Create Thanos storage config ==="

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: thanos-object-storage
  namespace: ${OBS_NAMESPACE}
type: Opaque
stringData:
  thanos.yaml: |
    type: s3
    config:
      bucket: thanos
      endpoint: minio.${OBS_NAMESPACE}.svc:9000
      insecure: true
      access_key: <minio-access-key>
      secret_key: <minio-secret-key>
EOF

# ─────────────────────────────────────────────────────────────────────
# Step 6: Create MultiClusterObservability CR
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 6: Create MultiClusterObservability ==="

cat <<EOF | kubectl apply -f -
apiVersion: observability.open-cluster-management.io/v1beta2
kind: MultiClusterObservability
metadata:
  name: observability
spec:
  observabilityAddonSpec: {}
  storageConfig:
    metricObjectStorage:
      name: thanos-object-storage
      key: thanos.yaml
    statefulSetSize: 10Gi
EOF


# ═════════════════════════════════════════════════════════════════════
# CHECK STATUS
# ═════════════════════════════════════════════════════════════════════

# ─────────────────────────────────────────────────────────────────────
# Step 7: Check observability status
# ─────────────────────────────────────────────────────────────────────
echo "=== Step 7: Observability status ==="

kubectl get multiclusterobservability observability \
  -o jsonpath='{range .status.conditions[*]}{.type}: {.status} — {.message}{"\n"}{end}'


# ═════════════════════════════════════════════════════════════════════
# TEARDOWN
# ═════════════════════════════════════════════════════════════════════

# kubectl delete multiclusterobservability observability
# kubectl delete deployment minio -n ${OBS_NAMESPACE}
# kubectl delete service minio -n ${OBS_NAMESPACE}
# kubectl delete secret thanos-object-storage -n ${OBS_NAMESPACE}
# kubectl delete namespace ${OBS_NAMESPACE}


# ═════════════════════════════════════════════════════════════════════
# Alternative: Using the acmlab CLI
# ═════════════════════════════════════════════════════════════════════
#
# acmlab monitor list
# acmlab monitor status spoke2
# acmlab monitor setup          # deploys MinIO + MCO in one step
# acmlab monitor obs-status
# acmlab monitor teardown
