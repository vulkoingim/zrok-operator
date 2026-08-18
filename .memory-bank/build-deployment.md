# Build & Deployment

> **Last Updated:** 2026-08-18

## Version stamp

No `version` file. Version/Date via `-ldflags -X`. GitRevision from `runtime/debug` (`vcs.revision`, `vcs.modified`) when `-buildvcs` is on (mise + GoReleaser). `mise run docker-build` compiles on the host (`-buildvcs=true`) then `Dockerfile.fast` COPY — no git inside the image. The multi-stage `Dockerfile` still accepts `VERSION`/`GIT_REVISION`/`DATE` build-args if you `docker build .` without mise.

| Var | Local (`mise run build` / `docker-build`) | GoReleaser | Multi-stage `Dockerfile` |
|---|---|---|---|
| `Version` | `git describe --tags --always --dirty` | `{{.Version}}` | `--build-arg` |
| `GitRevision` | `debug.ReadBuildInfo` | same (`-buildvcs=true`) | `-X` from host `git rev-parse --short HEAD` |
| `Date` | git committer time (`%cI`) | `{{.Date}}` | `--build-arg` (wall clock if unset) |

`go run` still gets VCS revision (`-buildvcs=auto`) **if you build the package** (`./cmd`, not `cmd/main.go` — file-list builds are `command-line-arguments` and omit VCS). Version stays `dev` unless ldflags. `bin/manager -version` prints them. There is no stdlib `git describe` — `Main.Version` is a module/pseudo-version, not what we stamp.

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
# make kind-up ≡ mise run kind:up  (Make cannot use colons in target names)

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
| `E2E` | `needs: [test]`; **not** on PRs. Runs on `merge_group`, `push` to `main`, or `workflow_dispatch` with `e2e=true`. Live share if repo secret `ZROK2_ENABLE_TOKEN` is set (`ci.yml` already maps it). | restore mise+Go; kindest/node tarball |

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

`.goreleaser.yaml` + `Dockerfile.goreleaser` (COPY pre-built `linux/<arch>/manager` into distroless — do not `go build` in that Dockerfile). Workflow: `goreleaser/goreleaser-action@v7` then `helm push .release/helm/*.tgz oci://ghcr.io/<owner>/charts`. Helm packages to `.release/helm` (not `dist/` — GoReleaser owns `dist` after `--clean`). Chart path is **`charts/zrok-operator`**, not the operator image repo. `helm package --version {{.Version}}` (no `v` prefix). After the first chart push, set the GHCR package **public** if the repo is public: [packages](https://github.com/vulkoingim?tab=packages).

Local image builds (`mise run docker-build` / `kind:load`) cross-compile `bin/manager-linux-<arch>` on the host, then `docker build -f Dockerfile.fast` from a **one-file temp context** (repo `.dockerignore` excludes `bin/`). `--load` so Kind can import the tag. `kind:load` skips import only when the **image ID** is already on the node (same tag + new digest must reload; `imagePullPolicy: IfNotPresent`). `mise run test:e2e` / `make test-e2e` run `kind:load` then set `SKIP_IMAGE_BUILD` + `SKIP_KIND_LOAD` so Ginkgo does not pay docker/kind twice.

## RBAC highlights

ClusterRole `manager-role`: CRDs+status+finalizers; Deployments; PVC/Secret/Service; NetworkPolicies; Pods get/list/watch; `namespaces` get (`kube-system` only, default uniqueID UUID); Ingress+status; core Events + `events.k8s.io/events` create/patch/update.

Helm: leader-election **Role** (leases/configmaps in the release namespace — not ClusterRole). Metrics TokenReview/SubjectAccessReview ClusterRole when `metrics.enabled`. Metrics Service when `metrics.enabled`. Agent NetworkPolicies are **opt-in** (`networkPolicy.enabled`, default false) because not every CNI enforces them.

After EventRecorder migration, missing events RBAC → event writes fail (reconcile may still work).

## Namespace

E2E uses `zrok-operator-system` (typical kubebuilder default layout under `config/default`).
