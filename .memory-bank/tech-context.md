# Tech Context

> **Last Updated:** 2026-08-12

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
| Image registry | `ghcr.io/vulkoingim/zrok-operator` |
| Chart | `charts/zrok-operator` version 0.1.0 |

## External systems

- **zrok controller API** — REST `/api/v2`, auth `X-TOKEN`, media `application/zrok.v1+json`
- **zrok2 agent** — gRPC over unix socket; console HTTP for version probe

## Codegen

```bash
mise run gen   # deepcopy + CRDs + mocks + copy CRDs → charts/.../crds/
```
