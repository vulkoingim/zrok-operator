# Testing Practices

> **Last Updated:** 2026-08-18

## Layers

| Layer | Where | How to run |
|---|---|---|
| Unit / envtest | `internal/...` (exclude e2e) | `mise run test` |
| Agent helpers | `internal/agent/*_test.go` | included in `mise run test` |
| E2E Kind | `test/e2e/` | Deploy manager via `make install`/`deploy`; optional live share when `ZROK2_ENABLE_TOKEN` is set (otherwise Skip). `cleanupLiveZrokTestResources` runs on `DeferCleanup` and in `AfterAll` **before** `make undeploy` (shares → env → nginx/agent orphans). `mise run test:e2e`. CI: merge queue / `main` / dispatch — **not** every PR. |

## Mocks (mandatory)

1. Define interface (prefer existing `RESTClient` / `AgentClient`)
2. Add to `.mockery.yml`
3. `mise run gen` → `internal/zrokclient/mock/mocks.go`
4. Use testify mock in tests

**Do not hand-write mocks.** See `.cursor/rules/testing.mdc`.

## Patterns

- Controller suite: Ginkgo + envtest (`internal/controller/suite_test.go`)
- Share tests: inject mockery clients (`share_controller_test.go`); name drift in `share_apply_test.go`
- Prefer table-driven / logical modules; fix failing suite before next module

## Agents must NOT

- Commit enable tokens or paste them into memory bank
- Point live e2e at production credentials casually
- Skip regenerating mocks after interface changes

## E2E / Makefile

Ginkgo e2e calls `make install` / `make deploy` (parses the **whole** Makefile). GNU Make treats `kind:up` as a static pattern → `target pattern contains no '%'`. Make wrappers are `kind-up` / `kind-load` / `kind-deploy` → `.mise-tasks/kind/{up,load,deploy}`. Mise names stay `kind:up`.

No cert-manager / prometheus-operator in e2e. Helm `metrics.serviceMonitor` is the in-cluster Prometheus scrape path when enabled. Live-share e2e creates the enable-token Secret via `--from-file=enable-token=/dev/stdin` (argv is redacted).

## One-test tip

```bash
KUBEBUILDER_ASSETS="$(setup-envtest use 1.36 --bin-dir bin -p path)" \
  go test ./internal/controller/ -run Share -count=1
```
