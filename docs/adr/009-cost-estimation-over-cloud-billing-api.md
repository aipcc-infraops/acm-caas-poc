# ADR-009: Estimate costs from node metadata instead of cloud billing APIs

## Status

Accepted

## Context

UC-11 (cost tracking) and UC-17 (budget alerting) need to attribute cloud spend per cluster and per team. There are two approaches: query each cloud provider's billing API for actual spend, or estimate costs from cluster metadata already available in ACM.

Cloud billing APIs (AWS Cost Explorer, IBM Cloud Billing, GCP Cloud Billing, Azure Cost Management) each require separate SDKs, credentials, and IAM permissions. They return data with a 24-48 hour delay, use provider-specific cost models, and would add four new dependencies to the project.

ACM's ManagedClusterInfo already provides node count, instance types, and uptime per cluster. Combined with a pricing table, this is sufficient to estimate compute costs.

## Decision

Estimate costs from ManagedClusterInfo node metadata and a configurable pricing table. Do not integrate with cloud billing APIs in the PoC.

The pricing table maps instance types to hourly rates across IBM Cloud, AWS, GCP, and Azure. Cost is calculated as: `node_count x price_per_hour x hours`. The table uses placeholder rates that can be replaced with real rates via configuration.

## Consequences

- **Pro:** No cloud billing SDK dependencies, consistent with ADR-001 (dynamic client only)
- **Pro:** No additional credentials or IAM configuration required
- **Pro:** Immediate cost data, no 24-48h billing pipeline delay
- **Pro:** Same calculation works across all four cloud providers
- **Con:** Estimates only compute costs, not storage, networking, or data transfer
- **Con:** Does not capture spot/reserved pricing, sustained-use discounts, or credits
- **Con:** Placeholder rates diverge from actual rates over time

## PoC to Production

| PoC | Production |
|-----|------------|
| Pricing table with placeholder rates | Cloud billing API integration (AWS Cost Explorer, GCP Billing, Azure Cost Management, IBM Cloud Billing) per provider |
| Node-count-based cost estimation | Actual spend from billing pipeline, reconciled against estimates |
| Single pricing map in Go source | External configuration (ConfigMap or CRD) with real instance-type rates, refreshed periodically |
| Compute cost only | Storage, networking, data transfer, and licensing costs included |

The attribution model (cost-center labels, team grouping, budget alerting) is cloud-agnostic and carries over unchanged.
