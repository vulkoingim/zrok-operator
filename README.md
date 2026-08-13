# zrok-operator

Kubernetes operator that exposes in-cluster Services through [zrok/v2](https://github.com/openziti/zrok) public (and private) shares.

This is a **client-side** operator (enable → agent → share), complementary to the official server chart [`openziti/zrok2`](https://netfoundry.io/docs/zrok/self-hosting/deployment/kubernetes). It does **not** deploy the zrok controller or frontend.

## Architecture

```
ZrokEnvironment  →  PVC + zrok2 agent Deployment (data plane)
ZrokShare        →  agent gRPC SharePublic/SharePrivate (TCP :7777 → unix agent.socket)
ZrokAccess       →  agent gRPC AccessPrivate
```

Manager talks to the agent over **gRPC**, not HTTP `/v1/agent/*`. Agent registry is wiped on every start (operator owns share lifecycle). Sticky public URLs need `spec.nameSelection`.

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

Short names: `zrokkenv`, `zrokshare`, `zrokaccess`.

zrok Share objects have **no description/labels**. Identity in the UI is env `description`/`host` (`zrok-operator/<ns>/<env>`), the **name**, and the upstream target.

[Reserved vs ephemeral is a property of the **name**, not the share](https://netfoundry.io/docs/zrok/concepts/namespaces/). `spec.nameSelection` is `zrok2 create name -n <ns> <name>` then `zrok2 share public … -n <ns>:<name>` ([manage reserved names](https://netfoundry.io/docs/zrok/how-tos/shares/manage-reserved-names/)). The Share panel has no `reserved` field so it always renders ephemeral — look at **Names** (`zrok2 list names` / UI Names list, `reserved=true`). `kubectl get zrokshare` `RESERVATION` is our `status.reservation`.

### ZrokEnvironment

Owns agent PVC + Deployment. Requires `enableTokenSecretRef`.

```yaml
apiVersion: zrok.k8s.zrok.io/v1alpha1
kind: ZrokEnvironment
metadata:
  name: default
spec:
  # apiEndpoint: https://api-v2.zrok.io   # omit = public zrok.io; set for self-hosted
  enableTokenSecretRef:
    name: zrok-credentials          # required
    key: enable-token               # default key name if omitted
  reclaimPolicy: Delete             # Delete | Retain  (Disable remote env on CR delete?)
  agent:
    image: docker.io/openziti/zrok2:2.0.4   # omit = this default
    replicas: 1                     # only 1 allowed (Recreate)
    consolePort: 8888               # HTTP console in-pod; manager uses gRPC :7777
    persistence:
      size: 1Gi                     # PVC for ~/.zrok2
      # storageClassName: standard  # omit = cluster default
    # resources:                    # standard corev1 ResourceRequirements
    #   requests: { cpu: 50m, memory: 64Mi }
    #   limits:   { cpu: 500m, memory: 256Mi }
status:
  envZID: ""                        # ziti identity after Enable
  agentService: default-agent.default.svc
  agentReady: true
  conditions:                       # Ready | Enabled | AgentReady
    - type: Ready                   # True | False | Unknown
      reason: Ready                 # Ready | WaitingForAgent | SharesExist | …
```

### ZrokShare

```yaml
apiVersion: zrok.k8s.zrok.io/v1alpha1
kind: ZrokShare
metadata:
  name: nginx
spec:
  environmentRef: { name: default } # required; same namespace
  shareMode: public                 # public | private   (default public)
  backendMode: proxy                # proxy | web | caddy | drive | tcpTunnel | udpTunnel | socks
  upstream:
    url: http://nginx.default.svc:80   # required; scheme http|https|tcp|udp
  nameSelection:                    # public only; omit = ephemeral random URL (dies on agent restart)
    namespace: public               # zrok namespace token (default public)
    name: ko-default-nginx          # DNS label [a-z0-9][a-z0-9-]* ; unique on the zrok account
  # privateShareToken: nginx-priv   # private only; omit = random private token
  insecure: false                   # skip TLS verify to upstream
  closed: false                     # closed permission mode
  accessGrants: []                  # emails allowed when closed: true
  # basicAuthSecretRef: { name: my-basic-auth }  # Secret keys: username, password
  # oauth:
  #   provider: google              # google | github
  #   emailDomains: ["example.com"]
  #   refreshInterval: 24h          # Go duration
  reclaimPolicy: Delete             # Delete | Retain  (delete reserved name on CR delete?)
status:
  assignedURL: https://ko-default-nginx.share.zrok.io
  frontendEndpoints: []
  shareToken: ""
  reservation: reserved             # ephemeral | reserved | private
  conditions:                       # Ready | EnvironmentReady | ShareCreated | NameReady
    - type: Ready
      reason: Ready                 # Ready | NameConflict | WaitingForEnvironment | InvalidSpec | …
```

`nameSelection` + `shareMode=private` is rejected (CEL + reconciler). `privateShareToken` + `shareMode=public` likewise.

### ZrokAccess

Private-share consumer via the agent (bind a local port to someone else's private share token).

```yaml
apiVersion: zrok.k8s.zrok.io/v1alpha1
kind: ZrokAccess
metadata:
  name: to-nginx
spec:
  environmentRef: { name: default } # required; same namespace
  shareToken: "<private share token>"  # required
  bindAddress: "0.0.0.0:0"          # default; agent-local listen addr
status:
  frontendEndpoint: ""              # bound address when known
  accessToken: ""                   # agent frontend token
  conditions:
    - type: Ready                   # True | False | Unknown
      reason: Ready                 # Ready | WaitingForEnvironment | AccessError | …
```

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

- `zrok_share_ready{namespace,name}`
- `zrok_environment_ready{namespace,name}`
- `zrok_share_reconcile_errors`
- `zrok_environment_reconcile_errors`
- `zrok_access_reconcile_errors`

Enable Prometheus `ServiceMonitor` via Helm `metrics.serviceMonitor.enabled=true`.
