# Documentation Maintenance

> **Last Updated:** 2026-08-12

How humans/agents update this memory bank. Philosophy: [BLUEPRINT.md](BLUEPRINT.md) Parts 6–7.

## When docs MUST update

- New CRD field / condition / finalizer
- Reconciler behavior change (create/heal/delete/409)
- Agent resource shape (seed, registry wipe, ports, images)
- zrokclient interface or auth/media-type change
- mise/CI/Helm/RBAC command or permission change
- Bug fix that revealed a missing invariant
- New ADR

**Docs with the PR.** Code wins conflicts; then fix docs immediately.

## Path classification table

| Changed Path | Memory Bank Target | Priority |
|---|---|---|
| `api/v1alpha1/` | `system-architecture.md`, relevant area + controller component | High |
| `internal/controller/share_controller.go` | `components/share-controller.md`, `areas/share-lifecycle.md` | High |
| `internal/controller/environment_controller.go` | `components/environment-controller.md`, `areas/environment-lifecycle.md` | High |
| `internal/controller/access_controller.go` | `components/access-controller.md` | High |
| `internal/controller/ingress_controller.go` | `areas/ingress-translation.md` | High |
| `internal/agent/` | `components/agent-resources.md`, `areas/environment-lifecycle.md`, ADR if policy change | High |
| `internal/zrokclient/` | `components/zrokclient.md`, areas that call APIs | High |
| `internal/metrics/` | `system-architecture.md` observability | Medium |
| `internal/build/` | `build-deployment.md` (ldflag stamp) | Low |
| `charts/`, `config/`, `.github/`, `mise.toml`, `.mise-tasks/` | `build-deployment.md`, `development-setup.md` | High |
| `.mockery.yml`, `*_test.go`, `test/` | `testing-practices.md` | Medium |
| `adr/` | Cross-link from related components/areas | Medium |
| `AGENTS.md` / `.memory-bank/README.md` | Keep navigation tables in sync | High |

When adding a major package, add a row here **and** a component doc in the same PR.

## Update workflow

1. **Scope** — PR / diff / verbal description / full audit
2. **Classify** — map paths → targets above
3. **Always re-read** — `README.md`, `active-context.md`, `system-architecture.md`, plus mapped files
4. **Edit** — facts, diagrams, failure modes; no TBD placeholders
5. **Refresh index** — README Quick Lookup + File Index match disk
6. **Refresh active-context** — Recent Progress entry with date + links
7. **Sync AGENTS.md** — if new areas/components/commands/critical rules
8. **Verify** — relative links, Mermaid fences, no secrets

## Priority order

1. Component doc(s) for changed code  
2. Area doc(s) if flow changed  
3. `system-architecture.md` / `components.md`  
4. Workflow/reference docs  
5. ADR if new decision  
6. `README.md`  
7. `active-context.md`  
8. `AGENTS.md`  
9. `areas/system-effects.md`

## Archive

Deprecated docs → `.memory-bank/archive/YYYY-MM-DD-<name>/` with a short README why. Remove from active indexes; fix dangling links.

## Quality bar

An agent should implement/debug the common case from docs alone. Include: exact paths, interfaces, error/retry behavior, ≥1 Mermaid per Tier 1–3 doc, known footguns.
