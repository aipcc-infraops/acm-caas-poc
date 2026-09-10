#!/bin/bash
# UC-13: Registry Mirror for Restricted Clusters
#
# Problem: Clusters that cannot pull from registry.redhat.io (ROKS, air-gapped)
# fail to import into ACM because the klusterlet pods can't pull images.
#
# Solution: Use ManagedClusterImageRegistry CRD to tell ACM to rewrite image
# references in klusterlet manifests to use a mirror registry.
#
# Prerequisites:
#   - oc/kubectl logged into the ACM hub
#   - skopeo installed (for mirroring images)
#   - Credentials for both registry.redhat.io and the target registry
#   - A target registry (quay.io, ICR, Artifactory, etc.)
#
# Usage: This script is a reference — run commands one section at a time.

set -euo pipefail

CLUSTER_NAME="import-test"
MIRROR_REGISTRY="quay.io/myorg/acm-mirror"
PULL_SECRET_PATH="$HOME/pull-secret.json"

# ─────────────────────────────────────────────────────────────────────
# Step 1: Identify required images
# ─────────────────────────────────────────────────────────────────────
# ACM creates ManifestWorks in the cluster's namespace. Each ManifestWork
# contains k8s manifests with image references that the spoke must pull.

echo "=== Step 1: List images from ManifestWorks ==="

kubectl get manifestwork -n "$CLUSTER_NAME" -o json | \
  jq -r '
    .items[] |
    .metadata.name as $mw |
    .. | .image? // empty |
    select(contains("/")) |
    "\(.) (\($mw))"
  ' | sort -u

# ─────────────────────────────────────────────────────────────────────
# Step 2: Mirror images with skopeo
# ─────────────────────────────────────────────────────────────────────
# Copy each image from registry.redhat.io to the target registry.
# The path structure must be preserved (multicluster-engine/*, rhacm2/*).

echo ""
echo "=== Step 2: Mirror images ==="

# Login to source (Red Hat) and target registries
skopeo login registry.redhat.io
skopeo login "$(echo $MIRROR_REGISTRY | cut -d/ -f1)"

# Extract and mirror each unique image
for IMAGE in $(kubectl get manifestwork -n "$CLUSTER_NAME" -o json | \
  jq -r '.. | .image? // empty | select(contains("/"))' | sort -u); do

  # Strip the source registry prefix to get the relative path
  RELATIVE_PATH="${IMAGE#registry.redhat.io/}"

  echo "Mirroring: $IMAGE"
  echo "      To: ${MIRROR_REGISTRY}/${RELATIVE_PATH}"
  skopeo copy --all \
    "docker://${IMAGE}" \
    "docker://${MIRROR_REGISTRY}/${RELATIVE_PATH}"
done

# ─────────────────────────────────────────────────────────────────────
# Step 3: Create a Placement to select the cluster
# ─────────────────────────────────────────────────────────────────────
# ManagedClusterImageRegistry uses a Placement (not direct cluster name)
# to determine which clusters get the mirror config.

echo ""
echo "=== Step 3: Create Placement ==="

cat <<EOF | kubectl apply -f -
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Placement
metadata:
  name: ${CLUSTER_NAME}-registry-placement
  namespace: ${CLUSTER_NAME}
spec:
  predicates:
    - requiredClusterSelector:
        labelSelector:
          matchLabels:
            name: ${CLUSTER_NAME}
  clusterSets:
    - default
EOF

# ─────────────────────────────────────────────────────────────────────
# Step 4: Create the pull secret for the mirror registry
# ─────────────────────────────────────────────────────────────────────
# The ManagedClusterImageRegistry needs a pull secret to authenticate
# against the mirror registry.

echo ""
echo "=== Step 4: Create pull secret ==="

kubectl create secret docker-registry "${CLUSTER_NAME}-registry-pull-secret" \
  --namespace "${CLUSTER_NAME}" \
  --from-file=.dockerconfigjson="${PULL_SECRET_PATH}" \
  --dry-run=client -o yaml | kubectl apply -f -

# ─────────────────────────────────────────────────────────────────────
# Step 5: Create the ManagedClusterImageRegistry
# ─────────────────────────────────────────────────────────────────────
# This CRD tells ACM: "for clusters matched by this Placement, rewrite
# image references from source to mirror before applying manifests."
#
# ACM regenerates the ManifestWorks with the new image references.
# The klusterlet will pull from the mirror instead of registry.redhat.io.

echo ""
echo "=== Step 5: Create ManagedClusterImageRegistry ==="

cat <<EOF | kubectl apply -f -
apiVersion: imageregistry.open-cluster-management.io/v1alpha1
kind: ManagedClusterImageRegistry
metadata:
  name: ${CLUSTER_NAME}-image-registry
  namespace: ${CLUSTER_NAME}
spec:
  placementRef:
    group: cluster.open-cluster-management.io
    resource: placements
    name: ${CLUSTER_NAME}-registry-placement
  pullSecret:
    name: ${CLUSTER_NAME}-registry-pull-secret
  registries:
    - source: registry.redhat.io/multicluster-engine
      mirror: ${MIRROR_REGISTRY}/multicluster-engine
    - source: registry.redhat.io/rhacm2
      mirror: ${MIRROR_REGISTRY}/rhacm2
EOF

# ─────────────────────────────────────────────────────────────────────
# Step 6: Verify
# ─────────────────────────────────────────────────────────────────────
# After applying, ACM regenerates the import manifests. Check the
# ManifestWorks to confirm images now point to the mirror.

echo ""
echo "=== Step 6: Verify image references ==="

sleep 10

kubectl get manifestwork -n "$CLUSTER_NAME" -o json | \
  jq -r '.. | .image? // empty | select(contains("/"))' | sort -u

echo ""
echo "If images now point to ${MIRROR_REGISTRY}, the mirror is working."
echo "Check import status with: kubectl get managedcluster ${CLUSTER_NAME} -o jsonpath='{.status.conditions}' | jq"

# ─────────────────────────────────────────────────────────────────────
# Cleanup (optional)
# ─────────────────────────────────────────────────────────────────────
# To remove the mirror configuration:
#
# kubectl delete managedclusterimageregistry ${CLUSTER_NAME}-image-registry -n ${CLUSTER_NAME}
# kubectl delete placement ${CLUSTER_NAME}-registry-placement -n ${CLUSTER_NAME}
# kubectl delete secret ${CLUSTER_NAME}-registry-pull-secret -n ${CLUSTER_NAME}

# ─────────────────────────────────────────────────────────────────────
# Alternative: Using the acmlab CLI
# ─────────────────────────────────────────────────────────────────────
#
# # List required images
# acmlab registry list-images import-test
#
# # Generate mirror script
# acmlab registry mirror-script import-test --target quay.io/myorg/acm-mirror > mirror.sh
# chmod +x mirror.sh && ./mirror.sh
#
# # Configure mirror
# acmlab registry configure import-test \
#   --mirror quay.io/myorg/acm-mirror \
#   --pull-secret ~/pull-secret.json
#
# # Check status
# acmlab registry status import-test
#
# # Remove mirror
# acmlab registry remove import-test
