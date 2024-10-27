# External Cluster Options

## NFS storage

Rook suggests a different mechanism for making use of an [NFS service running on the external Ceph standalone cluster](../../../Storage-Configuration/NFS/nfs-csi-driver.md#consuming-nfs-from-an-external-source), if desired.

## Exporting Rook to another cluster

If you have multiple K8s clusters running, and want to use the local `rook-ceph` cluster as the central storage,
you can export the settings from this cluster with the following steps.

1. Copy create-external-cluster-resources.py into the directory `/etc/ceph/` of the toolbox.

    ```console
    toolbox=$(kubectl get pod -l app=rook-ceph-tools -n rook-ceph -o jsonpath='{.items[*].metadata.name}')
    kubectl -n rook-ceph cp deploy/examples/external/create-external-cluster-resources.py $toolbox:/etc/ceph
    ```

2. Exec to the toolbox pod and execute create-external-cluster-resources.py with needed options to create required [users and keys](/Documentation/CRDs/Cluster/external-cluster/provider-export.md#1-create-all-users-and-keys).

!!! important
    For other clusters to connect to storage in this cluster, Rook must be configured with a networking configuration that is accessible from other clusters. Most commonly this is done by enabling host networking in the CephCluster CR so the Ceph daemons will be addressable by their host IPs.

## Admin privileges

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

## Connect to an External Object Store

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
