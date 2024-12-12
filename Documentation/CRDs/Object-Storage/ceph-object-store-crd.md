---
title: CephObjectStore CRD
---

Rook allows creation and customization of object stores through the custom resource definitions (CRDs). The following settings are available for Ceph object stores.

## Example

### Erasure Coded

Erasure coded pools can only be used with `dataPools`. The `metadataPool` must use a replicated pool.

!!! note
    This sample requires *at least 3 bluestore OSDs*, with each OSD located on a *different node*.

The OSDs must be located on different nodes, because the [`failureDomain`](../Block-Storage/ceph-block-pool-crd.md#spec) is set to `host` and the `erasureCoded` chunk settings require at least 3 different OSDs (2 `dataChunks` + 1 `codingChunks`).

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStore
metadata:
  name: my-store
  namespace: rook-ceph
spec:
  metadataPool:
    failureDomain: host
    replicated:
      size: 3
  dataPool:
    failureDomain: host
    erasureCoded:
      dataChunks: 2
      codingChunks: 1
  preservePoolsOnDelete: true
  gateway:
    # sslCertificateRef:
    # caBundleRef:
    port: 80
    # securePort: 443
    instances: 1
    # A key/value list of annotations
    annotations:
    #  key: value
    placement:
    #  nodeAffinity:
    #    requiredDuringSchedulingIgnoredDuringExecution:
    #      nodeSelectorTerms:
    #      - matchExpressions:
    #        - key: role
    #          operator: In
    #          values:
    #          - rgw-node
    #  tolerations:
    #  - key: rgw-node
    #    operator: Exists
    #  podAffinity:
    #  podAntiAffinity:
    #  topologySpreadConstraints:
    resources:
    #  limits:
    #    memory: "1024Mi"
    #  requests:
    #    cpu: "500m"
    #    memory: "1024Mi"
  #zone:
    #name: zone-a
  #hosting:
  #  advertiseEndpoint:
  #    dnsName: "mystore.example.com"
  #    port: 80
  #    useTls: false
  #  dnsNames:
  #    - "mystore.example.org"
```

## Object Store Settings

### Metadata

* `name`: The name of the object store to create, which will be reflected in the pool and other resource names.
* `namespace`: The namespace of the Rook cluster where the object store is created.

### Pools

The pools allow all of the settings defined in the Block Pool CRD spec. For more details, see the [Block Pool CRD](../Block-Storage/ceph-block-pool-crd.md) settings. In the example above, there must be at least three hosts (size 3) and at least three devices (2 data + 1 coding chunks) in the cluster.

When the `zone` section is set pools with the object stores name will not be created since the object-store will the using the pools created by the ceph-object-zone.

* `metadataPool`: The settings used to create all of the object store metadata pools. Must use replication.
* `dataPool`: The settings to create the object store data pool. Can use replication or erasure coding.
* `preservePoolsOnDelete`: If it is set to 'true' the pools used to support the object store will remain when the object store
    will be deleted. This is a security measure to avoid accidental loss of data. It is set to 'false' by default. If not specified
    is also deemed as 'false'.
* `allowUsersInNamespaces`: If a CephObjectStoreUser is created in a namespace other than the Rook cluster namespace,
    the namespace must be added to this list of allowed namespaces, or specify "*" to allow all namespaces.
    This is useful for applications that need object store credentials to be created in their own namespace,
    where neither OBCs nor COSI is being used to create buckets. The default is empty.

## Auth Settings

The `auth`-section allows the configuration of authentication providers in addition to the regular authentication mechanism.

Currently only OpenStack Keystone is supported.

### Keystone Settings

The keystone authentication can be configured in the `spec.auth.keystone` section of the CRD:

```yaml
spec:
  [...]
  auth:
    keystone:
      acceptedRoles:
        - admin
        - member
        - service
      implicitTenants: "swift"
      revocationInterval: 1200
      serviceUserSecretName: usersecret
      tokenCacheSize: 1000
      url: https://keystone.example-namespace.svc/
  protocols:
    swift:
      accountInUrl: true
      urlPrefix: swift
  [...]
```

Note: With this example configuration S3 is implicitly enabled even though it is not enabled in the `protocols` section.

The following options can be configured in the `keystone`-section:

* `acceptedRoles`: The OpenStack Keystone [roles](https://docs.openstack.org/keystone/latest/admin/cli-manage-projects-users-and-roles.html#roles-and-role-assignments) accepted by RGW when authenticating against Keystone.
* `implicitTenants`: Indicates whether to use implicit tenants. This can be `true`, `false`, `swift` and `s3`. For more details see the Ceph RadosGW documentation on [multitenancy](https://docs.ceph.com/en/latest/radosgw/multitenancy/).
* `revocationInterval`: The number of seconds between token revocation checks.
* `serviceUserSecretName`: the name of the user secret containing the credentials for the admin user to use by rgw when communicating with Keystone. See [Object Store with Keystone and Swift](../../Storage-Configuration/Object-Storage-RGW/ceph-object-swift.md) for more details on what the secret must contain.
* `tokenCacheSize`: specifies the maximum number of entries in each Keystone token cache.
* `url`: The url of the Keystone API endpoint to use.

### Protocols Settings

The protocols section is divided into three parts:

- `enableAPIs` - list of APIs to be enabled in RGW instance. If no values set, all APIs will be enabled. Possible values: `s3, s3website, swift, swift_auth, admin, sts, iam, notifications`. Represents RGW [rgw_enable_apis](https://docs.ceph.com/en/reef/radosgw/config-ref/#confval-rgw_enable_apis) config parameter.
- a section to configure S3
- a section to configure swift

```yaml
spec:
  [...]
  protocols:
    enableAPIs: []
    swift:
      # a section to configure swift
    s3:
      # a section to configure s3
  [...]
```

#### protocols/S3 settings

In the `s3` section of the `protocols` section the following options can be configured:

* `authKeystone`: Whether S3 should also authenticated using Keystone (`true`) or not (`false`). If set to `false` the default S3 auth will be used.
* `enabled`: Whether to enable S3 (`true`) or not (`false`). The default is `true` even if the section is not listed at all! Please note that S3 should not be disabled in a [Ceph Multi Site configuration](https://docs.ceph.com/en/latest/radosgw/multisite).

#### protocols/swift settings

In the `swift` section of the `protocols` section the following options can be configured:

* `accountInUrl`: Whether or not the Swift account name should be included in the Swift API URL. If set to `false` (the default), the Swift API will listen on a URL formed like `http://host:port/<rgw_swift_url_prefix>/v1`. If set to `true`, the Swift API URL will be `http://host:port/<rgw_swift_url_prefix>/v1/AUTH_<account_name>`. This option must be set to `true` if radosgw should support publicly-readable containers and temporary URLs.
* `urlPrefix`: The URL prefix for the Swift API, to distinguish it from the S3 API endpoint. The default is `swift`, which makes the Swift API available at the URL `http://host:port/swift/v1` (or `http://host:port/swift/v1/AUTH_%(tenant_id)s` if rgw swift account in url is enabled). "Warning: If you set this option to `/`, the S3 API is automatically disabled. It is not possible to operate radosgw with an urlPrefix of `/` and simultaneously support both the S3 and Swift APIs. [...]" [(see Ceph documentation on swift settings)](https://docs.ceph.com/en/octopus/radosgw/config-ref/#swift-settings).
* `versioningEnabled`: If set to `true`, enables the Object Versioning of OpenStack Object Storage API. This allows clients to put the X-Versions-Location attribute on containers that should be versioned.

## Gateway Settings

The gateway settings correspond to the RGW daemon settings.

* `type`: `S3` is supported
* `sslCertificateRef`: If specified, this is the name of the Kubernetes secret(`opaque` or `tls`
    type) that contains the TLS certificate to be used for secure connections to the object store.
    If it is an opaque Kubernetes Secret, Rook will look in the secret provided at the `cert` key name. The value of the `cert` key must be
    in the format expected by the [RGW service](https://docs.ceph.com/docs/master/install/ceph-deploy/install-ceph-gateway/#using-ssl-with-civetweb):
    "The server key, server certificate, and any other CA or intermediate certificates be supplied in
    one file. Each of these items must be in PEM form." They are scenarios where the certificate DNS is set for a particular domain
    that does not include the local Kubernetes DNS, namely the object store DNS service endpoint. If
    adding the service DNS name to the certificate is not empty another key can be specified in the
    secret's data: `insecureSkipVerify: true` to skip the certificate verification. It is not
    recommended to enable this option since TLS is susceptible to machine-in-the-middle attacks unless
    custom verification is used.
* `caBundleRef`: If specified, this is the name of the Kubernetes secret (type `opaque`) that
    contains additional custom ca-bundle to use. The secret must be in the same namespace as the Rook
    cluster. Rook will look in the secret provided at the `cabundle` key name.
* `hostNetwork`: Whether host networking is enabled for the rgw daemon. If not set, the network settings from the cluster CR will be applied.
* `port`: The port on which the Object service will be reachable. If host networking is enabled, the RGW daemons will also listen on that port. If running on SDN, the RGW daemon listening port will be 8080 internally.
* `securePort`: The secure port on which RGW pods will be listening. A TLS certificate must be
    specified either via `sslCerticateRef` or `service.annotations`. Refer to
    [enabling TLS](../../Storage-Configuration/Object-Storage-RGW/object-storage.md#enabling-tls)
    documentation for more details.
* `instances`: The number of pods that will be started to load balance this object store.
* `externalRgwEndpoints`: A list of IP addresses to connect to external existing Rados Gateways
    (works with external mode). This setting will be ignored if the `CephCluster` does not have
    `external` spec enabled. Refer to the [external cluster section](../Cluster/ceph-cluster-crd.md#external-cluster)
    for more details. Multiple endpoints can be given, but for stability of ObjectBucketClaims, we
    highly recommend that users give only a single external RGW endpoint that is a load balancer that
    sends requests to the multiple RGWs.

    Example of external rgw endpoints to connect to:

    ```yaml
    gateway:
    port: 80
    externalRgwEndpoints:
      - ip: 192.168.39.182
        # hostname: example.com
    ```

* `annotations`: Key value pair list of annotations to add.
* `labels`: Key value pair list of labels to add.
* `placement`: The Kubernetes placement settings to determine where the RGW pods should be started in the cluster.
* `resources`: Set resource requests/limits for the Gateway Pod(s), see [Resource Requirements/Limits](../Cluster/ceph-cluster-crd.md#resource-requirementslimits).
* `priorityClassName`: Set priority class name for the Gateway Pod(s)
* `additionalVolumeMounts`: additional volumes to be mounted to the RGW pod. The root directory for
    each additional volume mount is `/var/rgw`. Each volume mount has a `subPath` that defines the
    subdirectory where that volumes files will be mounted. Rook supports several standard Kubernetes
    volume types. Example: for an additional mount at subPath `ldap`, mounted from a secret that has
    key `bindpass.secret`, the file would reside at `/var/rgw/ldap/bindpass.secret`.
* `service`: The annotations to set on to the Kubernetes Service of RGW. The [service serving cert](https://docs.openshift.com/container-platform/4.6/security/certificates/service-serving-certificate.html) feature supported in Openshift is enabled by the following example:

    ```yaml
    gateway:
    service:
      annotations:
      service.beta.openshift.io/serving-cert-secret-name: <name of TLS secret for automatic generation>
    ```

## Zone Settings

The [zone](../../Storage-Configuration/Object-Storage-RGW/ceph-object-multisite.md) settings allow the object store to join custom created [ceph-object-zone](ceph-object-zone-crd.md).

* `name`: the name of the ceph-object-zone the object store will be in.

## Hosting Settings

`hosting` settings allow specifying object store endpoint configurations. These settings are only
supported for Ceph v18 and higher.

A common use case that requires configuring hosting is allowing
[virtual host-style](https://docs.aws.amazon.com/AmazonS3/latest/userguide/VirtualHosting.html)
bucket access. This use case is discussed in more detail in
[Rook object storage docs](../../Storage-Configuration/Object-Storage-RGW/object-storage.md#virtual-host-style-bucket-access).

* `advertiseEndpoint`: By default, Rook advertises the most direct connection to RGWs to dependent
    resources like CephObjectStoreUsers and ObjectBucketClaims. To advertise a different address
    (e.g., a wildcard-enabled ingress), define the preferred endpoint here. Default behavior is
    documented in more detail [here](../../Storage-Configuration/Object-Storage-RGW/object-storage.md#object-store-endpoint)
    * `dnsName`: The valid RFC-1123 (sub)domain name of the endpoint.
    * `port`: The nonzero port of the endpoint.
    * `useTls`: Set to true if the endpoint is HTTPS. False if HTTP.
* `dnsNames`: When this or `advertiseEndpoint` is set, Ceph RGW will reject S3 client connections
    who attempt to reach the object store via any unspecified DNS name. Add all DNS names that the
    object store should accept here. These must be valid RFC-1123 (sub)domain names.
    Rook automatically adds the known CephObjectStore service DNS name to this list, as well as
    corresponding CephObjectZone `customEndpoints` (if applicable).

!!! Note
    For DNS names that support wildcards, do not include wildcards.
    E.g., use `mystore.example.com` instead of `*.mystore.example.com`.

## Runtime settings

### MIME types

Rook provides a default `mime.types` file for each Ceph object store. This file is stored in a
Kubernetes ConfigMap with the name `rook-ceph-rgw-<STORE-NAME>-mime-types`. For most users, the
default file should suffice, however, the option is available to users to edit the `mime.types`
file in the ConfigMap as they desire. Users may have their own special file types, and particularly
security conscious users may wish to pare down the file to reduce the possibility of a file type
execution attack.

Rook will not overwrite an existing `mime.types` ConfigMap so that user modifications will not be
destroyed. If the object store is destroyed and recreated, the ConfigMap will also be destroyed and
created anew.

## Health settings

Rook will be default monitor the state of the object store endpoints.
The following CRD settings are available:

* `healthCheck`: main object store health monitoring section
    * `startupProbe`: Disable, or override timing and threshold values of the object gateway startup probe.
    * `readinessProbe`: Disable, or override timing and threshold values of the object gateway readiness probe.

Here is a complete example:

```yaml
healthCheck:
  startupProbe:
    disabled: false
  readinessProbe:
    disabled: false
    periodSeconds: 5
    failureThreshold: 2
```

You can monitor the health of a CephObjectStore by monitoring the gateway deployments it creates.
The primary deployment created is named `rook-ceph-rgw-<store-name>-a` where `store-name` is the
name of the CephObjectStore (don't forget the `-a` at the end).

## Security settings

Ceph RGW supports Server Side Encryption as defined in [AWS S3 protocol](https://docs.aws.amazon.com/AmazonS3/latest/userguide/serv-side-encryption.html) with three different modes: AWS-SSE:C, AWS-SSE:KMS and AWS-SSE:S3. The last two modes require a Key Management System (KMS) like HashiCorp Vault. Currently, Vault is the only supported KMS backend for CephObjectStore.

Refer to the [Vault KMS section](../../Storage-Configuration/Advanced/key-management-system.md#vault) for details about Vault. If these settings are defined, then RGW will establish a connection between Vault and whenever S3 client sends request with Server Side Encryption. [Ceph's Vault documentation](https://docs.ceph.com/en/latest/radosgw/vault/) has more details.

The `security` section contains settings related to KMS encryption of the RGW.

```yaml
security:
  kms:
    connectionDetails:
      KMS_PROVIDER: vault
      VAULT_ADDR: http://vault.default.svc.cluster.local:8200
      VAULT_BACKEND_PATH: rgw
      VAULT_SECRET_ENGINE: kv
      VAULT_BACKEND: v2
    # name of the k8s secret containing the kms authentication token
    tokenSecretName: rgw-vault-kms-token
  s3:
    connectionDetails:
      KMS_PROVIDER: vault
      VAULT_ADDR: http://vault.default.svc.cluster.local:8200
      VAULT_BACKEND_PATH: rgw
      VAULT_SECRET_ENGINE: transit
    # name of the k8s secret containing the kms authentication token
    tokenSecretName: rgw-vault-s3-token
```

For RGW, please note the following:

* `VAULT_SECRET_ENGINE`: the secret engine which Vault should use. Currently supports [kv](https://www.vaultproject.io/docs/secrets/kv) and [transit](https://www.vaultproject.io/docs/secrets/transit). AWS-SSE:KMS supports `transit` engine and `kv` engine version 2. AWS-SSE:S3 only supports `transit` engine.
* The Storage administrator needs to create a secret in the Vault server so that S3 clients use that key for encryption for AWS-SSE:KMS

```console
vault kv put rook/<mybucketkey> key=$(openssl rand -base64 32) # kv engine
vault write -f transit/keys/<mybucketkey> exportable=true # transit engine
```

* `tokenSecretName` can be (and often will be) the same for both kms and s3 configurations.

## Advanced configuration

!!! warning
    This feature is intended for advanced users. It allows breaking configurations to be easily
    applied. Use with caution.

CephObjectStore allows arbitrary Ceph configurations to be applied to RGW daemons that serve the
object store. [RGW config reference](https://docs.ceph.com/en/latest/radosgw/config-ref/).

Configurations are applied to all RGWs that serve the CephObjectStore. Values must be strings.
Below is an example showing how different RGW configs and values might be applied. The example is
intended only to show a selection of value data types.

```yaml
# THIS SAMPLE IS NOT A RECOMMENDATION
# ...
spec:
  gateway:
    # ...
    rgwConfig:
      debug_rgw: "10" # int
      # debug-rgw: "20" # equivalent config keys can have dashes or underscores
      rgw_s3_auth_use_ldap: "true" # bool
    rgwCommandFlags:
      rgw_dmclock_auth_res: "100.0" # float
      rgw_d4n_l1_datacache_persistent_path: /var/log/rook/rgwd4ncache # string
      rgw_d4n_address: "127.0.0.1:6379" # IP string
```

* `rgwConfig` - These configurations are applied and modified at runtime, without RGW restart.
* `rgwCommandFlags` - These configurations are applied as CLI arguments and result in RGW daemons
    restarting when updates are applied. Restarts are desired behavior for some RGW configs.

!!! note
    Once an `rgwConfig` is set, it will not be removed from Ceph's central config store when removed
    from the `rgwConfig` spec. Be sure to specifically set values back to their defaults once done.
    With this in mind, `rgwCommandFlags` may be a better choice for temporary config values like
    debug levels.

### Example - debugging

Users are often asked to provide RGW logs at a high log level when troubleshooting complex issues.
Apply log levels to RGWs easily using `rgwCommandFlags`.

This spec will restart the RGW(s) with the highest level debugging enabled.

```yaml
# ...
spec:
  gateway:
    # ...
    rgwCommandFlags:
      debug_ms: "20"
      debug_rgw: "20"
```

Once RGW debug logging is no longer needed, the values can simply be removed from the spec.

## Deleting a CephObjectStore

During deletion of a CephObjectStore resource, Rook protects against accidental or premature
destruction of user data by blocking deletion if there are any object buckets in the object store
being deleted. Buckets may have been created by users or by ObjectBucketClaims.

For deletion to be successful, all buckets in the object store must be removed. This may require
manual deletion or removal of all ObjectBucketClaims. Alternately, the
`cephobjectstore.ceph.rook.io` finalizer on the CephObjectStore can be removed to remove the
Kubernetes Custom Resource, but the Ceph pools which store the data will not be removed in this case.

Rook will warn about which buckets are blocking deletion in three ways:

1. An event will be registered on the CephObjectStore resource
2. A status condition will be added to the CephObjectStore resource
3. An error will be added to the Rook Ceph Operator log

If the CephObjectStore is configured in a [multisite setup](../../Storage-Configuration/Object-Storage-RGW/ceph-object-multisite.md) the above conditions are applicable only to stores that belong to a single master zone.
Otherwise the conditions are ignored. Even if the store is removed the user can access the
data from a peer object store.
