# External Cluster Upgrades

When upgrading an external cluster, Ceph and Rook versions will be updated independently. During the Rook update, the external provider cluster connection also needs to be updated with any settings and permissions for new features.

## Upgrade the cluster to consume latest ceph user caps (mandatory)

Upgrading the cluster would be different for restricted caps and non-restricted caps,

1. If consumer cluster doesn't have restricted caps, this will upgrade all the default CSI users (non-restricted)

    ```console
    python3 create-external-cluster-resources.py --upgrade
    ```

2. If the consumer cluster has restricted caps

    Restricted users created using `--restricted-auth-permission` flag need to pass mandatory flags: '`--rbd-data-pool-name`(if it is a rbd user), `--k8s-cluster-name` and `--run-as-user`' flags while upgrading, in case of cephfs users if you have passed `--cephfs-filesystem-name` flag while creating CSI users then while upgrading it will be mandatory too. In this example the user would be `client.csi-rbd-node-rookstorage-replicapool` (following the pattern `csi-user-clusterName-poolName`)
    If --cephx-key-rotate was set, it adds `.{x}` suffix to the user name, for example: `client.csi-rbd-node-rookstorage-replicapool.1`

    ```console
    python3 create-external-cluster-resources.py --upgrade --rbd-data-pool-name replicapool --k8s-cluster-name rookstorage --run-as-user client.csi-rbd-node-rookstorage-replicapool
    ```

    !!! note
        1) An existing non-restricted user cannot be converted to a restricted user by upgrading.
        2) The upgrade flag should only be used to append new permissions to users. It shouldn't be used for changing a CSI user already applied permissions. For example, be careful not to change pools(s) that a user has access to.

## Upgrade cluster to utilize a new feature (optional)

Some Rook upgrades may require re-running the import steps, or may introduce new external cluster features that can be most easily enabled by re-running the import steps.

To re-run the import steps with new options, the python script should be re-run using the same configuration options that were used for past invocations, plus the configurations that are being added or modified.

Starting with Rook v1.15, the script stores the configuration in the external-cluster-user-command configmap for easy future reference.

* arg: Exact arguments that were used for for processing the script.
Argument that are decided using the Priority: command-line-args has more priority than config.ini file values, and config.ini file values have more priority than default values.

### Example `external-cluster-user-command` ConfigMap:

1. Get the last-applied config, if its available

    ```console
    $ kubectl get configmap -namespace rook-ceph external-cluster-user-command --output jsonpath='{.data.args}'
    ```

2. Copy the output to config.ini

3. Make any desired modifications and additions to `config.ini``

4. Run the python script again using the [config file](provider-export.md#config-file)

5. [Copy the bash output](provider-export.md#2-copy-the-bash-output)

6. [Import-the-provider-data](consumer-import.md#import-the-provider-data)

!!! warning
    If the last-applied config is unavailable, run the current version of the script again using previously-applied config and CLI flags.
    Failure to reuse the same configuration options when re-invoking the python script can result in unexpected changes when re-running the import script.
