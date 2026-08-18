# Tech Context

> **Last Updated:** 2026-08-18

| Item | Value |
|---|---|
| Language | Go 1.26 (`mise.toml` / `go.mod`) |
| Module | `github.com/vulkoingim/zrok-operator` |
| Operator SDK style | kubebuilder + controller-runtime |
| API | `zrok.k8s.zrok.io/v1alpha1` |
| zrok agent image | `docker.io/openziti/zrok2:2.0.4` (default) |
| Agent gRPC proxy | `docker.io/alpine/socat:1.8.1.3` |
| Default API | `https://api-v2.zrok.io` |
| Tooling | mise, controller-gen v0.17.2, mockery v3, golangci-lint, kind, helm, kustomize |
| Envtest | K8s 1.36 assets |
| Tests | gotestsum, Ginkgo (controller suite / e2e), testify mocks |
| Image registry | `ghcr.io/vulkoingim/zrok-operator` (GoReleaser `dockers_v2`, linux/amd64+arm64) |
| Chart | `oci://ghcr.io/vulkoingim/charts/zrok-operator` (Chart.yaml 0.0.1 is overridden at release) |

## External systems

- **zrok controller API** — REST `/api/v2`, auth `X-TOKEN`, media `application/zrok.v1+json`
- zrok2 agent — gRPC over unix socket; HTTP console binds 127.0.0.1 (not on Service); kubelet probes TCP `:7777`

## Codegen

```bash
mise run gen   # deepcopy + CRDs + mocks + copy CRDs → charts/.../crds/
```
