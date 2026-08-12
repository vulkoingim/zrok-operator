# Build & Deployment

> **Last Updated:** 2026-08-12

## Artifacts

| Artifact | Source |
|---|---|
| Manager binary | `mise run build` → `bin/manager` |
| Container image | `ghcr.io/vulkoingim/zrok-operator` (CI on `v*` tags) |
| Helm chart | `charts/zrok-operator/` (packaged on release) |
| CRDs | `config/crd/bases/` + chart `crds/` |

## Install paths

```bash
# Kustomize / Kind
mise run deploy
# or: make deploy IMG=zrok-operator:dev   # runs .mise-tasks/deploy

# Helm
helm upgrade --install zrok-operator ./charts/zrok-operator \
  -n zrok-operator --create-namespace \
  --set image.repository=... --set image.tag=...
```

Prefer pre-created enable-token Secret over chart `--set credentials.enableToken`.

## CI workflows

| Workflow | Trigger | Action |
|---|---|---|
| `test.yml` | PR/push | `mise run test` (via mise-action) |
| `lint.yml` | PR/push | golangci-lint |
| `test-e2e.yml` | PR/push | Kind + `mise run test-e2e` (optional `ZROK2_ENABLE_TOKEN` secret) |
| `release.yml` | tag `v*` | multi-arch GHCR + helm package |

## RBAC highlights

ClusterRole `manager-role`: CRDs+status+finalizers; Deployments; PVC/Secret/Service; Pods get/list/watch; Ingress+status; core Events + `events.k8s.io/events` create/patch/update.

After EventRecorder migration, missing events RBAC → event writes fail (reconcile may still work).

## Namespace

E2E uses `zrok-operator-system` (typical kubebuilder default layout under `config/default`).
