# Build & Deployment

> **Last Updated:** 2026-08-18 (single CI workflow)

## Artifacts

| Artifact | Source |
|---|---|
| Manager binary | `mise run build` → `bin/manager`; release: linux/amd64 + linux/arm64 archives |
| Container image | `ghcr.io/vulkoingim/zrok-operator` (`{{.Tag}}`, `{{.Version}}`, `latest`) linux/amd64+arm64 |
| Helm chart | `oci://ghcr.io/vulkoingim/charts/zrok-operator` + GitHub Release tgz |
| CRDs | `config/crd/bases/` + chart `crds/` |

## Install paths

```bash
# Kustomize / Kind
mise run deploy
# or: make deploy IMG=zrok-operator:dev   # runs .mise-tasks/deploy

# Helm
helm upgrade --install zrok-operator oci://ghcr.io/vulkoingim/charts/zrok-operator \
  --version <semver without v> \
  -n zrok-operator --create-namespace
# local chart: ./charts/zrok-operator --set image.repository=... --set image.tag=...
```

Prefer pre-created enable-token Secret over chart `--set credentials.enableToken`.

## CI workflows

One workflow: `.github/workflows/ci.yml`. `release.yml` stays on `v*` tags.

Triggers: `pull_request`, `push` to `main`, `merge_group` (merge queue), `workflow_dispatch`. Concurrency cancels superseded PR/branch runs, **not** merge-queue runs.

| Job | When | Caching |
|---|---|---|
| `Lint` | always | writes shared mise tool cache; golangci analysis |
| `Test` | `needs: [lint]` | restores mise; writes Go mod/build cache on miss; envtest `bin/k8s` |
| `E2E` | `needs: [test]`; **not** on PRs. Runs on `merge_group`, `push` to `main`, or `workflow_dispatch` with `e2e=true` | restore mise+Go; kindest/node tarball; buildx `type=gha` |

Do **not** pass `install_args` to mise-action (splits the cache key). `MISE_TASK_RUN_AUTO_INSTALL=false` so `mise run` does not install extra tools after the action. Go restore **before** mise (`go:` tools make `pkg/mod` read-only; later tar restore → `File exists`). Lint writes mise; Test writes Go — jobs are sequential so they cannot race the same key.

Required checks: `CI / Lint` + `CI / Test` on PRs. **Do not** require `CI / E2E` on pull requests (the job is skipped → required check stays pending). Require `CI / E2E` on the [merge queue](https://github.com/vulkoingim/zrok-operator/settings/branches) if you want Kind before merge. Enable merge queue on `main` and add that check there.

Release stays in `release.yml` (`v*` tags → GoReleaser). Test job also runs `go mod verify` + `go mod tidy` + `git diff --exit-code`.

## Cutting a release

Annotated semver tag, leading `v`. Never retag; retract and roll forward.

```bash
goreleaser check   # optional local validate
git tag -a v0.0.1 -m "v0.0.1"
git push origin v0.0.1
```

`.goreleaser.yaml` + `Dockerfile.goreleaser` (COPY pre-built `linux/<arch>/manager` into distroless — do not `go build` in that Dockerfile). Workflow: `goreleaser/goreleaser-action@v7` then `helm push dist/helm/*.tgz oci://ghcr.io/<owner>/charts`. Chart path is **`charts/zrok-operator`**, not the operator image repo. `helm package --version {{.Version}}` (no `v` prefix). After the first chart push, set the GHCR package **public** if the repo is public: [packages](https://github.com/vulkoingim?tab=packages).

Local image builds (`mise run docker-build` / kind-deploy) still use the multi-stage `Dockerfile` and **must** `docker build --load` so the tag exists in the daemon Kind uses.

## RBAC highlights

ClusterRole `manager-role`: CRDs+status+finalizers; Deployments; PVC/Secret/Service; Pods get/list/watch; Ingress+status; core Events + `events.k8s.io/events` create/patch/update.

After EventRecorder migration, missing events RBAC → event writes fail (reconcile may still work).

## Namespace

E2E uses `zrok-operator-system` (typical kubebuilder default layout under `config/default`).
