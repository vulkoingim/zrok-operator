# ADR: Wipe agent-registry on agent start

## Status

Accepted

## Context

zrok2 agent persists shares in `agent-registry.json` on the PVC and `ReloadRegistry` calls `SharePublic` on start. The operator also creates/heals shares via gRPC. After pod restart, both tried to own the same reserved frontend name → remote `409 shareConflict` retry loops while the live remote share still held the name.

Alternatives considered:

1. Teach agent/operator to adopt registry entries as SoT
2. Disable registry persistence in upstream agent (not controlled here)
3. Wipe registry (+ socket) before `zrok2 agent start` so operator owns lifecycle

## Decision

Wipe `agent-registry.json` and `agent.socket` on every agent container start (`internal/agent.DesiredDeployment` command). Operator reconciles shares from CR status + REST.

## Consequences

- **Positive:** No ReloadRegistry vs operator race; heal paths are deterministic
- **Negative:** Ephemeral (non-reserved) shares never survive restart — reserved `nameSelection` is required for sticky URLs
- **Follow-up:** Document clearly in samples/AGENTS; keep heal/Unshare robust

## References

- [.memory-bank/components/agent-resources.md](../.memory-bank/components/agent-resources.md)
- [.memory-bank/areas/share-lifecycle.md](../.memory-bank/areas/share-lifecycle.md)
