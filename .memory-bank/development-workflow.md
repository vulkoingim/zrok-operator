# Development Workflow

> **Last Updated:** 2026-08-12

## Before commit

```bash
mise run gen
mise run fmt
mise run vet
mise run test
mise run lint
```

## Codegen ownership

- CRD/RBAC markers on types/reconcilers → `controller-gen` via `mise run manifests`
- DeepCopy → `mise run generate`
- Mocks → `.mockery.yml` + `mise run gen:mocks`
- Helm CRDs → copied from `config/crd/bases/` on `mise run gen`

## PR expectations

- Update memory bank when behavior/commands change ([documentation-maintenance.md](documentation-maintenance.md))
- No hand-written mocks
- No secrets (`manifest.yaml` tokens stay local)
- Prefer reserved `nameSelection` in samples

## Branching

No strict gitflow enforced in-repo. Release tags `v*` trigger `.github/workflows/release.yml`.
