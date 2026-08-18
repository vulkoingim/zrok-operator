# Testing Practices

> **Last Updated:** 2026-08-18

## Layers

| Layer | Where | How to run |
|---|---|---|
| Unit / envtest | `internal/...` (exclude e2e) | `mise run test` |
| Agent helpers | `internal/agent/resources_test.go` | included in `mise run test` |
| E2E Kind | `test/e2e/` | Manager pod Running + HTTPS `/metrics` scrape + optional live share (`ZROK2_ENABLE_TOKEN`). `mise run test-e2e`. CI: merge queue / `main` / dispatch — **not** every PR. |

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

## E2E / Makefile

Ginkgo e2e calls `make install` / `make deploy` (parses the **whole** Makefile). GNU Make treats `kind:up` as a static pattern → `target pattern contains no '%'`. Make wrappers are `kind-up` / `kind-load` / `kind-deploy` → `.mise-tasks/kind/{up,load,deploy}`. Mise names stay `kind:up`.

No cert-manager / prometheus-operator in e2e. Metrics TLS is controller-runtime self-signed; the scrape test uses `curl -k`. Helm `metrics.serviceMonitor` is the scrape path if you want Prometheus Operator in-cluster.

## One-test tip

```bash
KUBEBUILDER_ASSETS="$(setup-envtest use 1.36 --bin-dir bin -p path)" \
  go test ./internal/controller/ -run Share -count=1
```
