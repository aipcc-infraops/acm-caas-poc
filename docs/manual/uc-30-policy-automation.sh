#!/usr/bin/env bash
# UC-30: Policy automation — Ansible auto-remediation
# Manual oc/kubectl reference for PolicyAutomation CRs

set -euo pipefail

NAMESPACE="open-cluster-management-policies"
POLICY_NAME="image-policy"
AUTOMATION_NAME="auto-remediate-images"
TOWER_SECRET="tower-creds"
JOB_TEMPLATE="remediate-registry-violations"

echo "=== UC-30: Policy Automation (manual) ==="

echo "1. Create Ansible Tower credentials secret"
cat <<EOF | oc apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: ${TOWER_SECRET}
  namespace: ${NAMESPACE}
type: Opaque
stringData:
  host: "https://tower.example.com"
  token: "<ansible-tower-token>"
EOF

echo ""
echo "2. Create PolicyAutomation CR"
cat <<EOF | oc apply -f -
apiVersion: policy.open-cluster-management.io/v1beta1
kind: PolicyAutomation
metadata:
  name: ${AUTOMATION_NAME}
  namespace: ${NAMESPACE}
  labels:
    acmlab.redhat.com/managed: "true"
    acmlab.redhat.com/automation: "true"
spec:
  policyRef: ${POLICY_NAME}
  mode: scan
  automationDef:
    name: ${JOB_TEMPLATE}
    secret: ${TOWER_SECRET}
    type: AnsibleJob
    extra_vars:
      policy_name: "{{ policy_name }}"
      target_clusters: "{{ target_clusters }}"
EOF

echo ""
echo "3. Check PolicyAutomation status"
oc get policyautomation ${AUTOMATION_NAME} -n ${NAMESPACE} -o yaml

echo ""
echo "4. List all PolicyAutomations"
oc get policyautomation -n ${NAMESPACE}

echo ""
echo "5. Update mode to disabled"
oc patch policyautomation ${AUTOMATION_NAME} -n ${NAMESPACE} \
  --type merge -p '{"spec":{"mode":"disabled"}}'

echo ""
echo "6. Update mode to once (one-shot remediation)"
oc patch policyautomation ${AUTOMATION_NAME} -n ${NAMESPACE} \
  --type merge -p '{"spec":{"mode":"once"}}'

echo ""
echo "7. Check AnsibleJob runs triggered by the automation"
oc get ansiblejobs -n ${NAMESPACE} -l \
  tower_job_id

echo ""
echo "8. Delete PolicyAutomation"
oc delete policyautomation ${AUTOMATION_NAME} -n ${NAMESPACE}

echo ""
echo "9. Clean up tower credentials"
oc delete secret ${TOWER_SECRET} -n ${NAMESPACE}

echo ""
echo "=== Done ==="
