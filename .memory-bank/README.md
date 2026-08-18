# zrok-operator Memory Bank

Comprehensive documentation for zrok-operator, optimized for LLM agent consumption.
Read this file FIRST to find what you need without exploring source code.

> **Last Updated:** 2026-08-18

---

## How to Use This Memory Bank

1. **Start here.** Use Quick Lookup or File Index below.
2. **Don't explore code first.** Prefer these docs; open source only when docs are insufficient.
3. **Keep it updated.** After changes, follow [documentation-maintenance.md](documentation-maintenance.md).
4. **Diagrams are first-class.** Render Mermaid to understand flows visually.

Entry contract: [AGENTS.md](../AGENTS.md) → this index → Tier 2 area → Tier 3 as needed.

---

## Quick Lookup

| If you need to know about... | Read this file |
|---|---|
| Overall architecture | [system-architecture.md](system-architecture.md) |
| All components | [components.md](components.md) |
| Current work in flight | [active-context.md](active-context.md) |
| Env enable / agent PVC+Deploy | [areas/environment-lifecycle.md](areas/environment-lifecycle.md) |
| Share create/adopt/heal/delete / 409 | [areas/share-lifecycle.md](areas/share-lifecycle.md) |
| Ingress class `zrok` | [areas/ingress-translation.md](areas/ingress-translation.md) |
| Failure knock-ons | [areas/system-effects.md](areas/system-effects.md) |
| Share reconciler internals | [components/share-controller.md](components/share-controller.md) |
| Environment reconciler | [components/environment-controller.md](components/environment-controller.md) |
| Agent Deployment/seed/registry wipe | [components/agent-resources.md](components/agent-resources.md) |
| REST + gRPC clients | [components/zrokclient.md](components/zrokclient.md) |
| Private access consumer | [components/access-controller.md](components/access-controller.md) |
| Local setup / Kind | [development-setup.md](development-setup.md) |
| Tests / mocks | [testing-practices.md](testing-practices.md) |
| CI / Helm / release | [build-deployment.md](build-deployment.md) |
| Ops debugging (409, 401 NameConflict, stuck finalizer) | [debugging-troubleshooting.md](debugging-troubleshooting.md) |
| Go style | [code-standards.md](code-standards.md) |
| How to update this bank | [documentation-maintenance.md](documentation-maintenance.md) |
| Blueprint (meta) | [BLUEPRINT.md](BLUEPRINT.md) |

---

## Area Docs (`areas/`)

| Area | File | Covers |
|---|---|---|
| Environment lifecycle | [environment-lifecycle.md](areas/environment-lifecycle.md) | Enable, identity Secret, PVC, agent start, Ready, delete |
| Share lifecycle | [share-lifecycle.md](areas/share-lifecycle.md) | Public/private/reserved, adopt, heal, delete, 409 paths |
| Ingress translation | [ingress-translation.md](areas/ingress-translation.md) | Ingress class → owned ZrokShare → LB hostname |
| System effects | [system-effects.md](areas/system-effects.md) | Flywheel, side effects, failure matrix |

---

## File Index

### Root Level

| File | Purpose | Key Topics |
|---|---|---|
| [system-architecture.md](system-architecture.md) | Tier 1 overview | Topology, flows, stores, risks |
| [architecture.md](architecture.md) | Diagram-heavy companion | Mermaid map |
| [components.md](components.md) | Component directory | Links to Tier 3 |
| [active-context.md](active-context.md) | Working memory | In-flight work |
| [documentation-maintenance.md](documentation-maintenance.md) | Update process | Path map, checklist |
| [development-setup.md](development-setup.md) | Local env | mise, Kind, samples |
| [development-workflow.md](development-workflow.md) | Git / PR | Gen before commit |
| [testing-practices.md](testing-practices.md) | Unit/envtest/e2e | mockery |
| [build-deployment.md](build-deployment.md) | CI/CD | GHCR, Helm |
| [debugging-troubleshooting.md](debugging-troubleshooting.md) | Ops | 409, heal, finalizers |
| [code-standards.md](code-standards.md) | Style | Go conventions |
| [tech-context.md](tech-context.md) | Stack | Go 1.26, kubebuilder, zrok2 |
| [projectbrief.md](projectbrief.md) | Goals / non-goals | Scope |
| [BLUEPRINT.md](BLUEPRINT.md) | Meta pattern | How to build memory banks |

### Component Docs

| File | Purpose | Key Topics |
|---|---|---|
| [share-controller.md](components/share-controller.md) | ZrokShareReconciler | Adopt/heal/delete |
| [environment-controller.md](components/environment-controller.md) | ZrokEnvironmentReconciler | Enable/Disable |
| [agent-resources.md](components/agent-resources.md) | Desired K8s resources | Seed, registry wipe |
| [zrokclient.md](components/zrokclient.md) | REST + AgentClient | Interfaces |
| [access-controller.md](components/access-controller.md) | ZrokAccessReconciler | AccessPrivate |

### ADRs (`adr/`)

| File | Decision |
|---|---|
| [AGENT_REGISTRY_WIPE.md](../adr/AGENT_REGISTRY_WIPE.md) | Wipe agent-registry on start; operator owns shares |
| [RESERVED_FRONTEND_NAMES.md](../adr/RESERVED_FRONTEND_NAMES.md) | Sticky URLs via CreateShareName + reserved=true |

---

## Project Structure Quick Reference

```
zrok-operator/
├── api/v1alpha1/           # CRD types + conditions/finalizers
├── cmd/main.go             # Manager entry (4 reconcilers)
├── internal/
│   ├── agent/              # Desired Deployment/PVC/Service helpers
│   ├── controller/         # Env, Share, Access, Ingress reconcilers
│   ├── zrokclient/         # REST controller API + Agent gRPC
│   ├── status/             # Condition helpers
│   ├── metrics/            # Prometheus (partially wired)
│   ├── build/              # Version/Date ldflags; GitRevision from runtime/debug
├── config/                 # Kustomize CRDs, RBAC, samples
├── charts/zrok-operator/   # Helm chart
├── test/e2e/               # Kind e2e
├── .mise-tasks/            # Longer mise scripts
├── adr/                    # Architecture Decision Records
└── .memory-bank/           # This documentation tree
```

---

## Key Integration Points

| From | To | Protocol | Key Files |
|---|---|---|---|
| Environment reconciler | zrok controller API | HTTPS REST `/api/v2` | `internal/zrokclient/client.go` |
| Share/Access/Env Ready check | zrok2 agent | gRPC via socat `:7777`→unix | `internal/zrokclient/agent_grpc.go` |
| Agent pod | zrok controller | zrok2 CLI / agent | `internal/agent/resources.go` |
| Ingress reconciler | ZrokShare CR | K8s API (owns) | `internal/controller/ingress_controller.go` |
| Manager probes (agent) | Agent console | HTTP `:8888` `/v1/agent/version` | Deployment probes only |

---

## Updating This Memory Bank

See [documentation-maintenance.md](documentation-maintenance.md). Path classification table lives there. Bump **Last Updated** on touched docs and add a Recent Progress line in [active-context.md](active-context.md).
