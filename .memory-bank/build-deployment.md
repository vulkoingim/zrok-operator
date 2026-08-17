# Build & Deployment

> **Last Updated:** 2026-08-17 (CI caches)

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
| `lint.yml` | `mise-action@v4` → `golangci-lint config verify` + `mise run lint` | shared mise tool cache (binary) + Go cache before mise + `~/.cache/golangci-lint` |
| `test.yml` | `mise-action@v4` (full `[tools]`) → `mise run test` | mise tools (shared key w/ e2e) + Go cache **before** mise + `bin/k8s` |
| `test-e2e.yml` | same mise cache → kind-up + `mise run test-e2e` | Go cache before mise; kindest/node tarball; buildx `type=gha` for docker-build |
| `release.yml` | tag `v*` | GoReleaser (`dockers_v2` GHCR linux/amd64+arm64 + linux archives + helm tgz) |

CI also runs `go mod verify` + `go mod tidy` + `git diff --exit-code` (no silent tidy).

Do **not** pass `install_args` to mise-action: it hashes into the cache key (splits jobs) and only saves tools installed in that step. `mise run` auto-installs the rest of `[tools]` *after* the cache is written. Job env `MISE_TASK_RUN_AUTO_INSTALL=false` stops that. Go module `actions/cache` must restore **before** mise — `go:` packages make `~/go/pkg/mod` read-only and a later tar restore dies with `File exists`. `golangci-lint-action` always re-downloads the binary even when its analysis cache hits; lint uses mise for the binary.

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
