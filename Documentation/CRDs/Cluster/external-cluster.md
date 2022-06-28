---
title: External Storage Cluster
---

An external cluster is a Ceph configuration that is managed outside of the local K8s cluster. The external cluster could be managed by cephadm, or it could be another Rook cluster that is configured to allow the access (usually configured with host networking).

In external mode, Rook will provide the configuration for the CSI driver and other basic resources that allows your applications to connect to Ceph in the external cluster.

## Supported Features

The features available from the external cluster will vary depending on the version of Ceph. The following table shows the minimum version of Ceph for some of the features:

| FEATURE                                      | CEPH VERSION |
| -------------------------------------------- | ------------ |
| Dynamic provisioning RBD                     | 12.2.X       |
| Configure extra CRDs (object, file, nfs)[^1] | 13.2.3       |
| Dynamic provisioning CephFS                  | 14.2.3       |

[^1]: Configure an object store, shared filesystem, or NFS resources in the local cluster to connect to the external Ceph cluster

## External configuration

* Source cluster: The cluster providing the data, usually configured by [cephadm](https://docs.ceph.com/en/pacific/cephadm/#cephadm)

* Consumer cluster: The K8s cluster that will be consuming the external source cluster

## Commands on the source Ceph cluster

In order to configure an external Ceph cluster with Rook, we need to extract some information in order to connect to that cluster.

1. Run the python script [create-external-cluster-resources.py](/deploy/examples/create-external-cluster-resources.py) for creating all users and keys.

   ```console
   python3 create-external-cluster-resources.py --rbd-data-pool-name <pool_name> --cephfs-filesystem-name <filesystem-name> --rgw-endpoint  <rgw-endpoint> --namespace <namespace> --format bash
   ```

- `--namespace`: Namespace where CephCluster will run, for example `rook-ceph-external`
- `--format bash`: The format of the output
- `--rbd-data-pool-name`: The name of the RBD data pool
- `--rgw-endpoint`: (optional) The RADOS Gateway endpoint in the format `<IP>:<PORT>`
- `--rgw-pool-prefix`: (optional) The prefix of the RGW pools. If not specified, the default prefix is `default`
- `--rgw-tls-cert-path`: (optional) RADOS Gateway endpoint TLS certificate file path
- `--rgw-skip-tls`: (optional) Ignore TLS certification validation when a self-signed certificate is provided (NOT RECOMMENDED)
- `--rbd-metadata-ec-pool-name`: (optional) Provides the name of erasure coded RBD metadata pool, used for creating ECRBDStorageClass.
- `--monitoring-endpoint`: (optional) Ceph Manager prometheus exporter endpoints (comma separated list of <IP> entries of active and standby mgrs)
- `--monitoring-endpoint-port`: (optional) Ceph Manager prometheus exporter port
- `--ceph-conf`: (optional) Provide a Ceph conf file
- `--cluster-name`: (optional) Ceph cluster name
- `--output`: (optional) Output will be stored into the provided file
- `--dry-run`: (optional) Prints the executed commands without running them
- `--run-as-user`: (optional) Provides a user name to check the cluster's health status, must be prefixed by `client`.
- `--cephfs-metadata-pool-name`: (optional) Provides the name of the cephfs metadata pool
- `--cephfs-filesystem-name`: (optional) The name of the filesystem, used for creating CephFS StorageClass
- `--cephfs-data-pool-name`: (optional) Provides the name of the CephFS data pool, used for creating CephFS StorageClass
- `--rados-namespace`: (optional) Divides a pool into separate logical namespaces, used for creating RBD PVC in a RadosNamespaces
- `--subvolume-group`: (optional) Provides the name of the subvolume group, used for creating CephFS PVC in a subvolumeGroup
- `--restricted-auth-permission`: (optional) Restrict cephCSIKeyrings auth permissions to specific pools, and cluster. Mandatory flags that need to be set are `--rbd-data-pool-name`, and `--cluster-name`. `--cephfs-filesystem-name` flag can also be passed in case of CephFS user restriction, so it can restrict users to particular CephFS filesystem.

!!! note
    Restricting the csi-users per pool, and per cluster will require creating new csi-users and new secrets for that csi-users, so it can only be used for new deployments.

    Sample run:
    ```console
    python3 create-external-cluster-resources.py --cephfs-filesystem-name <filesystem-name> --rbd-data-pool-name <pool_name> --cluster-name <cluster-name> --restricted-auth-permission true --format <bash> --rgw-endpoint <rgw_endpoin> --namespace <rook-ceph-external>
    ```

- `--upgrade`: (optional) Upgrades the 'Ceph CSI keyrings (For example: client.csi-cephfs-provisioner) with new permissions needed for the new cluster version and older permission will still be applied.

    Sample run:
    ```console
    python3 create-external-cluster-resources.py --upgrade
    ```
    This will upgrade all the default csi-users(non-restricted)

    Restricted users created using --restricted-auth-permission flag (For example: client.csi-rbd-node-rookStorage-replicapool) need to pass mandatory flags: '--rbd-data-pool-name(if it is a rbd user), --cluster-name and --run-as-user' flags while upgrading, in case of cephfs users if you have passed --cephfs-filesystem-name flag while creating csi-users then while upgrading it will be mandatory too

    Sample run:
    ```console
    python3 create-external-cluster-resources.py --upgrade --rbd-data-pool-name replicapool --cluster-name rookstorage --run-as-user client.csi-rbd-node-rookStorage-replicapool
    ```

!!! note
    An existing non-restricted user cannot be downgraded to a restricted user by upgrading. Admin needs to create a new restricted user for this by re-running the script.The upgrade flag should only be used to append new permissions to users. It shouldn't be used for changing a csi user already applied permissions. For example, you shouldn't change the pool(s) a user has access to.

2. Copy the bash output.

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

1. Deploy Rook, create [common.yaml](/deploy/examples/common.yaml), [crds.yaml](/deploy/examples/crds.yaml) and [operator.yaml](/deploy/examples/operator.yaml) manifests.

2. Create [common-external.yaml](/deploy/examples/common-external.yaml) and [cluster-external.yaml](/deploy/examples/cluster-external.yaml)

3. Paste the above output from `create-external-cluster-resources.py` into your current shell to allow importing the source data.

4. Run the [import](/deploy/examples/import-external-cluster.sh) script.

    ```console
    . import-external-cluster.sh
    ```

5. Verify the consumer cluster is connected to the source ceph cluster:

    ```console
    $ kubectl -n rook-ceph-external  get CephCluster
    NAME                 DATADIRHOSTPATH   MONCOUNT   AGE    STATE       HEALTH
    rook-ceph-external   /var/lib/rook                162m   Connected   HEALTH_OK
    ```

6. StorageClass will also be created, verify its creation. `ceph-rbd` and `cephfs` StorageClass would be respective name for RBD and CephFS StorageClass.
    ```console
    kubectl -n rook-ceph-external get sc
    ```

7. Then you can now create a [persistent volume](/deploy/examples/csi) based on these StorageClass.

### CephCluster example (management)

The following CephCluster CR represents a cluster that will perform management tasks on the external cluster.
It will not only act as a consumer but will also allow the deployment of other CRDs such as CephFilesystem or CephObjectStore.
You would need to inject the admin keyring for that.

The corresponding YAML example:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph-external
  namespace: rook-ceph-external
spec:
  external:
    enable: true
  dataDirHostPath: /var/lib/rook
  cephVersion:
    image: quay.io/ceph/ceph:v17.2.1 # Should match external cluster version
```
