---
# AWS server side encryption SSE-S3 support for RGW
target-version: release-1.10
---

# Feature Name
AWS server side encryption SSE-S3 support for RGW

## Summary
The S3 protocol supports three different types of [server side encryption](https://docs.aws.amazon.com/AmazonS3/latest/userguide/serv-side-encryption.html): SSE-C, SSE-KMS and SSE-S3. For the last two RGW server need to configure with external services such as [vault](https://www.vaultproject.io/). Currently Rook configure RGW with `SSE-KMS` options to handle the S3 requests with the `sse:kms` header. Recently the support for handling the `sse:s3` was added to RGW, so Rook will now provide the option to configure RGW with `sse:s3`.

### Goals
Configure RGW with `SSE-S3` options, so that RGW can handle request with `sse:s3` headers.

## Proposal details
Introducing new field `s3` in `SecuritySpec` which defines `AWS-SSE-S3` support for RGW.
```yaml
security:
  kms:
    ..
  s3:
    connectionDetails:
      KMS_PROVIDER: vault
      VAULT_ADDR: https://vault.default.svc.cluster.local:8200
      VAULT_SECRET_ENGINE: transit
      VAULT_AUTH_METHOD: token
    # name of the k8s secret containing the kms authentication token
    tokenSecretName: rook-vault-token
```
The values for this configuration will be available in [Security field](Documentation/CRDs/Object-Storage/ceph-object-store-crd.md#security-settings) of `CephObjectStoreSpec`, so depending on the above option RGW can be configured with `SSE-KMS` and `SSE-S3` options. These two options can be configured independently and they both are mutually exclusive in Rook and Ceph level.

## Config options
Before this design is implemented, the user could manually set below options from the toolbox pod to configure with `SSE-S3` options for RGW.
```
ceph config set <ceph auth user for rgw> rgw_crypt_sse_s3_backend vault
ceph config set <ceph auth user for rgw> rgw_crypt_sse_s3_vault_secret_engine <kv|transit> # https://docs.ceph.com/en/latest/radosgw/vault/#vault-secrets-engines
ceph config set <ceph auth user for rgw> rgw_crypt_sse_s3_vault_addr <vault address>
ceph config set <ceph auth user for rgw> rgw_crypt_sse_s3_vault_auth token
ceph config set <ceph auth user for rgw> rgw_crypt_sse_s3_vault_token_file <location of file containing token for accessing vault>
```

The `ceph auth user for rgw` is the (ceph client user)[https://docs.ceph.com/en/latest/rados/operations/user-management/#user-management] who has permissions change the settings for RGW server. This can be listed from `ceph auth ls` command and in Rook ceph client user for RGW will always begins with `client.rgw`, followed by `store` name.

---

# Vault Agent Authentication for SSE-S3

The `agent` auth method uses a [Vault Agent](https://developer.hashicorp.com/vault/docs/agent-and-proxy/agent) running as a sidecar container in the RGW pod. The agent authenticates to Vault using the pod's service account (via Vault's [Kubernetes auth method](https://developer.hashicorp.com/vault/docs/auth/kubernetes)), then acts as a local proxy on `localhost`. RGW sends requests to the agent instead of directly to Vault, and the agent transparently injects the authentication token. This eliminates using and managing a static token.

The [Vault Agent Injector](https://developer.hashicorp.com/vault/docs/platform/k8s/injector) (a mutating admission webhook) is used to automatically inject the Vault Agent sidecar into RGW pods. Users get full control over Vault Agent configuration without Rook needing to manage sidecar details.

## Vault Prerequisites

Before using Vault Agent auth, the user must configure Vault:

```bash
# Enable Kubernetes auth method
vault auth enable kubernetes

# Configure Kubernetes auth
vault write auth/kubernetes/config \
    kubernetes_host="https://$KUBERNETES_HOST:443"

# Create a policy granting access to the transit engine.
# RGW requires the following transit paths:
#   transit/keys/*                  - create and manage encryption keys
#   transit/datakey/plaintext/*     - generate data encryption keys (used during object upload)
#   transit/decrypt/*               - decrypt data encryption keys (used during object download/head)
#   transit/export/encryption-key/* - export encryption keys
#   transit/keys/+/rotate           - rotate encryption keys
vault policy write rgw-sse-s3 - <<EOF
path "transit/keys/*" {
  capabilities = ["create", "read", "update", "delete"]
}
path "transit/keys/+/rotate" {
  capabilities = ["update"]
}
path "transit/datakey/plaintext/*" {
  capabilities = ["update"]
}
path "transit/decrypt/*" {
  capabilities = ["update"]
}
path "transit/export/encryption-key/*" {
  capabilities = ["read"]
}
EOF

# Create a role bound to the RGW service account
vault write auth/kubernetes/role/rook-ceph-rgw \
    bound_service_account_names=rook-ceph-rgw \
    bound_service_account_namespaces=rook-ceph \
    policies=rgw-sse-s3 \
    ttl=1h
```

## Prerequisites

1. **Deploy the Vault Agent Injector** in the cluster. See the [Vault Helm chart](https://developer.hashicorp.com/vault/docs/platform/k8s/helm) for installation instructions:

   ```bash
   helm repo add hashicorp https://helm.releases.hashicorp.com
   helm install vault hashicorp/vault \
       --set "injector.enabled=true" \
       --set "server.enabled=false"   # if using an external Vault server
   ```

2. **Configure Vault** with the Kubernetes auth method, policy, and role as described in [Vault Prerequisites](#vault-prerequisites) above.

## How It Works

1. The user annotates the RGW pods (via the CephObjectStore CR's `annotations` field on the `gateway` spec) with Vault Agent Injector annotations.
2. The Vault Agent Injector webhook intercepts the RGW pod creation and injects a Vault Agent sidecar container.
3. The injected Vault Agent authenticates to Vault using the pod's service account and acts as a local proxy.
4. RGW connects to the Vault Agent on `localhost` instead of directly to the Vault server.

## API Design

No new API types are needed. The user configures the Vault Agent Injector via pod annotations on the CephObjectStore CR and sets `VAULT_AUTH_METHOD: agent` in `connectionDetails` to tell the operator to configure RGW for agent auth.

## CephObjectStore CR Example

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStore
metadata:
  name: my-store
  namespace: rook-ceph
spec:
  gateway:
    annotations:
      vault.hashicorp.com/agent-inject: "true"
      vault.hashicorp.com/role: "rook-ceph-rgw"
      vault.hashicorp.com/agent-cache-enable: "true"
      vault.hashicorp.com/agent-cache-listener-port: "8100"
      # For TLS to Vault server:
      # vault.hashicorp.com/ca-cert: "/vault/tls/ca.crt"
      # vault.hashicorp.com/client-cert: "/vault/tls/tls.crt"
      # vault.hashicorp.com/client-key: "/vault/tls/tls.key"
  security:
    s3:
      connectionDetails:
        KMS_PROVIDER: vault
        VAULT_ADDR: http://127.0.0.1:8100
        VAULT_SECRET_ENGINE: transit
        VAULT_AUTH_METHOD: agent
      # No tokenSecretName since the injected agent handles authentication
```

## Ceph Config Options

When `VAULT_AUTH_METHOD` is set to `agent`, the operator sets these RGW daemon flags:

```
rgw crypt sse s3 backend = vault
rgw crypt sse s3 vault auth = agent
rgw crypt sse s3 vault addr = http://127.0.0.1:8100
rgw crypt sse s3 vault secret engine = transit
rgw crypt sse s3 vault prefix = /v1/transit
```

Note that `VAULT_ADDR` points to the local Vault Agent proxy (injected by the webhook), not the actual Vault server. The Vault Agent handles forwarding requests to the real Vault server.

## Operator Changes

When `VAULT_AUTH_METHOD: agent` is set in `connectionDetails`:

1. The operator configures RGW with `vault auth = agent` and sets `vault addr` to the value of `VAULT_ADDR` from `connectionDetails`. For a sidecar agent, this is typically `http://127.0.0.1:<port>`, but users can point it to any Vault Agent they manage (e.g., a DaemonSet-based agent on a node-local address).
2. The Vault Agent Injector webhook handles the injection of vault agent sidecar container.
3. The operator skips the `vault-initcontainer-token-file-setup` init container for SSE-S3 since no static token is needed. This does not affect KMS (SSE-KMS) token-based auth, which continues to use the init container as before.
4. Token file flags (`rgw crypt sse s3 vault auth` and `rgw crypt sse s3 vault token file`) are not needed since there is no static token and the Vault Agent handles authentication. KMS token file flags are not affected.

## Validation Rules

1. When `VAULT_AUTH_METHOD` is `agent`, `tokenSecretName` must not be set for SSE-S3.
2. `KMS_PROVIDER` must be `vault`.
3. `VAULT_SECRET_ENGINE: transit` is required for SSE-S3.

## Risks

If the Vault Agent Injector webhook is **down** when a new RGW pod is being created (e.g., after a node failure or rollout restart), the sidecar will not be injected. The pod would start without the Vault Agent, and RGW would fail to connect to Vault on localhost.

Mitigations:

- Run the Vault Agent Injector with **multiple replicas** for high availability.
- The webhook's default `failurePolicy: Fail` prevents the pod from being created if the webhook is unreachable. This means RGW pods will fail to schedule rather than run without the sidecar, and the Rook operator will keep retrying reconciliation until the injector is available again.
