# Debugging & Troubleshooting

> **Last Updated:** 2026-08-13

## Where to look

| Signal | Location |
|---|---|
| Operator logs | manager Deployment |
| Agent logs | `{env}-agent` pod (zrok2 + socat) |
| CR status/conditions | `kubectl get zrokkenv,zrokshare -o yaml` |
| Events | `kubectl describe` / Events API |
| Remote orphans | zrok UI Names + Shares |

## Symptom → fix

### Agent: `409 name already in use` on start / ReloadRegistry

**Cause:** Stale `agent-registry.json` re-SharePublic while remote still holds name.  
**Fix:** Ensure Deployment wipes registry; delete agent pod; operator heal Unshare orphan. See [adr/AGENT_REGISTRY_WIPE.md](../adr/AGENT_REGISTRY_WIPE.md).

### Share Ready=False NameConflict

**Cause:** Reserved frontend name is attached to a remote share whose `target` ≠ `spec.upstream`, and this CR does not own that token.  
**Fix:** Pick a different `nameSelection`, or unshare the other share in the zrok UI. Operator will not steal it.

### Share Ready but empty ShareToken / retry storm

**Cause:** Create appeared remotely but status not persisted; or inactive token.  
**Fix:** Heal path should clear status + Unshare **our** token + recreate. Check share-controller status updates.

### Finalizer stuck on ZrokShare

**Cause:** No ShareToken; DeleteShareName 409 still attached.  
**Fix:** Discover token (agent Status / ListShares by name+target / parse 409 if ours); Release + Unshare; then DeleteShareName. Stripping finalizer is last resort and leaves orphans. Do not Unshare a name holder whose target ≠ this CR.

### Env delete blocked

**Cause:** Shares still reference Environment.  
**Fix:** Delete Shares first.

### Env never Ready

Check: enable token Secret, identity Secret keys, PVC mount, agent probe `/v1/agent/version`, gRPC :7777 via socat, REST Enable errors in Events.

### UI says Reservation ephemeral but name is reserved

Share detail UI can lie; check Names list / overview API `reserved=true`.

### Manager can’t dial agent

Confirm Service `{env}-agent.{ns}.svc:7777`, socat sidecar, agent.socket after wipe+start.

## Related

- [areas/share-lifecycle.md](areas/share-lifecycle.md)
- [areas/system-effects.md](areas/system-effects.md)
