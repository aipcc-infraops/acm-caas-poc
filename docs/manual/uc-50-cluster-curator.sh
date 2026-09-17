#!/usr/bin/env bash
# UC-50: ClusterCurator day-2 automation hooks (manual)
# Shows raw ACM resources: ClusterCurator with pre/post hooks

set -euo pipefail

CLUSTER="spoke1"

echo "=== UC-50: ClusterCurator (manual) ==="

echo "1. Create ClusterCurator with pre and post hooks"
cat <<EOF | oc apply -f -
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: ClusterCurator
metadata:
  name: ${CLUSTER}
  namespace: ${CLUSTER}
  labels:
    acmlab.redhat.com/managed: "true"
spec:
  desiredCuration: upgrade
  upgrade:
    desiredUpdate: 4.22.10
    towerAuthSecret: ansible-creds
  prehook:
    - name: validate-health
      type: Job
  posthook:
    - name: notify-team
      type: AnsibleJob
      extra_vars:
        cluster_name: ${CLUSTER}
EOF

echo ""
echo "2. Verify ClusterCurator"
oc get clustercurator ${CLUSTER} -n ${CLUSTER}

echo ""
echo "3. Check curator conditions"
oc get clustercurator ${CLUSTER} -n ${CLUSTER} -o jsonpath='{.status.conditions[*].type}'
echo ""

echo ""
echo "4. List all ClusterCurators"
oc get clustercurators --all-namespaces

echo ""
echo "=== Done ==="
