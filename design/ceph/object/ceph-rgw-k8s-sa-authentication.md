---
Service authentication with vault for RGW
target-version: release-1.15
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

* `VAULT_AGENT_ADDR`     --> provides address of vault agent.

The user can set vault agent independently (preferred way) and set the address here. Also user can bring up vault agent as sidecar by setting following annotation on the RGW pod, set the value as `localhost` for `VAULT_AGENT_ADDR`

If vault agent address points to localhost following annotation will set on rgw pod:

* vault.hashicorp.com/agent-inject    --> brings vault agent with rgw pod
* vault.hashicorp.com/agent-configmap -->  config map containing settings for vault agent pod

If tls is configured then following annotations will be set

* vault.hashicorp.com/tls-secret  --> the name of kubernetes secret containing TLS certs to communicate with vault server, secret will be mounted in `/vault/tls`
* vault.hashicorp.com/ca-cert     --> path for ca cert `/vault/tls/ca.crt`
* vault.hashicorp.com/client-cert --> path for client cert from vault server  `/vault/tls/client.crt`
* vault.hashicorp.com/client-key  --> path for key from the vault server `/vault/tls/client.key`

### Vault Agent Configuration

Below is example for vault agent configuration which can be used as sidecar and independently.

```
kind: ConfigMap
metadata:
  name: <ceph-object-store>-vault-agent-cm
  namespace: rook-ceph # namespace of rgw pod
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
```

For independent configuration user can below sample pod configuration, details copied from [here](https://developer.hashicorp.com/vault/tutorials/vault-agent/agent-kubernetes):

```
apiVersion: v1
kind: Pod
metadata:
  name: vault-agent-example
  namespace: rook-ceph # namespace of rgw pod
spec:
  serviceAccountName: vault-agent-rook # service account added with rook-ceph role

  volumes:
    - configMap:
        items:
          - key: vault-agent-config.hcl
            path: vault-agent-config.hcl
        name: <ceph-object-store>-vault-agent-cm
      name: config
    - emptyDir: {}
      name: shared-data

  initContainers:
    - args:
        - agent
        - -config=/etc/vault/vault-agent-config.hcl
        - -log-level=debug
      env:
        - name: VAULT_ADDR
          value: http://EXTERNAL_VAULT_ADDR:8200
      image: vault
      name: vault-agent

  containers:
    - image: nginx
      name: nginx-container
      ports:
        - containerPort: 80
      volumeMounts:
        - mountPath: /usr/share/nginx/html
          name: shared-data
```

After configuring vault agent, user can either create k8s service, port-forward method etc to access vault agent address.

Other settings are similar to the configuration mentioned in [here](/Documentation/Storage-Configuration/Advanced/key-management-system/#kubernetes-based-authentication)

The `Ceph Object Store` CRD will look as :

```yaml
spec:
  security:
    kms:
      connectionDetails:
          KMS_PROVIDER: vault
          VAULT_AGENT_ADDR: http://127.0.0.1:8100
          VAULT_BACKEND_PATH: rook
          VAULT_SECRET_ENGINE: kv
          VAULT_AUTH_METHOD: kubernetes
```

### TLS authentication between vault agent and vault server

For TLS enabled vault server, the certs can be provided with help of k8s secret and added to `vault.hashicorp.com/tls-secret: <name of k8s secret>` and configure the `vault` block accordingly. The secret will be mounted in the path `/vault/tls/`. Again same settings can be used if vault agent is running as pod or sidecar.

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

!!! Note:
Same configuration can be applied on `sse:s3` configuration

```yaml
security:
  s3:
    connectionDetails:
        KMS_PROVIDER: vault
        VAULT_AGENT_ADDR: <address of vault agent>
        VAULT_BACKEND_PATH: rook
        VAULT_SECRET_ENGINE: kv
        VAULT_AUTH_METHOD: kubernetes
```

### TLS configuration between available vault agent and RGW server

Similar to vault server TLS configuration can be for agent as mentioned in the [docs](https://rook.io/docs/rook/v1.9/Storage-Configuration/Advanced/key-management-system/#tls-configuration).


### Config Commands

The user can itself bring up `vault agent` as separate deployment configuring with service account authentication details. Then set following for RGW via toolbox pod:

```bash
ceph config set <ceph auth user for rgw> rgw_crypt_sse_s3_vault_addr <vault agent address>
ceph config set <ceph auth user for rgw> rgw_crypt_sse_s3_vault_auth agent
```
