# Creating Rook Clusters
Rook allows creation and customization of storage clusters through the third party resources (TPRs). The following settings are available
for a cluster.

## Sample
```
apiVersion: rook.io/v1beta1
kind: Rookcluster
metadata:
  name: rook
spec:
  namespace: rook
  version: latest
  hostPath: /var/lib/rook
  useAllDevices: false
  deviceFilter: ^sd.
```

## Settings
Settings can be specified at the global level to apply to the cluster as a whole, while other settings can be specified at more fine-grained levels.

### Global settings
- `hostPath`: The host path where config and data should be stored for each of the services. If the directory does not exist, it will be created. In test scenarios, the path must be deleted if you are going to delete a cluster and start a new cluster on the same hosts.
- `namespace`: The Kubernetes namespace that will be created for the Rook cluster. The services, pods, and other resources created by the operator will be added to this namespace. Each cluster must have a unique namespace. The common scenario is to create a single Rook cluster. If multiple clusters are created, they must not have conflicting devices or host paths.
- `version`: The version of the `quay.io/rook/rookd` container that will be deployed. Upgrades are not yet supported if this setting is updated for an existing cluster, but upgrades will be coming.
- `useAllDevices`: `true` or `false`, indicating whether all devices found on nodes in the cluster should be automatically consumed by OSDs. **Not recommended** unless you have a very controlled environment where you will not risk formatting of devices with existing data. When `true`, all devices will be used except those with partitions created or a local filesystem. Is overridden by `deviceFilter` if specified.
- `deviceFilter`: A regular expression that allows selection of devices to be consumed by OSDs. If not specified, devices will not be selected. This directly uses the golang regular expression. See the [syntax](https://golang.org/pkg/regexp/syntax/). For example:
   - `sdb`: Only selects the `sdb` device if found
   - `^sd.`: Selects all devices starting with `sd`
   - `^sd[a-d]`: Selects devices starting with `sda`, `sdb`, `sdc`, and `sdd` if found
   - `^s`: Selects all devices that start with `s`
   - `^[^r]`: Selects all devices that do *not* start with `r`

### Node settings
Coming soon we will have more granular settings for nodes, devices, topology, etc.