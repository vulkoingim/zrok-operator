# ADR: Reserved frontend names for sticky shares

## Status

Accepted

## Context

zrok/v2 uses namespace/name reservations (not v1 `reserve`). Without a reserved name, SharePublic gets a random frontend name. Combined with [AGENT_REGISTRY_WIPE](AGENT_REGISTRY_WIPE.md), ephemeral shares churn every agent restart.

Users need stable URLs (`*.shares.zrok.io` style) and a visible signal that a share is operator-managed (Share API has no metadata field).

## Decision

1. For sticky public shares: require `spec.nameSelection` (`namespace` + `name`)
2. Always `CreateShareName` then `UpdateShareName(..., reserved=true)` to promote
3. Convention helper `agent.ManagedFrontendName` → `ko-<k8s-namespace>-<share-name>`
4. Env description `zrok-operator/<ns>/<env>` identifies the environment in UI
5. `status.reservation` reports `ephemeral|reserved|private` (status-only — never put reservation in spec)

## Consequences

- Stable URLs across agent restarts
- UI identification via name prefix `ko-` + env description
- Callers must unique-ify names across the zrok account/namespace token
- CreateShareName 409 treated as success (idempotent ensure)

## References

- [.memory-bank/areas/share-lifecycle.md](../.memory-bank/areas/share-lifecycle.md)
- `config/samples/zrok_v1alpha1_environment_share.yaml`
