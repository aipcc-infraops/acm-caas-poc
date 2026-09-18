#!/usr/bin/env bash
# UC-57: Cloud-native cluster discovery (AWS, IBM Cloud, kubeconfig)
# Demonstrates discovering clusters from cloud providers, auto-importing with
# automatic credential retrieval, and hub/spoke context switching.

set -euo pipefail

echo "=== UC-57: Cloud-Native Cluster Discovery ==="

echo "1. Scan all configured cloud providers"
acmlab discovery scan

echo ""
echo "2. Scan AWS only (EKS + ROSA)"
acmlab discovery scan --provider aws

echo ""
echo "3. Scan AWS with region filter"
acmlab discovery scan --provider aws --region us-east-1

echo ""
echo "4. Scan IBM Cloud (IKS + ROKS)"
acmlab discovery scan --provider ibmcloud

echo ""
echo "5. Scan kubeconfig directory"
acmlab discovery scan-kubeconfigs --dir ~/.kube/

echo ""
echo "6. Dry-run auto-import (preview without creating)"
acmlab discovery auto-import my-rosa-cluster --provider aws --dry-run

echo ""
echo "7. Auto-import with cluster-set assignment"
acmlab discovery auto-import my-rosa-cluster --provider aws --cluster-set production

echo ""
echo "8. Auto-import from kubeconfig"
acmlab discovery auto-import-kubeconfig --name my-cluster --kubeconfig ~/.kube/my-cluster.kubeconfig

echo ""
echo "9. Auto-import with TLS fix (converts insecure to secure)"
acmlab discovery auto-import my-onprem-cluster --provider aws --fix-tls

echo ""
echo "10. Fix TLS on an already-imported cluster"
acmlab discovery fix-tls my-onprem-cluster

echo ""
echo "11. List discovery-imported clusters"
acmlab discovery list-imports

echo ""
echo "12. Switch to spoke context"
acmlab context spoke my-rosa-cluster

echo ""
echo "13. Check current context"
acmlab context current

echo ""
echo "14. Switch back to hub"
acmlab context hub

echo ""
echo "=== Done ==="
