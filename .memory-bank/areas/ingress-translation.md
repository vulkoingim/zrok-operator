# Area: Ingress Translation

> **Last Updated:** 2026-08-18

## Overview

Maps Kubernetes Ingress objects with `ingressClassName: zrok` into owned `ZrokShare` CRs and mirrors `status.assignedURL` onto the Ingress load-balancer hostname. Convenience UX over writing ZrokShare YAML.

## Architecture Diagram

```mermaid
sequenceDiagram
  participant U as User
  participant I as IngressReconciler
  participant K as API server
  participant S as ZrokShareReconciler

  U->>K: Ingress ingressClassName=zrok
  I->>I: filter class == zrok
  I->>K: ensure owned ZrokShare same name/ns
  Note over I: annotations → EnvironmentRef, nameSelection
  S->>S: normal share lifecycle
  S->>K: status.assignedURL
  I->>K: Ingress status loadBalancer.hostname = assignedURL
```

## How It Works

1. Watch Ingress; process only `ingressClassName == "zrok"`
2. Build desired `ZrokShare` (same name as Ingress) from rules/backends + annotations; stamp `agent.ShareLabels`. `nameSelection.name` is the DNS label; `ingressReservedName` strips `.shares.zrok.io` / `.share.zrok.io` from Host or the annotation if someone pastes the FQDN.
3. `SetControllerReference` — Owns ZrokShare
4. When Share has `AssignedURL`, copy to Ingress status LB hostname

### Annotations

| Annotation | Purpose | Default |
|---|---|---|
| `zrok.k8s.zrok.io/environment` | ZrokEnvironment name | `default` |
| `zrok.k8s.zrok.io/name` | Reserved frontend **DNS label** (not the FQDN) | (from `rules[0].host`, stripping `.shares.zrok.io`) |
| `zrok.k8s.zrok.io/namespace-token` | Namespace token | `public` |

Sample: `config/samples/ingress_zrok.yaml`.

## Business Rules

1. Only class `zrok` is managed
2. Share name = Ingress name (same namespace)
3. Share lifecycle rules still apply (prefer reserved names)

## Edge Cases & Failure Modes

| Scenario | Behavior |
|---|---|
| Missing/invalid annotation combo | Event `InvalidIngress`; Share may not Ready |
| Host is `foo.shares.zrok.io` | Stored as `nameSelection.name=foo` |
| Env not Ready | Share waits; Ingress hostname empty |
| Ingress deleted | Owned Share deleted (GC) |

## Related Docs

- [areas/share-lifecycle.md](share-lifecycle.md)
- Source: `internal/controller/ingress_controller.go`
