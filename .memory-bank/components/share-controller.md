# Component: Share Controller

- **Location**: `internal/controller/share_controller.go`, `share_inventory.go`
- **Purpose**: Reconcile `ZrokShare` create/adopt/heal/delete against agent gRPC + controller REST
- **Owner**: manager (`zrokshare-controller` recorder)
- **Last analysed**: 2026-08-13

## Data Flow Diagram

```mermaid
flowchart LR
  CR[ZrokShare] --> R[ZrokShareReconciler]
  Env[ZrokEnvironment] -->|watch map| R
  R --> REST[RESTClient]
  R --> AG[AgentClient]
  REST --> API[api-v2 ListShares / Names / Unshare]
  AG --> Agent[agent :7777]
  R --> Status[CR status + labels + conditions]
```

## Business Intent & Domain Logic

### Purpose (The WHY)

Kubernetes-native share lifecycle so users don’t run `zrok2 share` by hand, and so reserved names survive agent restarts (registry wipe) **without stealing someone else's share**.

### Business Rules Enforced

| Rule | Implementation | Impact if Violated |
|------|----------------|--------------------|
| nameSelection ⇒ public | Spec validation early in reconcile | Invalid Ready=False |
| privateShareToken ⇒ private | Spec validation + CEL | Invalid Ready=False |
| CreateShareName then reserved=true | REST calls after inventory, skipped on NameConflict | Name stays ephemeral remotely |
| Adopt only active agent shares with matching target | `classifyShare` + `agentTargetOK` | Adopt dead/wrong share → fail or steal |
| Unshare only owned tokens | `TargetsEqual` vs `spec.upstream`, or status token | Steal foreign reserved name |
| Persist status clear on heal | Status update before recreate | In-memory clear → stuck 409 |

### Critical Edge Cases

| Edge Case | How It's Handled | What Could Go Wrong |
|-----------|------------------|---------------------|
| Name held, different target | NameConflict; no Unshare | Old code Unshared blindly |
| Name held, same target, agent empty | Unshare our token + SharePublic | 409 loop if Unshare fails |
| SharePublic 409 | Re-inventory; same ownership rules | |
| Token in status, not in agent | Heal if ours (target or status token) | |
| Delete without token | Discover via name+target only | |
| DeleteShareName 409 foreign | Leave name; NameRetained event | |

### Invariants

- Finalizer `zrok.k8s.zrok.io/share` until remote+agent cleanup done (or reclaim Retain semantics for name delete)
- `status.reservation` ∈ {ephemeral, reserved, private}
- Does not Own Deployments — only Env does
- CR labels from `agent.ShareLabels`

### Assumptions & Limitations

- zrok Share API has no metadata/tags — cannot push description/labels to shares
- Env Enable still gets description+host `zrok-operator/{ns}/{name}`
- Ephemeral shares are intentionally non-sticky under registry wipe
- Ready requeue 2m assumes eventual consistency with agent Status

## Dependencies

| Dependency | Type | Details |
|------------|------|---------|
| `zrokclient.RESTClient` | interface | Names, Unshare, **ListShares** |
| `zrokclient.AgentClient` | interface | SharePublic/Private, ReleaseShare, Status |
| `internal/agent` | helpers | ShareLabels, ManagedFrontendName, dial addr |
| Env Ready | precondition | Share waits until AgentReady |

## Dependents

| Dependent | How |
|-----------|-----|
| Ingress reconciler | Creates ZrokShare with ShareLabels; reads AssignedURL |
| Users / samples | Direct ZrokShare CRs |

## Interface

Reconciler entry: `Reconcile(ctx, req) (Result, error)` via controller-runtime.

Key internals:

- `loadInventory` / `classifyShare` (`share_inventory.go`)
- `ensureReservedName`: `CreateShareName` + `UpdateShareName(..., true)`
- `releaseOurs` / `handleShareConflict`
- Delete: resolve owned token → Release → Unshare → DeleteShareName

Conditions used: `Ready`, `EnvironmentReady`, `ShareCreated`, `NameReady`. Ready reason `NameConflict` when the reserved name is foreign.

## Configuration

- Env `spec.apiEndpoint` + identity Secret account token
- Agent dial: `agent.AgentDialAddr(env)` → `{env}-agent.{ns}.svc:7777`
- Requeues: Ready 2m; NameConflict 2m; rebind ~2s; name attach ~3s

## Error Handling & Retries

- REST CreateShareName 409 → nil (idempotent)
- Unshare/DeleteShareName 404 → nil
- SharePublic 409 → inventory path, not blind Unshare
- Reconcile errors → event `ShareError`; metric `zrok_share_reconcile_errors`

## Concurrency & Safety

Single reconcile per object via controller-runtime workqueue; leader election on manager. No extra locks.

## Observability

Events: `ShareError`, `ReleaseError`, `Ready`, `NameConflict`, `NameRetained`. Logs around inventory/heal/delete.

## Testing Approach

- `share_inventory_test.go` — classify/foreign/target (testing.T)
- `share_controller_test.go` — envtest + testify mocks (`ListShares`, Unshare, SharePublic)
- `internal/zrokclient/share_test.go` — `TargetsEqual` / name match

## Common Issues / Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Ready=False NameConflict | Reserved name attached to another target | Change nameSelection or free the other share in the UI |
| 409 name in use loop | Should be gone; check inventory Unshare of **ours** only | |
| Ready without ShareToken | Status not persisted / race | Check heal path; status update errors |
| Finalizer stuck | Token unknown + DeleteShareName 409 ours | Discover by target; Unshare; don’t strip finalizer unless last resort |
| Ephemeral after restart | No nameSelection | Add reserved name |

## References

- [areas/share-lifecycle.md](../areas/share-lifecycle.md)
- [components/zrokclient.md](zrokclient.md)
- [adr/RESERVED_FRONTEND_NAMES.md](../../adr/RESERVED_FRONTEND_NAMES.md)
