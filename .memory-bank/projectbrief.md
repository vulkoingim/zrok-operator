# Project Brief

> **Last Updated:** 2026-08-12

## Goals

- Expose in-cluster Services via zrok/v2 with Kubernetes CRDs
- Own share lifecycle in the operator (survive agent restarts via reserved names)
- Complement (not replace) the official zrok2 server Helm chart

## Non-goals

- Deploying zrok controller / frontend / OpenZiti
- Multi-cluster or SaaS control plane
- Full Gateway API support
- Persisting agent local registry as source of truth

## Constraints

- Agent gRPC is unix-socket-only upstream → socat TCP proxy required
- zrok Share API has no free-form metadata
- One agent replica per Environment
- Go 1.26, kubebuilder/controller-runtime, Kind-based e2e

## Success signals

- `ZrokEnvironment` Ready → Shares get stable reserved URLs
- Delete Share cleans agent + remote without stuck finalizers
- Agents can implement share/env changes from memory bank without rediscovering 409 footguns
