# Development Setup

> **Last Updated:** 2026-08-17

## Prerequisites

- [mise](https://mise.jdx.dev/) (`mise install` from repo root)
- Docker (Kind / image builds)
- Optional: `ZROK2_ENABLE_TOKEN` for live share e2e / samples

## Install tools

```bash
mise install
mise tasks
```

## Smoke

```bash
mise run gen
mise run test
mise run build
```

## Kind cluster

```bash
mise run kind-up
mise run kind-deploy   # docker build --load, kind load, deploy
# Kind needs the tag in the local docker daemon (`--load`). A GHCR push is not required.
kubectl create secret generic zrok-credentials \
  --from-literal=enable-token="$ZROK2_ENABLE_TOKEN"
mise run samples --secret
# or: kubectl apply -f config/samples/zrok_v1alpha1_environment_share.yaml
```

Prefer kube context `kind-kind` when debugging local Kind.

## Self-hosted zrok

Set `ZrokEnvironment.spec.apiEndpoint` to your controller URL. Token from ziggy account secret (see root README).

## Common setup failures

| Failure | Check |
|---|---|
| envtest binary missing | `mise run test` runs `setup-envtest` |
| CRDs outdated | `mise run gen` then reinstall |
| Agent 409 on start | Image includes registry wipe; delete agent pod |
| Enable fails | Token Secret key `enable-token`; API reachable |

Long scripts: `.mise-tasks/`. Makefile targets still work.
