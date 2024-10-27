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

### Pool Placement targets and Storage Classes

Object Storage API allows users to specify where bucket data will be stored during bucket creation. With `<LocationConstraint>` parameter in S3 API and `X-Storage-Policy` header in SWIFT. Similarly, users can override where object data will be stored by setting `X-Amz-Storage-Class` and `X-Object-Storage-Class` during object creation.

In Ceph these data locations represented by Pools. Placement targets control which Pools are associated with a particular bucket. A bucket’s placement target is selected on creation, and cannot be modified (See [Ceph doc](https://docs.ceph.com/en/latest/radosgw/placement/#pool-placement-and-storage-classes)). Placement target can also define custom Storage classes to override data pool where object will be stored.

Ceph administrator creates set of `index_pool`, `data_pool`, and optional `data_extra_pool` per each target placement. Selects arbitrary names for target placements. For example `fast` and `cold`:
> [!NOTE]
> `data_extra_pool` is for data that cannot use erasure coding. For example, multi-part uploads allow uploading a large object such as a movie in multiple parts. These parts must first be stored without erasure coding. So if `data_pool` is **not** erasure coded, then there is not need for `data_extra_pool`.

```yaml
# Default Data pool
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: rgw-meta-pool
  namespace: rook-ceph
spec:
  ...
---
# Default index pool
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: rgw-data-pool
  namespace: rook-ceph
spec:
  ...
# Bucket Data pool for fast storage
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: rgw-fast-data-pool
  namespace: rook-ceph
spec:
  ...
---
# Bucket index pool for fast storage
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: rgw-fast-meta-pool
  namespace: rook-ceph
spec:
  ...
---
# Bucket Data pool for cold storage
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: rgw-cold-data-pool
  namespace: rook-ceph
spec:
  ...
---
# Bucket index pool for cold storage
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: rgw-cold-meta-pool
  namespace: rook-ceph
spec:
  ...
```

If needed, create additional pool to use it in custom StorageClass, for example `GLACIER`:
> [!NOTE]
> Ceph allows arbitrary name for StorageClasses, however some clients/libs insist on AWS names so it is recommended to use one of the [valid x-amz-storage-class values](https://docs.aws.amazon.com/AmazonS3/latest/API/API_PutObject.html#API_PutObject_RequestSyntax) for better compatibility:
>
> `STANDARD | REDUCED_REDUNDANCY | STANDARD_IA | ONEZONE_IA | INTELLIGENT_TIERING | GLACIER | DEEP_ARCHIVE | OUTPOSTS | GLACIER_IR | SNOW | EXPRESS_ONEZONE`

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: glacier-data-pool
  namespace: rook-ceph
spec:
  ...
```

Then update `CephObjectStore` CRD  `sharedPools` section with target placements and StorageClasses definitions:

```yaml
spec:
  # [OPTIONAL] - sharedPools is mutually exclusive with spec.metadataPool and spec.dataPool
  # It is the place to tell RGW which pools can be used to store buckets and objects data and metadata.
  # All pools under this section should be created beforehand.
  # Rook will append rados namespace to each of the pools, so it is safe to use the same pool across multiple RGW instances.
  # It is also safe to use the same pool in different target placements.
  sharedPools:
    #  [EXISTING,OPTIONAL] - metadata pool. Will be used by default to store bucket index
    # default poolPlacement.metadataPoolName will override this value, so it is not makes sense to use together with poolPlacements.
    metadataPoolName: rgw-meta-pool # results to rgw-meta-pool:<CephObjectStore.name>.index

    #  [EXISTING,OPTIONAL] - data pool. Will be used by default to store bucket objects
    # default poolPlacement.dataPoolName will override this value, so it is not makes sense to use together with poolPlacements.
    dataPoolName: rgw-data-pool # results to rgw-data-pool:<CephObjectStore.name>.data

    # New settings:
    # [OPTIONAL] - defines pool placement targets.
    # Placement targets control which Pools are associated with a particular bucket.
    # See: https://docs.ceph.com/en/latest/radosgw/placement/#placement-targets
    poolPlacements:
      # Default pool placement:
      - name: "default"                         # [REQUIRED] - placement id. Placement with name "default" will be used as default,
                                                # and therefore will override sharedPools.metadataPoolName and sharedPools.dataPoolName if presented.
                                                # This means that default poolPlacement is also mutually exclusive with spec.metadataPool and spec.dataPool
        metadataPoolName: rgw-meta-pool         # [REQUIRED] results to rgw-meta-pool:<CephObjectStore.name>.index
        dataPoolName: rgw-data-pool             # [REQUIRED] results to rgw-data-pool:<CephObjectStore.name>.data
        dataNonECPoolName: ""                   # [OPTIONAL] results to rgw-data-pool:<CephObjectStore.name>.data.nonec
        # [OPTIONAL] - Storage classes for default placement target.
        # Any placement has default storage class named STANDARD pointing to dataPoolName.
        # So stageClasses allows to define additional storage classes on top of STANDARD for given placement.
        storageClasses:
      # Pool placement target named "fast"
      - name: "fast"
        metadataPoolName: rgw-fast-meta-pool     # [REQUIRED] results to rgw-fast-meta-pool:<CephObjectStore.name>.fast.index
        dataPoolName: rgw-fast-data-pool         # [REQUIRED] results to rgw-fast-data-pool:<CephObjectStore.name>.fast.data
        dataNonECPoolName: ""                   # [OPTIONAL] results to rgw-fast-data-pool:<CephObjectStore.name>.fast.data.nonec
        # [OPTIONAL] - Additional Storage classes for fast placement target. Provide alternatives to default STANDARD storage clsss.
        # STANDARD storage class cannot be changed or removed and always points to placement dataPoolName to store object data.
        storageClasses:
      # Pool placement target named "cold"
      - name: "cold"                            # [REQUIRED] - placement id. Can be arbitrary.
        metadataPoolName: rgw-cold-meta-pool    # [REQUIRED] results to rgw-cold-meta-pool:<CephObjectStore.name>.cold.index
        dataPoolName: rgw-cold-data-pool        # [REQUIRED] results to rgw-cold-data-pool:<CephObjectStore.name>.cold.data
        dataNonECPoolName: ""                   # [OPTIONAL] results to rgw-cold-data-pool:<CephObjectStore.name>.cold.data.nonec
        # [OPTIONAL] - Additional Storage classes for cold placement target. Provides alternatives to default STANDARD storage clsss.
        # STANDARD storage class cannot be changed or removed and always points to placement dataPoolName to store object data.
        storageClasses:
          - name: GLACIER                       # [REQUIRED] StorageClass name. Can be arbitrary but it is better to use one of AWS x-amz-storage-class names.
            dataPoolName: glacier-data-pool     # [REQUIRED] results to glacier-data-pool:<CephObjectStore.name>.glacier
```

Here we specified that pools `rgw-meta-pool` and `rgw-data-pool` will be used by default. We also added extra placements named `fast` and `cold` with corresponding data and metadata pools. We also introduced extra storage class `GLACIER` for `cold` placement, which can be used to override object data pool to `glacier-data-pool` within `cold` placement.

Rook CephObjectStore controller reconciliation loop for target placements:

1. CephObjectStore CRD changed
2. Validate `spec.sharedPools`:
3. Check that listed pools are created.
4. Calculate zone and zonegroup name: `CephObjectStore.spec.zone.name | CephObjectStore.metadata.name` - equal to zone name for multisite, otherwise equal to rgw name.
4. get default zonegroup: `radosgw-admin zonegroup get --rgw-zonegroup=<name>`
    ```json
    {
        "placement_targets": [
            {
                "name": "default-placement",
                "tags": [],
                "storage_classes": [
                    "STANDARD"
                ]
            }
        ],
        "default_placement": "default-placement",
         ...
    }
    ```
5. update json to:
    ```json
    {
        "placement_targets": [
            {
                "name": "default-placement",
                "tags": [],
                "storage_classes": [
                    "STANDARD"
                ]
            }
            {
                "name": "fast",
                "tags": [],
                "storage_classes": [
                    "STANDARD"
                ]
            },
            {
                "name": "cold",
                "tags": [],
                "storage_classes": [
                    "STANDARD","GLACIER"
                ]
            }
        ],
        "default_placement": "default-placement",
         ...
    }
    ```
6. update with `radosgw-admin zonegroup set --rgw-zonegroup=<name> --infile zonegroup.json`
7. get default zone: `radosgw-admin zone get --rgw-zone=<name>`
    ```json
    {
        "placement_pools": [
            {
                "key": "default-placement",
                "val": {
                    "index_pool": "<CephObjectStore name>.rgw.buckets.index",
                    "storage_classes": {
                        "STANDARD": {
                            "data_pool": "<CephObjectStore name>.rgw.buckets.data"
                        }
                    },
                    "data_extra_pool": "<CephObjectStore name>.rgw.buckets.non-ec",
                    "index_type": 0,
                    "inline_data": true
                }
            }
        ],
        ...
    }
    ```
8. update json to
    ```json
    {
        "placement_pools": [
            {
                "key": "default-placement",
                "val": {
                    "index_pool": "rgw-meta-pool:<CephObjectStore.name>.index",
                    "storage_classes": {
                        "STANDARD": {
                            "data_pool": "rgw-data-pool:<CephObjectStore.name>.data"
                        }
                    },
                    "data_extra_pool": "rgw-data-pool:<CephObjectStore.name>.data.nonec",
                    "index_type": 0,
                    "inline_data": true
                }
            },
            {
                "key": "fast",
                "val": {
                    "index_pool": "rgw-cold-meta-pool:<CephObjectStore.name>.fast.index",
                    "storage_classes": {
                        "STANDARD": {
                            "data_pool": "rgw-fast-data-pool:<CephObjectStore.name>.fast.data"
                        },
                    },
                    "data_extra_pool": "rgw-fast-data-pool:<CephObjectStore.name>.fast.data.nonec",
                    "index_type": 0,
                    "inline_data": true
                }
            },
            {
                "key": "cold",
                "val": {
                    "index_pool": "rgw-cold-meta-pool:<CephObjectStore.name>.cold.index",
                    "storage_classes": {
                        "STANDARD": {
                            "data_pool": "rgw-cold-data-pool:<CephObjectStore.name>.cold.data"
                        },
                        "GLACIER": {
                            "data_pool": "glacier-data-pool:<CephObjectStore.name>.glacier"
                        }

                    },
                    "data_extra_pool": "rgw-cold-data-pool:<CephObjectStore.name>.cold.data.nonec",
                    "index_type": 0,
                    "inline_data": true
                }
            },
        ],
        ...
    }
    ```
9. update with `radosgw-admin zone set --rgw-zone=<name> --infile zone.json`
10. commit: `radosgw-admin period update --commit`

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

### Virtual host-style access for buckets

The Ceph Object Gateway supports accessing buckets using
[virtual host-style](https://docs.aws.amazon.com/AmazonS3/latest/userguide/VirtualHosting.html)
addressing, which allows accessing buckets using the bucket name as a subdomain in the endpoint.
This is important because AWS (the primary definer of the S3 interface) has deprecated the
alternative, path-style access which is Ceph (and Rook's) default deployment. AWS has extended
support for path-style access, but Rook/OBC/COSI users have begun to identify applications which do
not support the long-deprecated path-style access.

Virtual host-style (vhost-style) addressing requires 2 things:
1. An endpoint that supports [wildcard addressing](https://en.wikipedia.org/wiki/Wildcard_DNS_record)
2. The above DNS endpoint must be added to RGW's `rgw_dns_names` list

The user can configure this option manually in Rook like below for a wildcard-enabled endpoint:

```sh
# ceph config set client.rgw.mystore.a rgw_dns_name my-ingress.mydomain.com,rook-ceph-rgw-my-store.rook-ceph.svc
```

In this example, if the object store contains a bucket named `sample`, then the S3 vhost-style
access URL would be `sample.my-ingress.mydomain.com`. More details about the feature can be
found in [Ceph Documentation](https://docs.ceph.com/en/latest/radosgw/s3/commons/#bucket-and-host-name).
This is supported from Ceph Reef release(v18.0) onwards.

Wildcard DNS addressing can be configured in myriad ways. Some options:
- Kubernetes [ingress loadbalancer](https://kubernetes.io/docs/concepts/services-networking/ingress/#hostname-wildcards)
- Openshift [DNS operator](https://docs.openshift.com/container-platform/latest/networking/dns-operator.html)

Rook will implement the following API to allow for user configuration, with detailed design notes to follow.

- `hosting` (optional) - configures hosting settings for the CephObjectStore
    - `advertiseEndpoint` (optional) - allow users to definitively specify which endpoint the user
      wants applications to use by default for internal S3 connections.
    - `dnsNames` (optional) - RGW will reject any S3 connections to unknown endpoints. Users should
      add any additional endpoints RGW should accept here.

The below example illustrates a commonly-anticipated configuration, and one that Rook can recommend
to users. The user is using an internal, wildcard-enabled K8s service as the advertised endpoint for
OBCs. The user also has externally-available ingress that the RGW needs to accept connections from,
which serves S3 applications outside the Kubernetes cluster. Both endpoints are wildcard-enabled,
but the internal service is preferable to the external ingress to prevent the ingress router from
being a cluster-wide S3 bottleneck for Kubernetes applications.

```yaml
spec:
  hosting:
    advertiseEndpoint:
      dnsName: my-internal-wildcard-service.mydomain.com
      port: 8443
      useTLS: true
    dnsNames:
      - my-external-ingress.mydomain.com
```

These proposed configurations are necessary for users to enable vhost-style addressing, but they are
not **only** useful when DNS wildcarding is enabled. These configurations are independent and may
provide some value to other user scenarios. Be clear in documentation that wildcarding is enabled by
these features but not required.

When the user configures DNS wildcarding on an endpoint other than the CephObjectStore service
endpoint, Rook should advertise that endpoint to CephObjectStores, OBCs, and COSI Buckets/Accesses
as a priority. However, Rook cannot know which endpoints support wildcarding in order to prioritize
advertising them. Therefore, allow users to disambiguate for Rook which endpoint should be
advertised via `advertiseEndpoint`.

S3 clients (and therefore OBCs and COSI) generally assume a single endpoint for an object store.
Multiple advertised endpoints will not be supported to avoid user and internal/developer confusion.

By default, Rook will advertise the CephObjectStore service endpoint with a priority on advertising
the HTTPS (`securePort`) endpoint. Because the advertised endpoint is primarily relevant for
resources internal to the Kubernetes cluster, this default should be sufficient for most users, and
this is the behavior expected by users when `dnsNames` is not configured, so it should be familiar.

When this feature is enabled, there should be no ambiguity about which endpoint Rook will use for
Admin Ops API communication. As an HTTP server, RGW is only able to return a single TLS certificate
to S3 clients ([more detail](https://github.com/rook/rook/issues/14530)). For maximum compatibility
while TLS is enabled, Rook should connect to the same endpoint that users do. Internally, Rook will
use the advertise endpoint as configured.

Rook documentation will inform users that if TLS is enabled, they must give Rook a certificate that
accepts the service endpoint. Alternately, if that is not possible, Rook will add an
`insecureSkipTlsVerification` option to the CephObjectStore to allow users to provision a healthy
CephObjectStore. This opens users up to machine-in-the-middle attacks, so users should be advised to
only use it for test/proof-of-concept clusters, or to work around bugs temporarily.

Some users have reported issues with Rook using a `dnsNames` endpoint
(or `advertiseEndpoint`) when they wish to set up ingress certificates after Rook deployment. The
obvious alternative is to have Rook always use the CephObjectStore service, but other users have
expressed troubles creating certificates or CAs that allow the service endpoint in the past.

Each `rgw_dns_name` entry must be a valid RFC-1123 hostname, and Rook Operator will perform input validation using k8s apimachinery `IsDNS1123Subdomain()`.

When `rgw_dns_name` is changed for an RGW cluster, all RGWs need to be restarted. To enforce this in Rook, we can apply the `--rgw-dns-name` flag, which will restart RGWs with no user action needed.

This is different from CephObjectZone `customEndpoints` which is used for configuring the object store to replicate and sync data amongst Multisite.

When endpoints are configured in `rgw_dns_names`, the RGW will reject any incoming connections
intended for endpoints not in the list. Therefore, all endpoints that might be used must be added.

For convenience, and to ensure other CephObjectStore configurations are not rendered unusable when
users add additional endpoints to `dnsNames`, the following should be added to the `rgw_dns_names`
list automatically by Rook:
- the `advertiseEndpoint.dnsName`
- the default service endpoint for the object store (e.g., `rook-ceph-rgw-my-store.rook-ceph.svc`)
- CephObjectZone `customEndpoints`

When Rook builds the `rgw_dns_names` list internally, Rook should remove any duplicate entries.
While Rook add endpoints to the list for safety and convenience, users might add the same endpoints,
which Rook should not treat as a configuration bug. Rook should also ensure the list ordering is
consistent between reconciles.

Rook can refer users to this Kubernetes doc for a suggested way that they can manage certificates
in a Kubernetes cluster that work with Kubernetes services like the CephObjectStore service:
https://kubernetes.io/docs/tasks/tls/managing-tls-in-a-cluster/

For external CephCephObjectStores (i.e., when `spec.gateway.externalRgwEndpoints` are set),
vhost-style addressing should be configured on the host cluster, and `hosting.dnsNames` is
irrelevant. The default `advertiseEndpoint` for external CephObjectStores is the first entry in the
`spec.gateway.externalRgwEndpoints` list, which users should be able to override if desired.
