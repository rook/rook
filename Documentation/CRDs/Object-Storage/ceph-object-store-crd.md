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
    #    cpu: "500m"
    #    memory: "1024Mi"
    #  requests:
    #    cpu: "500m"
    #    memory: "1024Mi"
  #zone:
    #name: zone-a
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

## Gateway Settings

The gateway settings correspond to the RGW daemon settings.

* `type`: `S3` is supported
* `sslCertificateRef`: If specified, this is the name of the Kubernetes secret(`opaque` or `tls`
  type) that contains the TLS certificate to be used for secure connections to the object store.
  If it is an opaque Kubernetes Secret, Rook will look in the secret provided at the `cert` key name. The value of the `cert` key must be
  in the format expected by the [RGW
  service](https://docs.ceph.com/docs/master/install/ceph-deploy/install-ceph-gateway/#using-ssl-with-civetweb):
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
* `securePort`: The secure port on which RGW pods will be listening. A TLS certificate must be specified either via `sslCerticateRef` or `service.annotations`
* `instances`: The number of pods that will be started to load balance this object store.
* `externalRgwEndpoints`: A list of IP addresses to connect to external existing Rados Gateways
  (works with external mode). This setting will be ignored if the `CephCluster` does not have
  `external` spec enabled. Refer to the [external cluster section](../Cluster/ceph-cluster-crd.md#external-cluster)
  for more details. Multiple endpoints can be given, but for stability of ObjectBucketClaims, we
	highly recommend that users give only a single external RGW endpoint that is a load balancer that
	sends requests to the multiple RGWs.
* `annotations`: Key value pair list of annotations to add.
* `labels`: Key value pair list of labels to add.
* `placement`: The Kubernetes placement settings to determine where the RGW pods should be started in the cluster.
* `resources`: Set resource requests/limits for the Gateway Pod(s), see [Resource Requirements/Limits](../Cluster/ceph-cluster-crd.md#resource-requirementslimits).
* `priorityClassName`: Set priority class name for the Gateway Pod(s)
* `service`: The annotations to set on to the Kubernetes Service of RGW. The [service serving cert](https://docs.openshift.com/container-platform/4.6/security/certificates/service-serving-certificate.html) feature supported in Openshift is enabled by the following example:

```yaml
gateway:
  service:
    annotations:
      service.beta.openshift.io/serving-cert-secret-name: <name of TLS secret for automatic generation>
```

Example of external rgw endpoints to connect to:

```yaml
gateway:
  port: 80
  externalRgwEndpoints:
    - ip: 192.168.39.182
      # hostname: example.com
```

This will create a service with the endpoint `192.168.39.182` on port `80`, pointing to the Ceph object external gateway.
All the other settings from the gateway section will be ignored, except for `securePort`.

## Zone Settings

The [zone](../../Storage-Configuration/Object-Storage-RGW/ceph-object-multisite.md) settings allow the object store to join custom created [ceph-object-zone](ceph-object-zone-crd.md).

* `name`: the name of the ceph-object-zone the object store will be in.

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

* TLS authentication with custom certificates between Vault and CephObjectStore RGWs are supported from ceph v16.2.6 onwards
* `tokenSecretName` can be (and often will be) the same for both kms and s3 configurations.
* `AWS-SSE:S3` requires Ceph Quincy (v17.2.3) and later.

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
1. A status condition will be added to the CephObjectStore resource
1. An error will be added to the Rook Ceph Operator log

If the CephObjectStore is configured in a [multisite setup](../../Storage-Configuration/Object-Storage-RGW/ceph-object-multisite.md) the above conditions are applicable only to stores that belong to a single master zone.
Otherwise the conditions are ignored. Even if the store is removed the user can access the
data from a peer object store.
