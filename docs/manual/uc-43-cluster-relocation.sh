#!/usr/bin/env bash
# UC-43: Cluster relocation — planned workload migration (manual)
# Shows raw ACM resources: ConfigMap for plan, ManifestWork for cordon + workload deploy

set -euo pipefail

SOURCE="prod-east"
TARGET="prod-west"
PLAN_ID="${SOURCE}-to-${TARGET}"
POLICY_NS="open-cluster-management-policies"

echo "=== UC-43: Cluster Relocation (manual) ==="

echo "1. Create migration plan as ConfigMap"
cat <<EOF | oc apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: migration-plan-${PLAN_ID}
  namespace: ${POLICY_NS}
  labels:
    acmlab.redhat.com/managed: "true"
    acmlab.redhat.com/migration-plan: "true"
    acmlab.redhat.com/source: ${SOURCE}
    acmlab.redhat.com/target: ${TARGET}
data:
  planID: ${PLAN_ID}
  sourceCluster: ${SOURCE}
  targetCluster: ${TARGET}
  workloads: "3"
  namespaces: "app-ns,data-ns"
  phase: Planned
  createdAt: "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
EOF

echo ""
echo "2. Cordon source cluster (mark as draining)"
cat <<EOF | oc apply -f -
apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: migration-cordon-${PLAN_ID}
  namespace: ${SOURCE}
  labels:
    acmlab.redhat.com/managed: "true"
    acmlab.redhat.com/migration: draining
    acmlab.redhat.com/plan-id: ${PLAN_ID}
spec:
  workload:
    manifests:
      - apiVersion: v1
        kind: ConfigMap
        metadata:
          name: migration-status
          namespace: openshift-config
        data:
          migration: draining
          plan-id: ${PLAN_ID}
          cordon-time: auto
EOF

echo ""
echo "3. Deploy workloads to target cluster"
cat <<EOF | oc apply -f -
apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: migration-workloads-${PLAN_ID}
  namespace: ${TARGET}
  labels:
    acmlab.redhat.com/managed: "true"
    acmlab.redhat.com/migration: deploying
    acmlab.redhat.com/plan-id: ${PLAN_ID}
    acmlab.redhat.com/source: ${SOURCE}
spec:
  workload:
    manifests:
      - apiVersion: v1
        kind: ConfigMap
        metadata:
          name: migration-manifest
          namespace: openshift-config
        data:
          source: ${SOURCE}
          target: ${TARGET}
          namespaces: "app-ns,data-ns"
EOF

echo ""
echo "4. Update plan phase to Verifying"
oc patch configmap "migration-plan-${PLAN_ID}" -n "${POLICY_NS}" \
  --type merge -p '{"data":{"phase":"Verifying"}}'

echo ""
echo "5. Verify migration resources"
oc get configmap -n "${POLICY_NS}" -l "acmlab.redhat.com/migration-plan=true"
oc get manifestwork -n "${SOURCE}" -l "acmlab.redhat.com/plan-id=${PLAN_ID}"
oc get manifestwork -n "${TARGET}" -l "acmlab.redhat.com/plan-id=${PLAN_ID}"

echo ""
echo "6. Rollback (if needed): delete target work, remove cordon, update phase"
# oc delete manifestwork migration-workloads-${PLAN_ID} -n ${TARGET}
# oc delete manifestwork migration-cordon-${PLAN_ID} -n ${SOURCE}
# oc patch configmap migration-plan-${PLAN_ID} -n ${POLICY_NS} --type merge -p '{"data":{"phase":"RolledBack"}}'

echo ""
echo "=== Done ==="
