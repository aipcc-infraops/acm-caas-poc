#!/usr/bin/env bash
# UC-51: VM lifecycle management — OpenShift Virtualization (manual)
# Shows raw ACM resources: ManifestWork wrapping KubeVirt VirtualMachine CRD

set -euo pipefail

VM_NAME="web-vm"
CLUSTER="spoke1"

echo "=== UC-51: VM Lifecycle (manual) ==="

echo "1. Create ManifestWork with VirtualMachine"
cat <<EOF | oc apply -f -
apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  name: vm-${VM_NAME}-${CLUSTER}
  namespace: ${CLUSTER}
  labels:
    acmlab.redhat.com/managed: "true"
    acmlab.redhat.com/vm: "true"
    acmlab.redhat.com/vm-name: ${VM_NAME}
spec:
  workload:
    manifests:
      - apiVersion: kubevirt.io/v1
        kind: VirtualMachine
        metadata:
          name: ${VM_NAME}
          namespace: default
        spec:
          running: true
          template:
            spec:
              domain:
                cpu:
                  cores: 4
                devices:
                  disks:
                    - name: rootdisk
                      disk:
                        bus: virtio
                memory:
                  guest: 8Gi
              volumes:
                - name: rootdisk
                  containerDisk:
                    image: registry.redhat.io/rhel9/rhel-guest-image:latest
EOF

echo ""
echo "2. Verify ManifestWork"
oc get manifestwork -n ${CLUSTER} -l "acmlab.redhat.com/vm-name=${VM_NAME}"

echo ""
echo "3. Check ManifestWork status"
oc get manifestwork vm-${VM_NAME}-${CLUSTER} -n ${CLUSTER} -o jsonpath='{.status.conditions[?(@.type=="Applied")].status}'
echo ""

echo ""
echo "=== Done ==="
