# Build & Deployment

> **Last Updated:** 2026-08-17 (GoReleaser)

## Artifacts

| Artifact | Source |
|---|---|
| Manager binary | `mise run build` → `bin/manager`; release: linux/amd64 + linux/arm64 archives |
| Container image | `ghcr.io/vulkoingim/zrok-operator` (`{{.Tag}}`, `{{.Version}}`, `latest`) linux/amd64+arm64 |
| Helm chart | `charts/zrok-operator/` packaged onto the GitHub Release (`zrok-operator-<version>.tgz`) |
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

Triggers: `push` to `main` + all `pull_request`. Concurrency cancels superseded runs.

| Workflow | Action | Caching |
|---|---|---|
| `lint.yml` | `setup-go@v6` + `golangci-lint-action@v9` (v2.12) | Go mod/build (setup-go) + golangci analysis cache |
| `test.yml` | `mise-action@v4` → `mise run test` (scoped `install_args`) | mise tools + `~/go/pkg/mod` + `~/.cache/go-build` + `bin/k8s` (envtest) |
| `test-e2e.yml` | `mise-action@v4` → kind-up + `mise run test-e2e` | mise tools + Go mod/build; optional `ZROK2_ENABLE_TOKEN` |
| `release.yml` | tag `v*` | GoReleaser (`dockers_v2` GHCR linux/amd64+arm64 + linux archives + helm tgz) |

CI also runs `go mod verify` + `go mod tidy` + `git diff --exit-code` (no silent tidy).

## Cutting a release

Annotated semver tag, leading `v`. Never retag; retract and roll forward.

```bash
goreleaser check   # optional local validate
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

`.goreleaser.yaml` + `Dockerfile.goreleaser` (COPY pre-built `linux/<arch>/manager` into distroless — do not `go build` in that Dockerfile). Workflow: `goreleaser/goreleaser-action@v7` (`distribution: goreleaser`, `~> v2`), `packages: write` + GHCR login. Helm is packaged in a GoReleaser `before` hook (`azure/setup-helm` on the job).

Local image builds (`mise run docker-build` / kind-deploy) still use the multi-stage `Dockerfile` and **must** `docker build --load` so the tag exists in the daemon Kind uses.

## RBAC highlights

ClusterRole `manager-role`: CRDs+status+finalizers; Deployments; PVC/Secret/Service; Pods get/list/watch; Ingress+status; core Events + `events.k8s.io/events` create/patch/update.

After EventRecorder migration, missing events RBAC → event writes fail (reconcile may still work).

## Namespace

E2E uses `zrok-operator-system` (typical kubebuilder default layout under `config/default`).
