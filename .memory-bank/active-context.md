# Active Context: zrok-operator

## Current Work Focus

| Focus Area | Description | Status |
|------------|-------------|--------|
| Prom-op controller alignment | Helm Ingress RBAC, Access Env watch+index, status Patch, CEL, metrics, predicates | ✅ 2026-08-12 |
| Memory bank bootstrap | Full Tier 1–3 + AGENTS.md from BLUEPRINT | ✅ 2026-08-12 |
| Share lifecycle harden | Adopt / heal / Unshare orphans / reserved promote | ✅ |
| Agent registry wipe | Operator owns shares; wipe on agent start | ✅ |
| README accuracy | Still claims HTTP agent console for manager | ❌ debt |
| Gateway API | `internal/gateway` placeholder | ❌ not started |

## Recent Progress

- **Prom-op pattern gaps** (2026-08-12): Helm Ingress RBAC; Access Env watch + field indexes + heal; `status.PatchStatus` + condition equality short-circuit; Ingress class predicate; CEL on ZrokShare; labeled Ready gauges + Env/Access error counters; transition-only Ready events. Docs: [access-controller.md](components/access-controller.md), [system-architecture.md](system-architecture.md).

- **Memory bank bootstrap** (2026-08-12): Created `AGENTS.md`, `.memory-bank/` Tier 1–3, reference docs, ADRs. Index: [README.md](README.md).

- **Share heal + reserved names** (2026-08-11 era): Heal inactive tokens via REST Unshare; CreateShareName + UpdateShareName(reserved=true); ManagedFrontendName `ko-<ns>-<share>`; registry wipe on agent start.

### Known local / untracked

- `manifest.yaml`, `kc`, `tmp_stock.go` may exist locally — do not commit secrets/tokens.

## Remaining Gaps (honest)

- No `/update-memory` slash command yet — process documented in [documentation-maintenance.md](documentation-maintenance.md)
- README agent transport docs still stale
- E2E live-share path needs `ZROK2_ENABLE_TOKEN`
