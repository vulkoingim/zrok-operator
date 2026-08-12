# Component: zrokclient

- **Location**: `internal/zrokclient/` (`client.go`, `agent_grpc.go`, `mock/`)
- **Purpose**: Typed clients for zrok controller REST API and zrok2 agent gRPC
- **Owner**: Environment, Share, Access reconcilers
- **Last analysed**: 2026-08-12

## Data Flow Diagram

```mermaid
flowchart LR
  Ctrl[Controllers] --> Bundle[Clients]
  Bundle --> REST[HTTPRESTClient]
  Bundle --> GRPC[GRPCAgentClient]
  REST -->|HTTPS X-TOKEN| API["/api/v2"]
  GRPC -->|TCP| Proxy[socat :7777]
  Proxy --> Sock[agent.socket]
```

## Business Intent

Isolate transport details (media type, auth header, dial) behind small interfaces mockable with mockery.

## Interface (exported)

```go
type RESTClient interface {
  Enable(ctx, apiEndpoint, accountToken, host, description string) (envZID, zitiCfg string, err error)
  Disable(ctx, apiEndpoint, accountToken, envZID string) error
  CreateShareName(ctx, apiEndpoint, accountToken, namespaceToken, name string) error
  UpdateShareName(ctx, apiEndpoint, accountToken, namespaceToken, name string, reserved bool) error
  DeleteShareName(ctx, apiEndpoint, accountToken, namespaceToken, name string) error
  Unshare(ctx, apiEndpoint, accountToken, envZID, shareToken string) error
  FindShareByFrontendName(ctx, apiEndpoint, accountToken, envZID, name string) (shareToken string, endpoints []string, err error)
}

type AgentClient interface {
  Status(ctx, addr string) (*agentGrpc.StatusResponse, error)
  SharePublic(ctx, addr string, req *agentGrpc.SharePublicRequest) (*agentGrpc.SharePublicResponse, error)
  SharePrivate(ctx, addr string, req *agentGrpc.SharePrivateRequest) (*agentGrpc.SharePrivateResponse, error)
  ReleaseShare(ctx, addr, token string) error
  AccessPrivate(ctx, addr string, req *agentGrpc.AccessPrivateRequest) (*agentGrpc.AccessPrivateResponse, error)
  ReleaseAccess(ctx, addr, token string) error
}
```

Helpers: `NewDefaultClients`, `FrontendEndpointMatchesName`, `PersistEnabledEnvironment` (test helper).

## Protocol details

| Client | Base | Auth | Notes |
|---|---|---|---|
| REST | `{apiEndpoint}/api/v2` | `X-TOKEN` | Media `application/zrok.v1+json`; default endpoint `https://api-v2.zrok.io` |
| Agent | `host:port` | none (cluster network) | Native gRPC `agentGrpc`; dial `AgentDialAddr` |

### Idempotent HTTP semantics

- `CreateShareName`: 409 / already → **nil**
- `DeleteShareName` / `Unshare`: 404 → **nil**

## Error Handling

Callers interpret SharePublic errors (409) at controller layer. REST methods wrap HTTP status in returned errors for non-special cases.

## Testing

- `.mockery.yml` → `internal/zrokclient/mock/mocks.go`
- **Never hand-write mocks** — `mise run gen`
- Controllers inject interfaces

## Common Issues

| Symptom | Cause |
|---|---|
| Dial fail :7777 | Agent/socat not Ready; wrong Service DNS |
| 401/403 REST | Wrong enable/account token in Secret |
| FindShare miss | Endpoint string format ≠ name matcher |

## References

- [areas/share-lifecycle.md](../areas/share-lifecycle.md)
- [testing-practices.md](../testing-practices.md)
