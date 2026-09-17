#!/usr/bin/env bash
# UC-42: Workload disaster recovery (manual)
# Shows raw ACM resources: ManifestWork for Velero, ConfigurationPolicy, ApplicationSet for failover

set -euo pipefail

SOURCE="prod-east"
TARGET="prod-west"
PAIR="${SOURCE}-${TARGET}"
POLICY_NS="open-cluster-management-policies"

echo "=== UC-42: Disaster Recovery (manual) ==="

echo "1. Create ManifestWork to deploy Velero schedule on source cluster"
cat <<EOF | oc apply -f -
apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: dr-velero-${PAIR}
  namespace: ${SOURCE}
  labels:
    acmlab.redhat.com/managed: "true"
    acmlab.redhat.com/dr-pair: ${PAIR}
    acmlab.redhat.com/dr-type: velero-schedule
spec:
  workload:
    manifests:
      - apiVersion: velero.io/v1
        kind: Schedule
        metadata:
          name: dr-backup-${PAIR}
          namespace: openshift-adp
        spec:
          schedule: "0 */4 * * *"
          template:
            ttl: "720h"
            includedNamespaces: ["*"]
            storageLocation: default
            volumeSnapshotLocations: ["default"]
EOF

echo ""
echo "2. Create ConfigurationPolicy to enforce Velero health"
cat <<EOF | oc apply -f -
apiVersion: policy.open-cluster-management.io/v1
kind: ConfigurationPolicy
metadata:
  name: dr-velero-policy-${PAIR}
  namespace: ${POLICY_NS}
  labels:
    acmlab.redhat.com/managed: "true"
    acmlab.redhat.com/dr-pair: ${PAIR}
spec:
  remediationAction: enforce
  severity: high
  object-templates:
    - complianceType: musthave
      objectDefinition:
        apiVersion: velero.io/v1
        kind: Schedule
        metadata:
          name: dr-backup-${PAIR}
          namespace: openshift-adp
        spec:
          schedule: "0 */4 * * *"
EOF

echo ""
echo "3. Create ManifestWork for Velero restore on target cluster"
cat <<EOF | oc apply -f -
apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: dr-restore-${PAIR}
  namespace: ${TARGET}
  labels:
    acmlab.redhat.com/managed: "true"
    acmlab.redhat.com/dr-pair: ${PAIR}
    acmlab.redhat.com/dr-type: restore
spec:
  workload:
    manifests:
      - apiVersion: velero.io/v1
        kind: Restore
        metadata:
          name: dr-restore-${PAIR}
          namespace: openshift-adp
        spec:
          backupName: dr-backup-${PAIR}
          includedNamespaces: ["*"]
          restorePVs: true
EOF

echo ""
echo "4. Create ApplicationSet for failover GitOps routing"
cat <<EOF | oc apply -f -
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: dr-failover-${PAIR}
  namespace: openshift-gitops
  labels:
    acmlab.redhat.com/managed: "true"
    acmlab.redhat.com/dr-pair: ${PAIR}
spec:
  generators:
    - clusterDecisionResource:
        configMapRef: acm-placement
        labelSelector:
          matchLabels:
            acmlab.redhat.com/dr-pair: ${PAIR}
        requeueAfterSeconds: 180
  template:
    metadata:
      name: "dr-{{name}}-${PAIR}"
    spec:
      project: default
      source:
        repoURL: https://github.com/example-org/cluster-configs
        path: workloads/
        targetRevision: main
      destination:
        server: "{{server}}"
        namespace: default
EOF

echo ""
echo "5. Verify DR resources"
oc get manifestwork -n "${SOURCE}" -l "acmlab.redhat.com/dr-pair=${PAIR}"
oc get manifestwork -n "${TARGET}" -l "acmlab.redhat.com/dr-pair=${PAIR}"
oc get configurationpolicy -n "${POLICY_NS}" -l "acmlab.redhat.com/dr-pair=${PAIR}"

echo ""
echo "=== Done ==="
