# Testing Practices

> **Last Updated:** 2026-08-18

## Layers

| Layer | Where | How to run |
|---|---|---|
| Unit / envtest | `internal/...` (exclude e2e) | `mise run test` |
| Agent helpers | `internal/agent/resources_test.go` | included in `mise run test` |
| E2E Kind | `test/e2e/` | Local: `mise run test-e2e`. CI: merge queue / `main` / Actions dispatch — **not** every PR. |

## Mocks (mandatory)

1. Define interface (prefer existing `RESTClient` / `AgentClient`)
2. Add to `.mockery.yml`
3. `mise run gen` → `internal/zrokclient/mock/mocks.go`
4. Use testify mock in tests

**Do not hand-write mocks.** See `.cursor/rules/testing.mdc`.

## Patterns

- Controller suite: Ginkgo + envtest (`internal/controller/suite_test.go`)
- Share tests: inject mockery clients (`share_controller_test.go`)
- Prefer table-driven / logical modules; fix failing suite before next module

## Agents must NOT

- Commit enable tokens or paste them into memory bank
- Point live e2e at production credentials casually
- Skip regenerating mocks after interface changes

## One-test tip

```bash
KUBEBUILDER_ASSETS="$(setup-envtest use 1.36 --bin-dir bin -p path)" \
  go test ./internal/controller/ -run Share -count=1
```
