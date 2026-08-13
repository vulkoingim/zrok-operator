# AGENTS.md — zrok-operator

Always-on contract for LLM coding agents. Read this first, then the memory bank — **do not** broad-explore `internal/` before consulting docs.

---

## First Steps

1. Read this file
2. Open [`.memory-bank/README.md`](.memory-bank/README.md)
3. Use Quick Lookup → open the matching Tier 2 area doc
4. Drill into Tier 3 component docs only as needed
5. Read source only when docs are insufficient or you need exact lines for an edit
6. After significant changes → update the memory bank ([documentation-maintenance.md](.memory-bank/documentation-maintenance.md))

---

## Documentation Navigation

### Tier 1 — System

| Topic | Doc |
|---|---|
| Overall architecture | [.memory-bank/system-architecture.md](.memory-bank/system-architecture.md) |
| Component catalog | [.memory-bank/components.md](.memory-bank/components.md) |
| Current work | [.memory-bank/active-context.md](.memory-bank/active-context.md) |

### Tier 2 — Areas (start here for most tasks)

| Area | Doc |
|---|---|
| Environment enable + agent | [.memory-bank/areas/environment-lifecycle.md](.memory-bank/areas/environment-lifecycle.md) |
| Share create / adopt / heal / delete | [.memory-bank/areas/share-lifecycle.md](.memory-bank/areas/share-lifecycle.md) |
| Ingress → ZrokShare | [.memory-bank/areas/ingress-translation.md](.memory-bank/areas/ingress-translation.md) |
| Side effects / failure matrix | [.memory-bank/areas/system-effects.md](.memory-bank/areas/system-effects.md) |

### Tier 3 — Components

| Component | Doc |
|---|---|
| Share reconciler | [.memory-bank/components/share-controller.md](.memory-bank/components/share-controller.md) |
| Environment reconciler | [.memory-bank/components/environment-controller.md](.memory-bank/components/environment-controller.md) |
| Agent K8s resources | [.memory-bank/components/agent-resources.md](.memory-bank/components/agent-resources.md) |
| REST + Agent gRPC clients | [.memory-bank/components/zrokclient.md](.memory-bank/components/zrokclient.md) |
| Access reconciler | [.memory-bank/components/access-controller.md](.memory-bank/components/access-controller.md) |

---

## Commands Reference

```bash
mise install                 # tools
mise run gen                 # deepcopy + CRDs + mocks + sync Helm CRDs
mise run test                # unit + envtest (excludes e2e)
mise run lint
mise run build               # bin/manager
mise run kind-up && mise run kind-deploy
mise run test-e2e            # needs Kind; ZROK2_ENABLE_TOKEN for live share
mise run samples --secret    # apply sample CRs
```

Mocks: edit `.mockery.yml` → `mise run gen` (or `mise run gen:mocks`). **Never hand-write mocks.**

---

## Architecture Quick Reference

| Layer | Path | Purpose |
|---|---|---|
| API / CRDs | `api/v1alpha1/` | ZrokEnvironment, ZrokShare, ZrokAccess |
| Manager | `cmd/main.go` | Registers 4 reconcilers |
| Controllers | `internal/controller/` | Env, Share, Access, Ingress |
| Agent resources | `internal/agent/` | Desired Deployment/PVC/Service/naming |
| Clients | `internal/zrokclient/` | REST (controller API) + Agent gRPC |
| Helm | `charts/zrok-operator/` | Install chart + CRDs |
| Samples | `config/samples/` | Example CRs |

---

## Critical Rules

1. **Agent registry wipe is intentional.** Every agent start deletes `agent-registry.json` + `agent.socket`. Operator owns share lifecycle — do not "fix" by persisting the registry.
2. **Reserved names for sticky URLs.** Ephemeral public shares die on agent restart. Prefer `nameSelection` / `agent.ManagedFrontendName` → `ko-<ns>-<share>`.
3. **Always promote reserved:** `CreateShareName` then `UpdateShareName(..., reserved=true)`. CreateShareName 409 = name already exists (treat as OK).
4. **Share ownership is three-way.** Inventory remote `ListShares` + agent Status + CR. Adopt / Unshare only if **target matches** `spec.upstream`. Reserved name held by a different target **or another ZrokShare** → `Ready=False` reason `NameConflict`, **do not Unshare**. Ours remotely + agent empty (registry wipe) → Unshare **our** token then SharePublic.
5. **`nameSelection` only with `shareMode=public`.** `privateShareToken` only with `private`.
6. **Agent replicas must be 1** (Recreate strategy). Manager talks to agent via **gRPC** through socat TCP→unix (`AgentDialAddr`), not HTTP `/v1/agent/*` (README is stale on that point).
7. **Do not hand-write mocks.** Interfaces in `.mockery.yml` → `mise run gen`.
8. **Env delete blocked** while live Shares reference the Environment.
9. **No secrets in docs** (`manifest.yaml` enable tokens stay local/untracked).
10. **Status writes** go through `status.PatchStatus` (MergeFrom + conflict retry), not bare `_ = Status().Update`.
11. **When docs and code disagree, code wins — then update the memory bank immediately.**

---

## Code Style Summary

- Effective Go + project `.cursor/rules/go-coding-standards-and-guidelines.mdc`
- Errors: wrap with `%w`; check `err != nil`
- Lines < 120; `go fmt` / goimports
- Full detail: [.memory-bank/code-standards.md](.memory-bank/code-standards.md)

---

## Memory Bank Maintenance

After significant PRs: follow [.memory-bank/documentation-maintenance.md](.memory-bank/documentation-maintenance.md) (path classification table + update checklist). Refresh `active-context.md` Recent Progress.
