# ADR-007: MinIO for observability object storage in PoC, cloud object storage in production

## Status

Accepted

## Context

ACM's MultiClusterObservability requires S3-compatible object storage for Thanos. The hub runs on IBM Cloud VPC (ROSA HCP). Options:

1. **MinIO in-cluster** — single pod with a PVC, deployed by acmlab
2. **IBM Cloud Object Storage (COS)** — managed service, requires HMAC credentials and bucket setup
3. **OpenShift Data Foundation (ODF)** — NooBaa/Ceph, heavy operator install

## Decision

Three tiers of object storage are supported, selected via `--storage-backend`:

**PoC (minio):** MinIO deployed as a single pod with a PVC (`ibmc-vpc-block-10iops-tier`, 20Gi). The `acmlab observability setup` command deploys MinIO, creates the Thanos secret, and creates the MCO CR — all idempotent. `acmlab observability teardown` removes everything cleanly.

**Lab (obc) — planned:** ObjectBucketClaim (OBC) backed by NooBaa or a compatible CSI driver. When implemented, the `acmlab observability configure-storage --storage-class <sc>` command will create the OBC, wait for it to bind, read the generated bucket name and credentials from the OBC ConfigMap/Secret, and build the Thanos secret automatically. No MinIO pod will be deployed. The OBC storage class will be configurable independently from the volume storage class. This backend is not yet available — the CLI currently accepts the flag but the end-to-end OBC credential discovery is pending.

**Production:** Replace with the cloud provider's native object storage (IBM COS, AWS S3, etc.). The only change is the Thanos secret content — the MCO CR and all other configuration remain identical.

```yaml
# PoC — MinIO
type: s3
config:
  bucket: thanos
  endpoint: minio.open-cluster-management-observability.svc.cluster.local:9000
  insecure: true
  access_key: <minio-access-key>
  secret_key: <minio-secret-key>

# Lab — OBC (planned, not yet implemented)
# type: s3
# config:
#   bucket: <obc-generated-bucket>
#   endpoint: <obc-generated-endpoint>
#   access_key: <obc-generated-key>
#   secret_key: <obc-generated-secret>

# Production — cloud object storage
type: s3
config:
  bucket: <prod-bucket>
  endpoint: <provider-endpoint>
  access_key: <HMAC-access-key>
  secret_key: <HMAC-secret-key>
```

## Consequences

- **Pro:** PoC is self-contained — no external service dependencies
- **Pro:** Setup and teardown are fully automated and idempotent
- **Pro:** Migration to production is a secret change, not an architecture change
- **Pro:** MinIO PVC uses the same storage class as other workloads — no new infra
- **Pro (planned):** OBC path will automate credential discovery — no manual secret assembly (not yet implemented)
- **Con:** MinIO is single-replica, not HA — acceptable for PoC, not for production
- **Con:** MinIO PVC data is lost on teardown — acceptable since Thanos metrics are ephemeral for the PoC
- **Con:** MinIO credentials are hardcoded constants — production must use proper secret management
- **Con:** Backend switch after initial setup is rejected to prevent data loss
