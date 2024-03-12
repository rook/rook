# Rook Object Store

## Overview

An object store is a collection of resources and services that work together to serve HTTP requests to PUT and GET objects. Rook will automate the configuration of the Ceph resources and services that are necessary to start and maintain a highly available, durable, and performant object store.

The Ceph object store supports S3 and Swift APIs and a multitude of features such as replication of object stores between different zones. The Rook object store is designed to support all of these features, though will take some time to implement them. We welcome contributions! In the meantime, features that are not yet implemented can be configured by using the [Rook toolbox](/Documentation/Troubleshooting/ceph-toolbox.md) to run the `radosgw-admin` and other tools for advanced configuration.

### Prerequisites

A Rook storage cluster must be configured and running in Kubernetes. In this example, it is assumed the cluster is in the `rook` namespace.

## Object Store Walkthrough

When the storage admin is ready to create an object storage, the admin will specify his desired configuration settings in a yaml file such as the following `object-store.yaml`. This example is a simple object store with metadata that is replicated across different hosts, and the data is erasure coded across multiple devices in the cluster.

```yaml
apiVersion: ceph.rook.io/v1alpha1
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
    failureDomain: device
    erasureCoded:
      dataChunks: 6
      codingChunks: 2
  gateway:
    port: 80
    securePort: 443
    instances: 3
    hosting:
      dnsNames:
      - "my-ingress.mydomain.com"
      - "rook-ceph-rgw-my-store.rook-ceph.svc"
```

Now create the object store.
```bash
kubectl create -f object-store.yaml
```

At this point the Rook operator recognizes that a new object store resource needs to be configured. The operator will create all of the resources to start the object store.
1. Metadata pools are created (`.rgw.root`, `my-store.rgw.control`, `my-store.rgw.meta`, `my-store.rgw.log`, `my-store.rgw.buckets.index`)
2. The data pool is created (`my-store.rgw.buckets.data`)
3. A Kubernetes service is created to provide load balancing for the RGW pod(s)
4. A Kubernetes deployment is created to start the RGW pod(s) with the settings for the new zone
5. The zone is modified to add the RGW pod endpoint(s) if zone is mentioned in the configuration

When the RGW pods start, the object store is ready to receive the http or https requests as configured.

## Object Store CRD

The object store settings are exposed to Rook as a Custom Resource Definition (CRD). The CRD is the Kubernetes-native means by which the Rook operator can watch for new resources. The operator stays in a control loop to watch for a new object store, changes to an existing object store, or requests to delete an object store.

### Pools

#### Pools created by CephObjectStore CRD

The pools are the backing data store for the object store and are created with specific names to be private to an object store. Pools can be configured with all of the settings that can be specified in the [Pool CRD](/Documentation/CRDs/Block-Storage/ceph-block-pool-crd.md). The underlying schema for pools defined by a pool CRD is the same as the schema under the `metadataPool` and `dataPool` elements of the object store CRD. All metadata pools are created with the same settings, while the data pool can be created with independent settings. The metadata pools must use replication, while the data pool can use replication or erasure coding.

If `preservePoolsOnDelete` is set to 'true' the pools used to support the object store will remain when the object store will be deleted. This is a security measure to avoid accidental loss of data. It is set to 'false' by default. If not specified is also deemed as 'false'.

```yaml
spec:
  metadataPool:
    failureDomain: host
    replicated:
      size: 3
  dataPool:
    failureDomain: device
    erasureCoded:
      dataChunks: 6
      codingChunks: 2
  preservePoolsOnDelete: true
```

#### Pools shared by multiple CephObjectStore

If user want to use existing pools for metadata and data, the pools must be created before the object store is created. This will be useful if multiple objectstore can share same pools. The detail of pools need to shared in `sharedPools` settings in object-store CRD. Now the object stores can consume same pool isolated with different namespaces. Usually RGW server itself create different [namespaces](https://docs.ceph.com/en/latest/radosgw/layout/#appendix-compendium) on the pools. User can create via [Pool CRD](/Documentation/CRDs/Block-Storage/ceph-block-pool-crd.md), this is need to present before the object store is created. Similar to `preservePoolsOnDelete` setting, `preserveRadosNamespaceDataOnDelete` is used to preserve the data in the rados namespace when the object store is deleted. It is set to 'false' by default.

```yaml
spec:
  sharedPools:
    metadataPoolName: rgw-meta-pool
    dataPoolName: rgw-data-pool
    preserveRadosNamespaceDataOnDelete: true
```

To create the pools that will be shared by multiple object stores, create the following CephBlockPool CRs:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
    name: rgw-meta-pool
  spec:
    failureDomain: host
    replicated:
      size: 3
    parameters:
      pg_num: 8
    application: rgw
---
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
 metadata:
    name: rgw-data-pool
  spec:
    failureDomain: osd
      erasureCoded:
        dataChunks: 6
        codingChunks: 2
    application: rgw
---
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
 metadata:
    name: .rgw.root
  spec:
    name: .rgw.root
    failureDomain: host
    replicated:
      size: 3
    parameters:
      pg_num: 8
    application: rgw
```

The pools for this configuration will be created as below:

```bash
# ceph osd pool ls
.rgw.root
rgw-meta-pool
rgw-data-pool
```

And the pool configuration in zone is as below:

```json
# radosgw-admin zone get --rgw-zone=my-store
{
    "id": "2220eb5f-2751-4a51-9c7d-da4ce1b0e4e1",
    "name": "my-store",
    "domain_root": "rgw-meta-pool:my-store.meta.root",
    "control_pool": "rgw-meta-pool:my-store.control",
    "gc_pool": "rgw-meta-pool:my-store.log.gc",
    "lc_pool": "rgw-meta-pool:my-store.log.lc",
    "log_pool": "rgw-meta-pool:my-store.log",
    "intent_log_pool": "rgw-meta-pool:my-store.log.intent",
    "usage_log_pool": "rgw-meta-pool:my-store.log.usage",
    "roles_pool": "rgw-meta-pool:my-store.meta.roles",
    "reshard_pool": "rgw-meta-pool:my-store.log.reshard",
    "user_keys_pool": "rgw-meta-pool:my-store.meta.users.keys",
    "user_email_pool": "rgw-meta-pool:my-store.meta.users.email",
    "user_swift_pool": "rgw-meta-pool:my-store.meta.users.swift",
    "user_uid_pool": "rgw-meta-pool:my-store.meta.users.uid",
    "otp_pool": "rgw-meta-pool:my-store.otp",
    "system_key": {
        "access_key": "",
        "secret_key": ""
    },
    "placement_pools": [
        {
            "key": "default-placement",
            "val": {
                "index_pool": "rgw-metadata-pool:my-store.buckets.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "rgw-data-pool:my-store.buckets.data"
                    }
                },
                "data_extra_pool": "rgw-data-pool:my-store.buckets.non-ec", #only pool is not erasure coded, otherwise use different pool
                "index_type": 0,
                "inline_data": "true"
            }
        }
    ],
    "realm_id": "65a7bf34-42d3-4344-ac40-035e160d7f9e",
    "notif_pool": "rgw-meta-pool:my-store.notif"
}
```

The following steps need to implement internally in Rook Operator to add this feature assuming zone and pool are already created:

```bash

# radosgw-admin zone get --rgw-zone=my-store > zone.json

# modify the zone.json to add the pools as above

# radosgw-admin zone set --rgw-zone=my-store --infile=zone.json

# radosgw-admin period update --commit
```

After deleting the object store the data in the rados namespace can deleted by Rook Operator using following commands:

```bash

# rados -p rgw-meta-pool ls --all | grep my-store

# rados -p rgw-data-pool ls --all | grep my-store

# rados -p <rgw-meta-pool | rgw-data-pool> rm <each object> -N <namespace> (from above list)
```

#### For mulisite use case

If there is a `zone` section in object-store configuration, then the pool creation will configured by the [CephObjectZone CR](/Documentation/CRDs/Object-Storage/ceph-object-zone-crd.md). The `CephObjectStore` CR will include below section to specify the zone name.

```yaml
spec:
  zone:
    name: zone1
```

### Gateway

The gateway settings correspond to the RGW service.
- `type`: Can be `s3`. In the future support for `swift` can be added.
- `sslCertificateRef`: If specified, this is the name of the Kubernetes secret that contains the SSL
  certificate to be used for secure connections to the object store. The secret must be in the same
  namespace as the Rook cluster. If it is an opaque Kubernetes Secret, Rook will look in the secret
  provided at the `cert` key name. The value of the `cert` key must be in the format expected by the
  [RGW
  service](https://docs.ceph.com/docs/master/install/ceph-deploy/install-ceph-gateway/#using-ssl-with-civetweb):
  "The server key, server certificate, and any other CA or intermediate certificates be supplied in
  one file. Each of these items must be in pem form." If the certificate is not specified, SSL will
  not be configured.
- `caBundleRef`: If specified, this is the name of the Kubernetes secret (type `opaque`) that contains ca-bundle to use. The secret must be in the same namespace as the Rook cluster. Rook will look in the secret provided at the `cabundle` key name.
- `port`: The service port where the RGW service will be listening (http)
- `securePort`: The service port where the RGW service will be listening (https)
- `instances`: The number of RGW pods that will be started for this object store
- `placement`: The rgw pods can be given standard Kubernetes placement restrictions with `nodeAffinity`, `tolerations`, `podAffinity`, `podAntiAffinity`, and `topologySpreadConstraints` similar to placement defined for daemons configured by the [cluster CRD](/deploy/examples/cluster.yaml).

The RGW service can be configured to listen on both http and https by specifying both `port` and `securePort`.

```yaml
 gateway:
    sslCertificateRef: my-ssl-cert-secret
    securePort: 443
    instances: 1
```

### Multisite

By default, the object store will be created independently from any other object stores and replication to another object store will not be configured. This done by creating a new Ceph realm, zone group, and zone all with the name of the new object store.

If desired to configure the object store to replicate and sync data amongst object-store or Ceph clusters, the `zone` section would be required.

This section enables the object store to be part of a specified ceph-object-zone.

Specifying this section also ensures that the pool section in the ceph-object-zone is used for the object-store. If pools are specified for the object-store they are neither created nor deleted.

- `name`: name of the [ceph-object-zone](/design/ceph/object/ceph-object-zone.md) the object store is in. This name must be of a ceph-object-zone resource not just of a zone that has been already created.

```yaml
  zone:
    name: "name"
```

### LDAP

The Ceph Object Gateway supports integrating with LDAP for authenticating and creating users, please refer [here](https://docs.ceph.com/docs/master/radosgw/ldap-auth/). This means that the `rgw backend user` is also required to be part of groups in the LDAP server otherwise, authentication will fail. The `rgw backend user` can be generated from `CephObjectStoreUser` or `ObjectBucketClaim` CRDs. For the both resources credentials are saved in Kubernetes Secrets which may not be valid with `LDAP Server`, user need to follow the steps mentioned [here](https://docs.ceph.com/en/latest/radosgw/ldap-auth/#generating-an-access-token-for-ldap-authentication).The following settings need to be configured in the RGW server:
```
rgw ldap binddn =
rgw ldap secret = /etc/ceph/ldap/bindpass.secret
rgw ldap uri =
rgw ldap searchdn =
rgw ldap dnattr =
rgw ldap searchfilter =
rgw s3 auth use ldap = true
```
So the CRD for the Ceph Object Store will be modified to include the above changes:

```yaml
spec:
  security
    ldap:
      config:
        uri: ldaps://ldap-server:636
        binddn: "uid=ceph,cn=users,cn=accounts,dc=example,dc=com"
        searchdn: "cn=users,cn=accounts,dc=example,dc=com"
        dnattr: "uid"
        searchfilter: "memberof=cn=s3,cn=groups,cn=accounts,dc=example,dc=com"
      credential:
        volumeSource:
          secret:
            secretName: object-my-store-ldap-creds
            defaultMode: 0600 #required

```

The `config` section includes options used for RGW wrt LDAP server. These options are strongly typed rather than string map approach since very less chance to modify in future.

* `uri`: It specifies the address of LDAP server to use.* `binddn`: The bind domain for the service account used by RGW server.
* `searchdn`: The search domain where can it look for the user details.
* `dnattr`: The attribute being used in the constructed search filter to match a username, this can either be `uid` or `cn`.
* `searchfilter`: A generic search filter. If `dnattr` is set, this filter is `&()`'d together with the automatically constructed filter.

The `credential` defines where the password for accessing ldap server should be sourced from

* `volumeSource`: this is a standard Kubernetes VolumeSource for the Kerberos keytab file like
      what is normally used to configure Volumes for a Pod. For example, a Secret or HostPath.
      There are two requirements for the source's content:
    1. The config file must be mountable via `subPath: password`. For example, in a Secret, the
        data item must be named `password`, or `items` must be defined to select the key and
        give it path `password`. A HostPath directory must have the `password` file.
    2. The volume or config file must have mode 0600.

The CA bundle for ldap can be added to the `caBundleRef` option in `Gateway` settings:

```yaml
spec:
  gateway:
    caBundleRef: #ldaps-cabundle
```

### Virtual Host Style access for buckets

The Ceph Object Gateway supports accessing buckets using [virtual host style](https://docs.aws.amazon.com/AmazonS3/latest/userguide/VirtualHosting.html) which allows accessing buckets using the bucket name as a subdomain in the endpoint. The user can configure this option manually like below:

```sh
# ceph config set client.rgw.mystore.a rgw_dns_name my-ingress.mydomain.com,rook-ceph-rgw-my-store.rook-ceph.svc
```

Multiple hostnames can be added to the list separated by comma. Each entry must be a valid RFC-1123 hostname, and Rook Operator will perform input validation using k8s apimachinery `IsDNS1123Subdomain()`.

When `rgw_dns_name` is changed for an RGW cluster, all RGWs need to be restarted. To enforce this in Rook, we can apply the `--rgw-dns-name` flag, which will restart RGWs with no user action needed. This is supported from Ceph Reef release(v18.0) onwards.

This is different from `customEndpoints` which is used for configuring the object store to replicate and sync data amongst Multisite.

The default service endpoint for the object store `rook-ceph-rgw-my-store.rook-ceph.svc` and `customEndpoints` in `CephObjectZone` need to be added automatically by the Rook operator otherwise existing object store may impacted. Also check for deduplication in the `rgw_dns_name` list if user manually add the default service endpoint.

For accessing the bucket point user need to configure wildcard dns in the cluster using [ingress loadbalancer](https://kubernetes.io/docs/concepts/services-networking/ingress/#hostname-wildcards) or in openshift cluster use [dns operator](https://docs.openshift.com/container-platform/latest/networking/dns-operator.html). Same for TLS certificate, user need to configure the TLS certificate for the wildcard dns for the RGW endpoint. This option won't be enabled by default, user need to enable it by adding the `hosting` section in the `Gateway` settings:

```yaml
spec:
  hosting:
    dnsNames:
    - "my-ingress.mydomain.com"
```

A list of hostnames to use for accessing the bucket directly like a subdomain in the endpoint. For example if the ingress service endpoint `my-ingress.mydomain.com` added and the object store contains a bucket named `sample`, then the s3 [virtual host style](https://docs.aws.amazon.com/AmazonS3/latest/userguide/VirtualHosting.html) would be `http://sample.my-ingress.mydomain.com`. More details about the feature can be found in [Ceph Documentation](https://docs.ceph.com/en/latest/radosgw/s3/commons/#bucket-and-host-name).

When this feature is enabled the endpoint in OBCs and COSI Bucket Access can be clubbed with bucket name and host name. For OBC  the `BUCKET_NAME` and the `BUCKET_HOST` from the config map combine to `http://$BUCKETNAME.$BUCKETHOST`. For COSI Bucket Access the `bucketName` and the `endpoint` in the `BucketInfo` to `http://bucketName.endpoint`.
