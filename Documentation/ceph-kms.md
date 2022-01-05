---
title: Key Management System
weight: 3650
indent: true
---

# Key Management System

Rook has the ability to encrypt OSDs of clusters running on PVC via the flag (`encrypted: true`) in your `storageClassDeviceSets` [template](#pvc-based-cluster).
By default, the Key Encryption Keys (also known as Data Encryption Keys) are stored in a Kubernetes Secret.
However, if a Key Management System exists Rook is capable of using it.

The `security` section contains settings related to encryption of the cluster.

* `security`:
  * `kms`: Key Management System settings
    * `connectionDetails`: the list of parameters representing kms connection details
    * `tokenSecretName`: the name of the Kubernetes Secret containing the kms authentication token

Supported KMS providers:

* [Vault](#vault)
## Vault

Rook supports storing OSD encryption keys in [HashiCorp Vault KMS](https://www.vaultproject.io/).

### Authentication methods

Rook support two authentication methods:

* [token-based](#token-based-authentication): a token is provided by the user and is stored in a Kubernetes Secret. It's used to
  authenticate the KMS by the Rook operator. This has several pitfalls such as:
    * when the token expires it must be renewed, so the secret holding it must be updated
    * no token automatic rotation
* [Kubernetes Service Account](#kubernetes-based-authentication) uses [Vault Kubernetes native
  authentication](https://www.vaultproject.io/docs/auth/kubernetes) mechanism and alleviate some of the limitations from the token authentication such as token automatic renewal. This method is
  generally recommended over the token-based authentication.

#### Token-based authentication

When using the token-based authentication, a Kubernetes Secret must be created to hold the token.
This is governed by the `tokenSecretName` parameter.

Note: Rook supports **all** the Vault [environment variables](https://www.vaultproject.io/docs/commands#environment-variables).

The Kubernetes Secret `rook-vault-token` should contain:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: rook-vault-token
  namespace: rook-ceph
data:
  token: <TOKEN> # base64 of a token to connect to Vault, for example: cy5GWXpsbzAyY2duVGVoRjhkWG5Bb3EyWjkK
```

You can create a token in Vault by running the following command:

```console
vault token create -policy=rook
```

Refer to the official vault document for more details on [how to create a
token](https://www.vaultproject.io/docs/commands/token/create). For which policy to apply see the
next section.

In order for Rook to connect to Vault, you must configure the following in your `CephCluster` template:

```yaml
security:
  kms:
    # name of the k8s config map containing all the kms connection details
    connectionDetails:
      KMS_PROVIDER: vault
      VAULT_ADDR: https://vault.default.svc.cluster.local:8200
      VAULT_BACKEND_PATH: rook
      VAULT_SECRET_ENGINE: kv
      VAULT_AUTH_METHOD: token
    # name of the k8s secret containing the kms authentication token
    tokenSecretName: rook-vault-token
```

#### Kubernetes-based authentication

In order to use the Kubernetes Service Account authentication method, the following must be run to properly configure Vault:

```sh
ROOK_NAMESPACE=rook-ceph
ROOK_VAULT_SA=rook-vault-auth
ROOK_SYSTEM_SA=rook-ceph-system
ROOK_OSD_SA=rook-ceph-osd
VAULT_POLICY_NAME=rook

# create service account for vault to validate API token
kubectl -n "$ROOK_NAMESPACE" create serviceaccount "$ROOK_VAULT_SA"

# create the RBAC for this SA
kubectl -n "$ROOK_NAMESPACE" create clusterrolebinding vault-tokenreview-binding --clusterrole=system:auth-delegator --serviceaccount="$ROOK_NAMESPACE":"$ROOK_VAULT_SA"

# get the service account common.yaml created earlier
VAULT_SA_SECRET_NAME=$(kubectl -n "$ROOK_NAMESPACE" get sa "$ROOK_VAULT_SA" -o jsonpath="{.secrets[*]['name']}")

# Set SA_JWT_TOKEN value to the service account JWT used to access the TokenReview API
SA_JWT_TOKEN=$(kubectl -n "$ROOK_NAMESPACE" get secret "$VAULT_SA_SECRET_NAME" -o jsonpath="{.data.token}" | base64 --decode)

# Set SA_CA_CRT to the PEM encoded CA cert used to talk to Kubernetes API
SA_CA_CRT=$(kubectl -n "$ROOK_NAMESPACE" get secret "$VAULT_SA_SECRET_NAME" -o jsonpath="{.data['ca\.crt']}" | base64 --decode)

# get kubernetes endpoint
K8S_HOST=$(kubectl config view --minify --flatten -o jsonpath="{.clusters[0].cluster.server}")

# enable kubernetes auth
vault auth enable kubernetes

# To fetch the service account issuer
kubectl proxy &
proxy_pid=$!

# configure the kubernetes auth
vault write auth/kubernetes/config \
    token_reviewer_jwt="$SA_JWT_TOKEN" \
    kubernetes_host="$K8S_HOST" \
    kubernetes_ca_cert="$SA_CA_CRT" \
    issuer="$(curl --silent http://127.0.0.1:8001/.well-known/openid-configuration | jq -r .issuer)"

kill $proxy_pid

# configure a role for rook
vault write auth/kubernetes/role/"$ROOK_NAMESPACE" \
    bound_service_account_names="$ROOK_SYSTEM_SA","$ROOK_OSD_SA" \
    bound_service_account_namespaces="$ROOK_NAMESPACE" \
    policies="$VAULT_POLICY_NAME" \
    ttl=1440h
```

Once done, your `CephCluster` CR should look like:

```yaml
security:
  kms:
    connectionDetails:
        KMS_PROVIDER: vault
        VAULT_ADDR: https://vault.default.svc.cluster.local:8200
        VAULT_BACKEND_PATH: rook
        VAULT_SECRET_ENGINE: kv
        VAULT_AUTH_METHOD: kubernetes
        VAULT_AUTH_KUBERNETES_ROLE: rook-ceph
```

Note that the `VAULT_ADDR` value above assumes that Vault is accessible within the cluster itself on the default port (8200). If running elsewhere, please update the URL accordingly.

### General Vault configuration

As part of the token, here is an example of a policy that can be used:

```hcl
path "rook/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}
path "sys/mounts" {
capabilities = ["read"]
}
```

You can write the policy like so and then create a token:

```console
vault policy write rook /tmp/rook.hcl
vault token create -policy=rook
```
>```
>Key                  Value
>---                  -----
>token                s.FYzlo02cgnTehF8dXnAoq2Z9
>token_accessor       oMo7sAXQKbYtxU4HtO8k3pko
>token_duration       768h
>token_renewable      true
>token_policies       ["default" "rook"]
>identity_policies    []
>policies             ["default" "rook"]
>```

In the above example, Vault's secret backend path name is `rook`. It must be enabled with the following:

```console
vault secrets enable -path=rook kv
```

If a different path is used, the `VAULT_BACKEND_PATH` key in `connectionDetails` must be changed.

### TLS configuration

This is an advanced but recommended configuration for production deployments, in this case the `vault-connection-details` will look like:

```yaml
security:
  kms:
    # name of the k8s config map containing all the kms connection details
    connectionDetails:
      KMS_PROVIDER: vault
      VAULT_ADDR: https://vault.default.svc.cluster.local:8200
      VAULT_CACERT: <name of the k8s secret containing the PEM-encoded CA certificate>
      VAULT_CLIENT_CERT: <name of the k8s secret containing the PEM-encoded client certificate>
      VAULT_CLIENT_KEY: <name of the k8s secret containing the PEM-encoded private key>
    # name of the k8s secret containing the kms authentication token
    tokenSecretName: rook-vault-token
```

Each secret keys are expected to be:

* VAULT_CACERT: `cert`
* VAULT_CLIENT_CERT: `cert`
* VAULT_CLIENT_KEY: `key`

For instance `VAULT_CACERT` Secret named `vault-tls-ca-certificate` will look like:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: vault-tls-ca-certificate
  namespace: rook-ceph
data:
  cert: <PEM base64 encoded CA certificate>
```

Note: if you are using self-signed certificates (not known/approved by a proper CA) you must pass `VAULT_SKIP_VERIFY: true`.
Communications will remain encrypted but the validity of the certificate will not be verified.
