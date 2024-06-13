---
title: External Storage Cluster
---

An external cluster is a Ceph configuration that is managed outside of the local K8s cluster. The external cluster could be managed by cephadm, or it could be another Rook cluster that is configured to allow the access (usually configured with host networking).

In external mode, Rook will provide the configuration for the CSI driver and other basic resources that allows your applications to connect to Ceph in the external cluster.

## External configuration

* Source cluster: The cluster providing the data, usually configured by [cephadm](https://docs.ceph.com/en/pacific/cephadm/#cephadm)

* Consumer cluster: The K8s cluster that will be consuming the external source cluster

## Prerequisites

Create the desired types of storage in the provider Ceph cluster:

* [RBD pools](https://docs.ceph.com/en/latest/rados/operations/pools/#create-a-pool)
* [CephFS filesystem](https://docs.ceph.com/en/quincy/cephfs/createfs/)

## Commands on the source Ceph cluster

In order to configure an external Ceph cluster with Rook, we need to extract some information in order to connect to that cluster.

### 1. Create all users and keys

Run the python script [create-external-cluster-resources.py](https://github.com/rook/rook/blob/master/deploy/examples/external/create-external-cluster-resources.py) for creating all users and keys.

```console
python3 create-external-cluster-resources.py --rbd-data-pool-name <pool_name> --cephfs-filesystem-name <filesystem-name> --rgw-endpoint  <rgw-endpoint> --namespace <namespace> --format bash
```

* `--namespace`: Namespace where CephCluster will run, for example `rook-ceph`
* `--format bash`: The format of the output
* `--rbd-data-pool-name`: The name of the RBD data pool
* `--alias-rbd-data-pool-name`: Provides an alias for the  RBD data pool name, necessary if a special character is present in the pool name such as a period or underscore
* `--rgw-endpoint`: (optional) The RADOS Gateway endpoint in the format `<IP>:<PORT>` or `<FQDN>:<PORT>`.
* `--rgw-pool-prefix`: (optional) The prefix of the RGW pools. If not specified, the default prefix is `default`
* `--rgw-tls-cert-path`: (optional) RADOS Gateway endpoint TLS certificate file path
* `--rgw-skip-tls`: (optional) Ignore TLS certification validation when a self-signed certificate is provided (NOT RECOMMENDED)
* `--rbd-metadata-ec-pool-name`: (optional) Provides the name of erasure coded RBD metadata pool, used for creating ECRBDStorageClass.
* `--monitoring-endpoint`: (optional) Ceph Manager prometheus exporter endpoints (comma separated list of IP entries of active and standby mgrs)
* `--monitoring-endpoint-port`: (optional) Ceph Manager prometheus exporter port
* `--skip-monitoring-endpoint`: (optional) Skip prometheus exporter endpoints, even if they are available. Useful if the prometheus module is not enabled
* `--ceph-conf`: (optional) Provide a Ceph conf file
* `--keyring`: (optional) Path to Ceph keyring file, to be used with `--ceph-conf`
* `--k8s-cluster-name`: (optional) Kubernetes cluster name
* `--output`: (optional) Output will be stored into the provided file
* `--dry-run`: (optional) Prints the executed commands without running them
* `--run-as-user`: (optional) Provides a user name to check the cluster's health status, must be prefixed by `client`.
* `--cephfs-metadata-pool-name`: (optional) Provides the name of the cephfs metadata pool
* `--cephfs-filesystem-name`: (optional) The name of the filesystem, used for creating CephFS StorageClass
* `--cephfs-data-pool-name`: (optional) Provides the name of the CephFS data pool, used for creating CephFS StorageClass
* `--rados-namespace`: (optional) Divides a pool into separate logical namespaces, used for creating RBD PVC in a CephBlockPoolRadosNamespace (should be lower case)
* `--subvolume-group`: (optional) Provides the name of the subvolume group, used for creating CephFS PVC in a subvolumeGroup
* `--rgw-realm-name`: (optional) Provides the name of the rgw-realm
* `--rgw-zone-name`: (optional) Provides the name of the rgw-zone
* `--rgw-zonegroup-name`: (optional) Provides the name of the rgw-zone-group
* `--upgrade`: (optional) Upgrades the cephCSIKeyrings(For example: client.csi-cephfs-provisioner) and client.healthchecker ceph users with new permissions needed for the new cluster version and older permission will still be applied.
* `--restricted-auth-permission`: (optional) Restrict cephCSIKeyrings auth permissions to specific pools, and cluster. Mandatory flags that need to be set are `--rbd-data-pool-name`, and `--k8s-cluster-name`. `--cephfs-filesystem-name` flag can also be passed in case of CephFS user restriction, so it can restrict users to particular CephFS filesystem.
* `--v2-port-enable`: (optional) Enables the v2 mon port (3300) for mons.
* `--topology-pools`: (optional) Comma-separated list of topology-constrained rbd pools
* `--topology-failure-domain-label`: (optional) K8s cluster failure domain label (example: zone, rack, or host) for the topology-pools that match the ceph domain
* `--topology-failure-domain-values`: (optional) Comma-separated list of the k8s cluster failure domain values corresponding to each of the pools in the `topology-pools` list

### Multi-tenancy

To enable multi-tenancy, run the script with the `--restricted-auth-permission` flag and pass the mandatory flags with it,
It will generate the secrets which you can use for creating new `Consumer cluster` deployment using the same `Source cluster`(ceph cluster).
So you would be running different isolated consumer clusters on top of single `Source cluster`.

!!! note
    Restricting the csi-users per pool, and per cluster will require creating new csi-users and new secrets for that csi-users.
    So apply these secrets only to new `Consumer cluster` deployment while using the same `Source cluster`.

```console
python3 create-external-cluster-resources.py --cephfs-filesystem-name <filesystem-name> --rbd-data-pool-name <pool_name> --k8s-cluster-name <k8s-cluster-name> --restricted-auth-permission true --format <bash> --rgw-endpoint <rgw_endpoint> --namespace <rook-ceph>
```

### RGW Multisite

Pass the `--rgw-realm-name`, `--rgw-zonegroup-name` and `--rgw-zone-name` flags to create the admin ops user in a master zone, zonegroup and realm.
See the [Multisite doc](https://docs.ceph.com/en/quincy/radosgw/multisite/#configuring-a-master-zone) for creating a zone, zonegroup and realm.

```console
python3 create-external-cluster-resources.py --rbd-data-pool-name <pool_name> --format bash --rgw-endpoint <rgw_endpoint> --rgw-realm-name <rgw_realm_name>> --rgw-zonegroup-name <rgw_zonegroup_name> --rgw-zone-name <rgw_zone_name>>
```

### Topology Based Provisioning

Enable Topology Based Provisioning for RBD pools by passing `--topology-pools`, `--topology-failure-domain-label` and `--topology-failure-domain-values` flags.
A new storageclass named `ceph-rbd-topology` will be created by the import script with `volumeBindingMode: WaitForFirstConsumer`.
The storageclass is used to create a volume in the pool matching the topology where a pod is scheduled.

For more details, see the [Topology-Based Provisioning](topology-for-external-mode.md)

### Upgrade Example

1. If consumer cluster doesn't have restricted caps, this will upgrade all the default csi-users (non-restricted):

    ```console
    python3 create-external-cluster-resources.py --upgrade
    ```

2. If the consumer cluster has restricted caps:
Restricted users created using `--restricted-auth-permission` flag need to pass mandatory flags: '`--rbd-data-pool-name`(if it is a rbd user), `--k8s-cluster-name` and `--run-as-user`' flags while upgrading, in case of cephfs users if you have passed `--cephfs-filesystem-name` flag while creating csi-users then while upgrading it will be mandatory too. In this example the user would be `client.csi-rbd-node-rookstorage-replicapool` (following the pattern `csi-user-clusterName-poolName`)

    ```console
    python3 create-external-cluster-resources.py --upgrade --rbd-data-pool-name replicapool --k8s-cluster-name rookstorage --run-as-user client.csi-rbd-node-rookstorage-replicapool
    ```

!!! note
    An existing non-restricted user cannot be converted to a restricted user by upgrading.
    The upgrade flag should only be used to append new permissions to users. It shouldn't be used for changing a csi user already applied permissions. For example, you shouldn't change the pool(s) a user has access to.

### Admin privileges

If in case the cluster needs the admin keyring to configure, update the admin key `rook-ceph-mon` secret with client.admin keyring

!!! note
    Sharing the admin key with the external cluster is not generally recommended

1. Get the `client.admin` keyring from the ceph cluster

    ```console
    ceph auth get client.admin
    ```

2. Update two values in the `rook-ceph-mon` secret:
    - `ceph-username`: Set to `client.admin`
    - `ceph-secret`: Set the client.admin keyring

After restarting the rook operator (and the toolbox if in use), rook will configure ceph with admin privileges.

### 2. Copy the bash output

Example Output:

```console
export ROOK_EXTERNAL_FSID=797f411a-aafe-11ec-a254-fa163e1539f5
export ROOK_EXTERNAL_USERNAME=client.healthchecker
export ROOK_EXTERNAL_CEPH_MON_DATA=ceph-rados-upstream-w4pdvq-node1-installer=10.0.210.83:6789
export ROOK_EXTERNAL_USER_SECRET=AQAdm0FilZDSJxAAMucfuu/j0ZYYP4Bia8Us+w==
export ROOK_EXTERNAL_DASHBOARD_LINK=https://10.0.210.83:8443/
export CSI_RBD_NODE_SECRET=AQC1iDxip45JDRAAVahaBhKz1z0WW98+ACLqMQ==
export CSI_RBD_PROVISIONER_SECRET=AQC1iDxiMM+LLhAA0PucjNZI8sG9Eh+pcvnWhQ==
export MONITORING_ENDPOINT=10.0.210.83
export MONITORING_ENDPOINT_PORT=9283
export RBD_POOL_NAME=replicated_2g
export RGW_POOL_PREFIX=default
```

## Commands on the K8s consumer cluster

### Helm Installation

To install with Helm, the rook cluster helm chart will configure the necessary resources for the external cluster with the example `values-external.yaml`.

```console
clusterNamespace=rook-ceph
operatorNamespace=rook-ceph
cd deploy/examples/charts/rook-ceph-cluster
helm repo add rook-release https://charts.rook.io/release
helm install --create-namespace --namespace $clusterNamespace rook-ceph rook-release/rook-ceph -f values.yaml
helm install --create-namespace --namespace $clusterNamespace rook-ceph-cluster \
--set operatorNamespace=$operatorNamespace rook-release/rook-ceph-cluster -f values-external.yaml
```

Skip the manifest installation section and continue with [Cluster Verification](#cluster-verification).

### Manifest Installation

If not installing with Helm, here are the steps to install with manifests.

1. Deploy Rook, create [common.yaml](https://github.com/rook/rook/blob/master/deploy/examples/common.yaml), [crds.yaml](https://github.com/rook/rook/blob/master/deploy/examples/crds.yaml) and [operator.yaml](https://github.com/rook/rook/blob/master/deploy/examples/operator.yaml) manifests.

2. Create [common-external.yaml](https://github.com/rook/rook/blob/master/deploy/examples/external/common-external.yaml) and [cluster-external.yaml](https://github.com/rook/rook/blob/master/deploy/examples/external/cluster-external.yaml)

### Import the Source Data

1. Paste the above output from `create-external-cluster-resources.py` into your current shell to allow importing the source data.

2. The import script in the next step uses the current kubeconfig context by
    default. If you want to specify the kubernetes cluster to use without
    changing the current context, you can specify the cluster name by setting
    the KUBECONTEXT environment variable.

    ```console
    export KUBECONTEXT=<cluster-name>
    ```

3. Here is the link for [import](https://github.com/rook/rook/blob/master/deploy/examples/external/import-external-cluster.sh) script. The script has used the `rook-ceph` namespace and few parameters that also have referenced from namespace variable. If user's external cluster has a different namespace, change the namespace parameter in the script according to their external cluster. For example with `new-namespace` namespace, this change is needed on the namespace parameter in the script.

    ```console
    NAMESPACE=${NAMESPACE:="new-namespace"}
    ```

4. Run the import script.

    !!! note
        If your Rook cluster nodes are running a kernel earlier than or equivalent to 5.4, remove
        `fast-diff, object-map, deep-flatten,exclusive-lock` from the `imageFeatures` line.

    ```console
    . import-external-cluster.sh
    ```

### Cluster Verification

1. Verify the consumer cluster is connected to the source ceph cluster:

    ```console
    $ kubectl -n rook-ceph  get CephCluster
    NAME                 DATADIRHOSTPATH   MONCOUNT   AGE    STATE       HEALTH
    rook-ceph-external   /var/lib/rook                162m   Connected   HEALTH_OK
    ```

2. Verify the creation of the storage class depending on the rbd pools and filesystem provided.
    `ceph-rbd` and `cephfs` would be the respective names for the RBD and CephFS storage classes.

    ```console
    kubectl -n rook-ceph get sc
    ```

3. Then you can now create a [persistent volume](https://github.com/rook/rook/tree/master/deploy/examples/csi) based on these StorageClass.

### Connect to an External Object Store

Create the [external object store CR](https://github.com/rook/rook/blob/master/deploy/examples/external/object-external.yaml) to configure connection to external gateways.

```console
cd deploy/examples/external
kubectl create -f object-external.yaml
```

Consume the S3 Storage, in two different ways:

1. Create an [Object store user](https://github.com/rook/rook/blob/master/deploy/examples/object-user.yaml) for credentials to access the S3 endpoint.

    ```console
    cd deploy/examples
    kubectl create -f object-user.yaml
    ```

2. Create a [bucket storage class](https://github.com/rook/rook/blob/master/deploy/examples/external/storageclass-bucket-delete.yaml) where a client can request creating buckets and then create the [Object Bucket Claim](https://github.com/rook/rook/blob/master/deploy/examples/external/object-bucket-claim-delete.yaml), which will create an individual bucket for reading and writing objects.

    ```console
    cd deploy/examples/external
    kubectl create -f storageclass-bucket-delete.yaml
    kubectl create -f object-bucket-claim-delete.yaml
    ```

!!! hint
    For more details see the [Object Store topic](../../../Storage-Configuration/Object-Storage-RGW/object-storage.md#connect-to-an-external-object-store)

### Connect to v2 mon port

If encryption or compression on the wire is needed, specify the `--v2-port-enable` flag.
If the v2 address type is present in the `ceph quorum_status`, then the output of 'ceph mon data' i.e, `ROOK_EXTERNAL_CEPH_MON_DATA` will use the v2 port(`3300`).

### NFS storage

Rook suggests a different mechanism for making use of an [NFS service running on the external Ceph standalone cluster](../../../Storage-Configuration/NFS/nfs-csi-driver.md#consuming-nfs-from-an-external-source), if desired.

## Exporting Rook to another cluster

If you have multiple K8s clusters running, and want to use the local `rook-ceph` cluster as the central storage,
you can export the settings from this cluster with the following steps.

1. Copy create-external-cluster-resources.py into the directory `/etc/ceph/` of the toolbox.

    ```console
    toolbox=$(kubectl get pod -l app=rook-ceph-tools -n rook-ceph -o jsonpath='{.items[*].metadata.name}')
    kubectl -n rook-ceph cp deploy/examples/external/create-external-cluster-resources.py $toolbox:/etc/ceph
    ```

2. Exec to the toolbox pod and execute create-external-cluster-resources.py with needed options to create required [users and keys](#1-create-all-users-and-keys).

!!! important
    For other clusters to connect to storage in this cluster, Rook must be configured with a networking configuration that is accessible from other clusters. Most commonly this is done by enabling host networking in the CephCluster CR so the Ceph daemons will be addressable by their host IPs.
