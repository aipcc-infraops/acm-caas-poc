# ADR-013: Unified policy CLI with engine auto-detection from file extension

## Status

Accepted

## Context

UC-29 supports two admission policy engines: Gatekeeper (Rego) and Kyverno (YAML). The initial implementation had 6 separate commands: `apply-rego-policy`, `list-rego-policies`, `remove-rego-policy`, `apply-kyverno-policy`, `list-kyverno-policies`, `remove-kyverno-policy`. This duplication made the CLI surface larger than necessary and would not scale well if a third engine were added.

## Decision

Collapse the 6 engine-specific commands into 3 unified commands: `apply-policy`, `list-policies`, `remove-policy`. The engine is auto-detected from the policy file extension:

- `.rego` → Gatekeeper
- `.yaml` / `.yml` → Kyverno

An `--engine` flag is available for explicit override when the extension is ambiguous or the user wants to force a specific engine. For `remove-policy`, `--engine` is required because there is no file to inspect.

## Consequences

- **Pro:** CLI surface stays constant regardless of how many engines are supported
- **Pro:** Users who only work with one engine never need to think about the other
- **Pro:** Adding a third engine (e.g. Falco, Datree) requires extending the detection logic, not adding new commands
- **Con:** `.yaml` defaulting to Kyverno could surprise users passing non-Kyverno YAML; mitigated by the `--engine` override
- **Con:** `remove-policy` requires `--engine` because there is no file to infer from

## Notes

The auto-detection logic lives in `cmd/acmlab/security.go`. The underlying `internal/security/` package keeps engine-specific methods (`ApplyCustomPolicy` for Gatekeeper, `ApplyKyvernoPolicy` for Kyverno) — the unification is a CLI concern, not a domain concern.
