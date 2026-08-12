# zrok-operator

Kubernetes operator that exposes in-cluster Services through [zrok/v2](https://github.com/openziti/zrok) public (and private) shares.

This is a **client-side** operator (enable → agent → share), complementary to the official server chart [`openziti/zrok2`](https://netfoundry.io/docs/zrok/self-hosting/deployment/kubernetes). It does **not** deploy the zrok controller or frontend.

## Architecture

```
ZrokEnvironment  →  PVC + zrok2 agent Deployment (data plane)
ZrokShare        →  agent SharePublic via HTTP console gateway (:8888)
ZrokAccess       →  agent AccessPrivate (private share consumer)
```

## Quickstart (Kind)

```bash
mise install
mise run kind-deploy

# Credentials (zrok.io or self-hosted enable token)
kubectl create secret generic zrok-credentials \
  --from-literal=enable-token="$ZROK2_ENABLE_TOKEN"

# Sample Environment + Share (edit nameSelection.name to be unique)
kubectl apply -f config/samples/zrok_v1alpha1_environment_share.yaml

kubectl wait --for=condition=Ready zrokenvironment/default --timeout=180s
kubectl wait --for=condition=Ready zrokshare/nginx --timeout=180s
kubectl get zrokshare nginx -o jsonpath='{.status.assignedURL}{"\n"}'
```

## CRDs

### ZrokEnvironment

Owns agent PVC + Deployment. Requires `enableTokenSecretRef`.

### ZrokShare

```yaml
spec:
  environmentRef: { name: default }
  shareMode: public # public|private
  backendMode: proxy # proxy|web|caddy|drive|tcpTunnel|udpTunnel|socks
  upstream: { url: http://mysvc.ns.svc:80 }
  nameSelection: # strongly recommended (agent auto-restart)
    namespace: public
    name: myapp
  basicAuthSecretRef: { name: my-basic-auth } # keys: username, password
  oauth:
    provider: google
    emailDomains: ["example.com"]
```

Reserved names use the v2 namespaces/names model (not v1 `reserve`). See [migrate v1→v2](https://netfoundry.io/docs/zrok/how-tos/migration/migrate-v1-to-v2/).

### ZrokAccess

Private-share consumer via the agent.

## Helm

```bash
helm upgrade --install zrok-operator ./charts/zrok-operator \
  -n zrok-operator --create-namespace \
  --set image.repository=zrok-operator \
  --set image.tag=dev
```

Prefer a pre-created Secret over `--set credentials.enableToken=...`.

## Development

```bash
mise install                 # tools: go, controller-gen, kind, helm, …
mise tasks                   # list
mise run gen                 # deepcopy + CRDs + helm crds/
mise run test                # unit + envtest via gotestsum
mise run build               # bin/manager
mise run kind-up && mise run deploy
mise run test-e2e            # Kind e2e; set ZROK2_ENABLE_TOKEN for live share
mise run samples --secret    # apply sample CRs (creates secret from env)
```

Longer flows live under `.mise-tasks/`; short ones in `mise.toml`.
`Makefile` runs the same recipes / file-tasks directly (does not invoke `mise run`).

## Ingress translation

Create an Ingress with `ingressClassName: zrok` (see `config/samples/ingress_zrok.yaml`). Annotations:

| Annotation                         | Purpose                                  |
| ---------------------------------- | ---------------------------------------- |
| `zrok.k8s.zrok.io/environment`     | ZrokEnvironment name (default `default`) |
| `zrok.k8s.zrok.io/name`            | Reserved frontend name                   |
| `zrok.k8s.zrok.io/namespace-token` | Namespace token (default `public`)       |

The controller creates an owned `ZrokShare` and mirrors `status.assignedURL` onto the Ingress load balancer hostname.

## Metrics

- `zrok_share_ready`
- `zrok_share_reconcile_errors`
- `zrok_environment_ready`

Enable Prometheus `ServiceMonitor` via Helm `metrics.serviceMonitor.enabled=true`.
