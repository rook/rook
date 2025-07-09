# Export config from the Ceph provider cluster

In order to configure an external Ceph cluster with Rook, we need to extract some information in order to connect to that cluster.

## 1. Create all users and keys

Run the python script [create-external-cluster-resources.py](https://github.com/rook/rook/blob/master/deploy/examples/external/create-external-cluster-resources.py) in the provider Ceph cluster cephadm shell, to have access to create the necessary users and keys.

```console
python3 create-external-cluster-resources.py --rbd-data-pool-name <pool_name> --cephfs-filesystem-name <filesystem-name> --rgw-endpoint  <rgw-endpoint> --namespace <namespace> --format bash
```

* `--namespace`: Namespace where CephCluster will run, for example `rook-ceph`
* `--format bash`: The format of the output
* `--rbd-data-pool-name`: The name of the RBD data pool
* `--alias-rbd-data-pool-name`: Provides an alias for the  RBD data pool name, necessary if a special character is present in the pool name such as a period or underscore
* `--rgw-endpoint`: (optional) The RADOS Gateway endpoint in the format `<IP>:<PORT>` or `<FQDN>:<PORT>`.
* `--rgw-pool-prefix`: (optional) The prefix of the RGW pools. If not specified, the default prefix is `default`
* `--rgw-tls-cert-path`: (optional) RADOS Gateway endpoint TLS certificate (or intermediate signing certificate) file path
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
* `--config-file`: Path to the configuration file, Priority: command-line-args > config.ini values > default values
* `--cephx-key-rotate`: Enable cephx key rotation for the users created by this script. This will create a new user with suffix `.{x}` and update the secrets with the new key. Set `--cephx-key-rotation rotate` to initiate rotation. To revert keys to the prior generation, set `--cephx-key-rotation revert`
Note: If user are reverting to prior generation, then the user should manually delete the prior used user keys from the cluster after reverting.

## 2. Copy the bash output

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

## Examples on utilizing Advance flags

### Config-file

Use the config file to set the user configuration file, add the flag `--config-file` to set the file path.

Example:

`/config.ini`

```console
[Configurations]
format = bash
cephfs-filesystem-name = <filesystem-name>
rbd-data-pool-name = <pool_name>
...
```

```console
python3 create-external-cluster-resources.py --config-file /config.ini
```

!!! note
    You can use both config file and other arguments at the same time
    Priority: command-line-args has more priority than config.ini file values, and config.ini file values have more priority than default values.

### Multi-tenancy

To enable multi-tenancy, run the script with the `--restricted-auth-permission` flag and pass the mandatory flags with it,
It will generate the secrets which you can use for creating new `Consumer cluster` deployment using the same `Provider cluster`(ceph cluster).
So you would be running different isolated consumer clusters on top of single `Provider cluster`.

!!! note
    Restricting the csi-users per pool, and per cluster will require creating new csi-users and new secrets for that csi-users.
    So apply these secrets only to new `Consumer cluster` deployment while using the same `Provider cluster`.

```console
python3 create-external-cluster-resources.py --cephfs-filesystem-name <filesystem-name> --rbd-data-pool-name <pool_name> --k8s-cluster-name <k8s-cluster-name> --restricted-auth-permission true --format <bash> --rgw-endpoint <rgw_endpoint> --namespace <rook-ceph>
```

### RGW Multisite

Pass the `--rgw-realm-name`, `--rgw-zonegroup-name` and `--rgw-zone-name` flags to create the admin ops user in a master zone, zonegroup and realm.
See the [Multisite doc](https://docs.ceph.com/en/latest/radosgw/multisite/#configuring-a-master-zone) for creating a zone, zonegroup and realm.

```console
python3 create-external-cluster-resources.py --rbd-data-pool-name <pool_name> --format bash --rgw-endpoint <rgw_endpoint> --rgw-realm-name <rgw_realm_name>> --rgw-zonegroup-name <rgw_zonegroup_name> --rgw-zone-name <rgw_zone_name>>
```

### Topology Based Provisioning

Enable Topology Based Provisioning for RBD pools by passing `--topology-pools`, `--topology-failure-domain-label` and `--topology-failure-domain-values` flags.
A new storageclass named `ceph-rbd-topology` will be created by the import script with `volumeBindingMode: WaitForFirstConsumer`.
The storageclass is used to create a volume in the pool matching the topology where a pod is scheduled.

For more details, see the [Topology-Based Provisioning](topology-for-external-mode.md)

### Connect to v2 mon port

If encryption or compression on the wire is needed, specify the `--v2-port-enable` flag.
If the v2 address type is present in the `ceph quorum_status`, then the output of 'ceph mon data' i.e, `ROOK_EXTERNAL_CEPH_MON_DATA` will use the v2 port(`3300`).
