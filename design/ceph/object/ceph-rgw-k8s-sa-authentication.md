---
Service authentication with vault for RGW
target-version: release-1.14
---

# Feature Name

Service account authentication with vault for RGW.

## Summary

When `Vault` is set up in Kubernetes environment, it can be authenticated using service account token. The OSD encryption already supports this feature and adding same for RGW. It uses same set of  config option in `ConnectionDetails` for `KeyManagementServiceSpec`.

### Goals

Configure RGW with service account authentication with Vault with help of vault agent.

### Non-Goals

There are different ways RGW can authenticate with help of [vault agent](https://docs.ceph.com/en/latest/radosgw/vault/#vault-agent), but only service account authentication is supported here.

## Proposal details

The [vault agent](https://developer.hashicorp.com/vault/docs/agent-and-proxy/agent#vault-stanza) can be configure with help of following settings in `connectionDetails` in `CephObjectStore CRD` and it will set following annotations in rgw pods.

* `VAULT_AGENT_ADDR`     --> provides address of vault agent
* `VAULT_AGENT_CM`       --> if address is localhost then agent need to configured along with rgw pod passed with help of cm, role for k8s service account need to add here
* `VAULT_AGENT_TLS_CERT` --> tls cert for vault agent to communicate with vault server

If vault agent address points to localhost following annotation will set on rgw pod:

* vault.hashicorp.com/agent-inject    --> brings vault agent with rgw pod
* vault.hashicorp.com/agent-configmap -->  config map containing settings for vault agent pod

If tls is configured then following annotations will be set

* vault.hashicorp.com/tls-secret  --> the name of kubernetes secret containing TLS certs to communicate with vault server, secret will be mounted in `/vault/tls`
* vault.hashicorp.com/ca-cert     --> path for ca cert `/vault/tls/ca.crt`
* vault.hashicorp.com/client-cert --> path for client cert from vault server  `/vault/tls/client.crt`
* vault.hashicorp.com/client-key  --> path for key from the vault server `/vault/tls/client.key`

Other settings are similar to the configuration mentioned in [here](/Documentation/Storage-Configuration/Advanced/key-management-system/#kubernetes-based-authentication)

The `Ceph Object Store` CRD will look as :

```yaml
spec:
  security:
    kms:
      connectionDetails:
          KMS_PROVIDER: vault
          VAULT_AGENT_ADDR: http://127.0.0.1:8100
          VAULT_AGENT_CM: <ceph-object-store>-vault-agent-cm
          VAULT_BACKEND_PATH: rook
          VAULT_SECRET_ENGINE: kv
          VAULT_AUTH_METHOD: kubernetes

---
kind: ConfigMap
metadata:
  name: <ceph-object-store>-vault-agent-cm
  namespace: rook-ceph
apiVersion: v1
data:

  config.hcl: |
    pid_file = "/home/vault/pidfile"

    auto_auth {
        method "kubernetes" {
            mount_path = "auth/kubernetes"
            config = {
                role = "rook-ceph"  # role assigned to service account
            }
        }
    }

    cache {
        use_auto_auth_token = true
    }

    exit_after_auth = false

    listener "tcp" {
        address = "127.0.0.1:8100" # port where vault agent listens in the pod
        tls_disable = "true"
    }

    vault {
      address = "http://vault.default:8200" # address of vault server
      tls_skip_verify = "true"
    }

  config-init.hcl: |
    pid_file = "/home/vault/pidfile"

    auto_auth {
        method "kubernetes" {
            mount_path = "auth/kubernetes"
            config = {
                role = "rook-ceph" # role assigned to service account
            }
        }
    }

    cache {
        use_auto_auth_token = true
    }

    listener "tcp" {
        address = "127.0.0.1:8100" # port where vault agent listens in the pod
        tls_disable = "true"
    }

    exit_after_auth = true

    vault {
      address = "http://vault.default:8200" # address of vault server
      tls_skip_verify = "true"
    }

---
pod:
  annotations:
    vault.hashicorp.com/agent-inject: 'true'
    vault.hashicorp.com/agent-configmap: <ceph-object-store>-vault-agent-cm

```

### TLS authentication

For TLS enabled vault server, the certs can be provided with help of k8s secret and added to `vault.hashicorp.com/tls-secret: <name of k8s secret>` and configure the `vault` block accordingly. The secret will be mounted in the path `/vault/tls/`.

```yaml
pod:
  annotations:
    vault.hashicorp.com/agent-inject: 'true'
    vault.hashicorp.com/agent-configmap: <ceph-object-store>-vault-agent-cm
    vault.hashicorp.com/tls-secret: <name of k8s secret>
    vault.hashicorp.com/ca-cert: '/vault/tls/ca.crt'
    vault.hashicorp.com/client-cert: '/vault/tls/client.crt'
    vault.hashicorp.com/client-key: '/vault/tls/client.key'

---
kind: Secret
metadata:
  name: <name of k8s secret>
  namespace: rook-ceph
type: Opaque
data:
  client.key: <>
  client.crt: <>
  ca.crt:     <>

```

Another option is user can set vault agent independently then set `VAULT_AGENT_ADDR` to agent without mentioning config map.

!!! Note:
Same configuration can be applied on `sse:s3` configuration

```yaml
security:
  s3:
    connectionDetails:
        KMS_PROVIDER: vault
        VAULT_AGENT_ADDR: <address of vault agent>
        VAULT_AGENT_CM: <cm for vault agent>
        VAULT_AGENT_TLS: <k8s secret containing tls certs>
        VAULT_BACKEND_PATH: rook
        VAULT_SECRET_ENGINE: kv
        VAULT_AUTH_METHOD: kubernetes
```

## Config Commands

The user can itself bring up `vault agent` as separate deployment configuring with service account authentication details. Then set following for RGW via toolbox pod:

```bash
ceph config set <ceph auth user for rgw> rgw_crypt_sse_s3_vault_addr <vault agent address>
ceph config set <ceph auth user for rgw> rgw_crypt_sse_s3_vault_auth agent
```
