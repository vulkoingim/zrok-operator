# Active Context: zrok-operator

## Current Work Focus

| Focus Area | Description | Status |
|------------|-------------|--------|
| Share lifecycle / ownership | Three-way inventory; nameSelection rename; NameConflict | ✅ 2026-08-18 |
| Prom-op controller alignment | Helm Ingress RBAC, Access Env watch+index, status Patch, CEL, metrics, predicates | ✅ 2026-08-12 |
| Memory bank bootstrap | Full Tier 1–3 + AGENTS.md from BLUEPRINT | ✅ 2026-08-12 |
| Share lifecycle harden | Adopt / heal / Unshare orphans / reserved promote | ✅ |
| Agent registry wipe | Operator owns shares; wipe on agent start | ✅ |
| README accuracy | Still claims HTTP agent console for manager | ❌ debt |
| Gateway API | HTTPRoute translation | ❌ not started |

## Recent Progress

- **Share reserved-name rename** (2026-08-18): `spec.nameSelection.name` is the DNS label (not the FQDN). Changing it Unshares **ours**, `DeleteShareName`s the old label (unless Retain), reserves+SharePublics the new name. Adopt now requires frontend name / shareMode / backendMode / closed match; oauth/basicAuth/insecure/grants via `applied-digest`. Access `shareToken` / explicit bind rebuilds. Ingress strips `.shares.zrok.io` from Host. DeleteShareName **401** → NameRetained + drop finalizer. Live e2e waits on `d1592fdb60580fe884c3e43946d9` and recreates the enable-token Secret after sample apply.

- **Hardening** (2026-08-18): https+allowlist `apiEndpoint` (drop `X-TOKEN` on redirect); agent image allowlist; ClusterIP-only agent Service; localhost console; `automountServiceAccountToken: false`; unowned identity Secret deleted; Helm `networkPolicy.enabled` (default false) gates manager NP + `--agent-network-policy`; namespaced leader-election Role; metrics TokenReview RBAC + metrics Service; e2e enable token via stdin (argv redacted). `--restrict-upstream` optional.

- **Dead scaffold purge** (2026-08-18): no cert-manager/prometheus-operator e2e; dropped unused `config/prometheus`, `config/network-policy`, cert-manager metrics patch, samples kustomization, webhook wiring in `cmd/main.go`, unused `internal/gateway` stub. E2e is manager + `/metrics` + optional live share.

- **E2E `make install` parse fail** (2026-08-18): GNU Make `.PHONY: kind:up` → `target pattern contains no '%'`. Wrappers are `kind-up` → `.mise-tasks/kind/up`. E2e Ginkgo still calls `make install`.

- **Fresh env apply races** (2026-08-18): Env `isAgentReady` Get NotFound after Create → WaitingForAgent (not reconcile error). Share `UpdateShareName` 401 → NameConflict (reserved name owned by another zrok account; public names are globally unique). CreateShareName 409 is swallowed first.

- **UniqueID Enable host** (2026-08-18): `{uniqueID}/zrok-operator/{ns}/{name}`. `spec.uniqueID` overrides; default is the kube-system Namespace UUID. Status records the prefix used at Enable.

- **Version embed** (2026-08-18): `internal/build` + git-describe ldflags on `mise run build` / docker-build / GoReleaser. `bin/manager -version`. No version file.

- **Helm OCI** (2026-08-18): `helm push` to `oci://ghcr.io/vulkoingim/charts` after GoReleaser. Distinct from operator image path.

- **Single CI workflow** (2026-08-18): `ci.yml` — Lint (writes mise cache) → Test → E2E only on merge queue / `main` / manual dispatch. See [build-deployment.md](build-deployment.md).

- **CI caches** (2026-08-17): Go restore before mise. Shared mise `[tools]` cache; only `test.yml` saves mise+Go keys (lint/e2e restore-only — GHA forbids parallel save of the same key). Kind node image + buildx GHA layer cache on e2e. Lint uses mise for the golangci-lint binary.

- **GoReleaser + e2e image load** (2026-08-17): `.goreleaser.yaml` `dockers_v2` publishes linux/amd64+arm64 to GHCR; helm tgz on the GitHub Release. E2E Kind failure was `example.com/zrok-operator:v0.0.1` not in the daemon — `--load` + shared `IMG` + `SKIP_IMAGE_BUILD`, not a registry push. See [build-deployment.md](build-deployment.md).

- **Share ownership inventory** (2026-08-13): `ListShares` + agent Status + CR. Adopt/Unshare only if target matches `spec.upstream`. Foreign reserved name **or another ZrokShare claiming the same name** → NameConflict (no Unshare). Empty agent backend is not a match. ListShares failure keeps Ready if the agent still serves the share. NameConflict events are transition-only.

- **CI speed/correctness** (2026-08-12): lint → setup-go@v6 + golangci-lint-action@v9; test/e2e → mise-action@v4 (pinned 2026.8.2, scoped install_args) + Go/envtest caches; concurrency + main/PR-only triggers; tidy gate. See [build-deployment.md](build-deployment.md).

- **Prom-op pattern gaps** (2026-08-12): Helm Ingress RBAC; Access Env watch + field indexes + heal; `status.PatchStatus` + condition equality short-circuit; Ingress class predicate; CEL on ZrokShare; labeled Ready gauges + Env/Access error counters; transition-only Ready events. Docs: [access-controller.md](components/access-controller.md), [system-architecture.md](system-architecture.md).

- **Memory bank bootstrap** (2026-08-12): Created `AGENTS.md`, `.memory-bank/` Tier 1–3, reference docs, ADRs. Index: [README.md](README.md).

- **Share heal + reserved names** (2026-08-11 era): Heal inactive tokens via REST Unshare; CreateShareName + UpdateShareName(reserved=true); ManagedFrontendName `ko-<ns>-<share>`; registry wipe on agent start.

### Known local / untracked

- `manifest.yaml`, `kc`, `tmp_stock.go` may exist locally — do not commit secrets/tokens.

## Remaining Gaps (honest)

- No `/update-memory` slash command yet — process documented in [documentation-maintenance.md](documentation-maintenance.md)
- README agent transport docs still stale
- E2E live-share uses repo secret `ZROK2_ENABLE_TOKEN` on main / merge queue / dispatch (not PRs). Empty/missing → Skip.
- Changing `ZrokShare`/`ZrokAccess` `environmentRef` after create does not Unshare/Release on the **old** env (new env heals; old agent may keep the share until registry wipe)
- `spec.uniqueID` / `spec.apiEndpoint` after Enable do not re-Enable (by design)
