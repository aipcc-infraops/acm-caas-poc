#!/usr/bin/env bash
# UC-45: Fleet right-sizing — MCOA monitoring (manual)
# Shows raw ACM resources: ManifestWork with PrometheusRule for metrics collection

set -euo pipefail

CLUSTER="spoke1"

echo "=== UC-45: Right-Sizing (manual) ==="

echo "1. Create ManifestWork to deploy PrometheusRule for resource metrics"
cat <<EOF | oc apply -f -
apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: rightsizing-metrics-${CLUSTER}
  namespace: ${CLUSTER}
  labels:
    acmlab.redhat.com/managed: "true"
    acmlab.redhat.com/rightsizing: "true"
spec:
  workload:
    manifests:
      - apiVersion: monitoring.coreos.com/v1
        kind: PrometheusRule
        metadata:
          name: acmlab-rightsizing
          namespace: openshift-monitoring
        spec:
          groups:
            - name: acmlab-rightsizing
              interval: 5m
              rules:
                - record: acmlab:container_cpu_usage_ratio
                  expr: |
                    sum by (namespace, pod, container) (
                      rate(container_cpu_usage_seconds_total{container!=""}[5m])
                    ) /
                    sum by (namespace, pod, container) (
                      kube_pod_container_resource_requests{resource="cpu"}
                    )
                - record: acmlab:container_memory_usage_ratio
                  expr: |
                    sum by (namespace, pod, container) (
                      container_memory_working_set_bytes{container!=""}
                    ) /
                    sum by (namespace, pod, container) (
                      kube_pod_container_resource_requests{resource="memory"}
                    )
EOF

echo ""
echo "2. Verify ManifestWork created"
oc get manifestwork -n ${CLUSTER} -l "acmlab.redhat.com/rightsizing=true"

echo ""
echo "3. Check ManifestWork status"
oc get manifestwork rightsizing-metrics-${CLUSTER} -n ${CLUSTER} -o jsonpath='{.status.conditions[?(@.type=="Applied")].status}'
echo ""

echo ""
echo "=== Done ==="
