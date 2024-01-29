---
title: Key Management System
---

Rook has the ability to encrypt OSDs of clusters running on PVC via the flag (`encrypted: true`) in your `storageClassDeviceSets` [template](#pvc-based-cluster).
Rook also has the ability to rotate encryption keys of OSDs using a cron job per OSD.
By default, the Key Encryption Keys (also known as Data Encryption Keys) are stored in a Kubernetes Secret.
However, if a Key Management System exists Rook is capable of using it.

The `security` section contains settings related to encryption of the cluster.

* `security`:
  * `kms`: Key Management System settings
    * `connectionDetails`: the list of parameters representing kms connection details
    * `tokenSecretName`: the name of the Kubernetes Secret containing the kms authentication token
  * `keyRotation`: Key Rotation settings
    * `enabled`: whether key rotation is enabled or not, default is `false`
    * `schedule`: the schedule, written in [cron format](https://en.wikipedia.org/wiki/Cron), with which key rotation [CronJob](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/) is created, default value is `"@weekly"`.

!!! note
    Currently key rotation is only supported for the default type, where the Key Encryption Keys are stored in a Kubernetes Secret.

Supported KMS providers:

* [Vault](#vault)
  * [Authentication methods](#authentication-methods)
    * [Token-based authentication](#token-based-authentication)
    * [Kubernetes-based authentication](#kubernetes-based-authentication)
  * [General Vault configuration](#general-vault-configuration)
  * [TLS configuration](#tls-configuration)
* [IBM Key Protect](#ibm-key-protect)
  * [Configuration](#configuration)
* [Key Management Interoperability Protocol](#key-management-interoperability-protocol)
  * [Configuration](#configuration-1)
* [Azure Key Vault](#azure-key-vault)
  * [Client Authentication](#client-authentication)

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

```console
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

!!! note
    The `VAULT_ADDR` value above assumes that Vault is accessible within the cluster itself on the default port (8200). If running elsewhere, please update the URL accordingly.

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
$ vault policy write rook /tmp/rook.hcl
$ vault token create -policy=rook
Key                  Value
---                  -----
token                s.FYzlo02cgnTehF8dXnAoq2Z9
token_accessor       oMo7sAXQKbYtxU4HtO8k3pko
token_duration       768h
token_renewable      true
token_policies       ["default" "rook"]
identity_policies    []
policies             ["default" "rook"]
```

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

## IBM Key Protect

Rook supports storing OSD encryption keys in [IBM Key
Protect](https://www.ibm.com/cloud/key-protect). The current implementation stores OSD encryption
keys as [Standard Keys](https://cloud.ibm.com/docs/key-protect?topic=key-protect-envelope-encryption#key-types) using the [Bring Your Own
Key](https://cloud.ibm.com/docs/key-protect?topic=key-protect-importing-keys) (BYOK) method. This
means that the Key Protect instance policy must have Standard Imported Key enabled.

### Configuration

First, you need to [provision the Key Protect service](https://cloud.ibm.com/docs/key-protect?topic=key-protect-provision) on the IBM Cloud. Once
completed, [retrieve the instance ID](https://cloud.ibm.com/docs/key-protect?topic=key-protect-retrieve-instance-ID&interface=ui).
Make a record of it; we need it in the CRD.

On the IBM Cloud, the user must create a Service ID, then assign an Access Policy to this service.
Ultimately, a Service API Key needs to be generated. All the steps are summarized in the [official
documentation](https://www.ibm.com/docs/en/cloud-private/3.2.0?topic=dg-creating-service-id-by-using-cloud-private-management-console).

The Service ID must be granted access to the Key Protect Service. Once the Service API Key is
generated, store it in a Kubernetes Secret.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: ibm-kp-svc-api-key
  namespace: rook-ceph
data:
  IBM_KP_SERVICE_API_KEY: <service API Key>
```

In order for Rook to connect to IBM Key Protect, you must configure the following in your `CephCluster` template:

```yaml
security:
  kms:
    # name of the k8s config map containing all the kms connection details
    connectionDetails:
      KMS_PROVIDER: ibmkeyprotect
      IBM_KP_SERVICE_INSTANCE_ID: <instance ID that was retrieved in the first paragraph>
    # name of the k8s secret containing the service API Key
    tokenSecretName: ibm-kp-svc-api-key
```

More options are supported such as:

* `IBM_BASE_URL`: the base URL of the Key Protect instance, depending on your
  [region](https://cloud.ibm.com/docs/key-protect?topic=key-protect-regions). Defaults to `https://us-south.kms.cloud.ibm.com`.
* `IBM_TOKEN_URL`: the URL of the Key Protect instance to retrieve the token. Defaults to
  `https://iam.cloud.ibm.com/oidc/token`. Only needed for private instances.

## Key Management Interoperability Protocol

Rook supports storing OSD encryption keys in [Key Management Interoperability Protocol (KMIP)](https://www.ibm.com/cloud/key-protect) supported
KMS.
The current implementation stores OSD encryption
keys using the [Register](https://docs.oasis-open.org/kmip/kmip-spec/v2.0/os/kmip-spec-v2.0-os.html#_Toc6497565) operation.
Key is fetched and deleted using [Get](https://docs.oasis-open.org/kmip/kmip-spec/v2.0/os/kmip-spec-v2.0-os.html#_Toc6497545)
and [Destroy](https://docs.oasis-open.org/kmip/kmip-spec/v2.0/os/kmip-spec-v2.0-os.html#_Toc6497541) operations respectively.

### Configuration

The Secret with credentials for the KMIP KMS is expected to contain the following.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: kmip-credentials
  namespace: rook-ceph
stringData:
  CA_CERT: <ca certificate>
  CLIENT_CERT: <client certificate>
  CLIENT_KEY: <client key>
```

In order for Rook to connect to KMIP, you must configure the following in your `CephCluster` template:

```yaml
security:
  kms:
    # name of the k8s config map containing all the kms connection details
    connectionDetails:
      KMS_PROVIDER: kmip
      KMIP_ENDPOINT: <KMIP endpoint address>
      # (optional) The endpoint server name. Useful when the KMIP endpoint does not have a DNS entry.
      TLS_SERVER_NAME: <tls server name>
      # (optional) Network read timeout, in seconds. The default value is 10.
      READ_TIMEOUT: <read timeout>
      # (optional) Network write timeout, in seconds. The default value is 10.
      WRITE_TIMEOUT: <write timeout>
    # name of the k8s secret containing the credentials.
    tokenSecretName: kmip-credentials
```

## Azure Key Vault

Rook supports storing OSD encryption keys in [Azure Key vault](https://learn.microsoft.com/en-us/azure/key-vault/general/quick-create-portal)

### Client Authentication

Different methods are available in Azure to authenticate a client. Rook supports Azure recommended method of authentication with Service Principal and a certificate. Refer the following Azure documentation to set up key vault and authenticate it via service principal and certtificate

* [Create Azure Key Vault](https://learn.microsoft.com/en-us/azure/key-vault/general/quick-create-portal)
  * `AZURE_VAULT_URL` can be retrieved at this step

* [Create Service Principal](https://learn.microsoft.com/en-us/entra/identity-platform/howto-create-service-principal)
  * `AZURE_CLIENT_ID` and `AZURE_TENANT_ID` can be obtained after creating the service principal
  * Ensure that the service principal is authenticated with a certificate and not with a client secret.

* [Set Azure Key Vault RBAC](https://learn.microsoft.com/en-us/azure/key-vault/general/rbac-guide?tabs=azure-cli#enable-azure-rbac-permissions-on-key-vault)
  * Ensure that the role assigned to the key vault should be able to create, retrieve and delete secrets in the key vault.

Provide the following KMS connection details in order to connect with Azure Key Vault.

```yaml
security:
  kms:
    connectionDetails:
      KMS_PROVIDER: azure-kv
      AZURE_VAULT_URL: https://<key-vault name>.vault.azure.net
      AZURE_CLIENT_ID: Application ID of an Azure service principal
      AZURE_TENANT_ID: ID of the application's Microsoft Entra tenant
      AZURE_CERT_SECRET_NAME: <name of the k8s secret containing the certificate along with the private key (without password protection)>
```

* `AZURE_CERT_SECRET_NAME` should hold the name of the k8s secret. The secret data should be base64 encoded certificate along with private key (without password protection)
