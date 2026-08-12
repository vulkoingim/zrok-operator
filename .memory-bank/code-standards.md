# Code Standards

> **Last Updated:** 2026-08-12

Authoritative detail also in `.cursor/rules/go-coding-standards-and-guidelines.mdc`. Summary for agents:

## Musts

- `go fmt` / goimports; `go vet`; golangci-lint clean
- Explicit `if err != nil`; wrap with `fmt.Errorf("…: %w", err)`
- Context as first param; don’t store Context on structs
- Small interfaces at consumer side (`RESTClient`, `AgentClient`)
- Exported symbols have GoDoc
- Lines preferably < 120 chars
- Preserve intentional comments; don’t delete unless obsolete after a change

## Project-specific

- Kubebuilder RBAC markers stay in sync with `mise run manifests`
- Mocks only via mockery (`.mockery.yml`)
- Prefer `events.EventRecorder` (events.k8s.io-aware) over legacy recorder APIs
- Naming helpers live in `internal/agent` — reuse, don’t duplicate `ko-` prefix logic

## Tests

- Table-driven where natural; Ginkgo for controller suite
- Logical modules; fix suite before advancing
- No secrets in fixtures committed to git
