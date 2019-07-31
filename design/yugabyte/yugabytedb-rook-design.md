YugaByte DB is a high-performance distributed SQL database. (More information [here](https://docs.yugabyte.com/latest/introduction/)). This document outlines design and interfaces for a Rook YugaByte operator for running YugaByte on Kubernetes with Rook as the underlying storage engine.

**What is use case behind this feature:**

A cluster of YugaByte DB backed by Rook should be created using the `ybcluster.yugabytedb.rook.io` custom resource definition. Below is a sample custom resource spec for creating a 3-master, 3 t-servers cluster using the CRD. The sample is followed by an explanation of different configuration options available on the YugaByte DB CRD.

## Sample

```yaml
apiVersion: yugabytedb.rook.io/v1alpha1
kind: YBCluster
metadata:
  name: rook-yugabytedb
  namespace: rook-yugabytedb
spec:
  master:
    # Replica count for Master.
    replicas: 3
    # Mentioning network ports is optional. If some or all ports are not specified, then they will be defaulted to below-mentioned values, except for tserver-ui.
    network:
      ports:
        - name: yb-master-ui
          port: 7000          # default value
        - name: yb-master-rpc
          port: 7100          # default value
    # Volume claim template for Master
    volumeClaimTemplate:
      metadata:
        name: datadir
      spec:
        accessModes: [ "ReadWriteOnce" ]
        resources:
          requests:
            storage: 1Gi
        storageClassName: standard
  tserver:
    # Replica count for TServer
    replicas: 3
    # Mentioning network ports is optional. If some or all ports are not specified, then they will be defaulted to below-mentioned values, except for tserver-ui.
    # For tserver-ui a cluster ip service will be created if the yb-tserver-ui port is explicitly mentioned. If it is not specified, only StatefulSet & headless service will be created for TServer. TServer ClusterIP service creation will be skipped. Whereas for Master, all 3 kubernetes objects will always be created.
    network:
      ports:
        - name: yb-tserver-ui
          port: 9000
        - name: yb-tserver-rpc
          port: 9100          # default value
        - name: ycql
          port: 9042          # default value
        - name: yedis
          port: 6379          # default value
        - name: ysql
          port: 5433          # default value
    # Volume claim template for TServer
    volumeClaimTemplate:
      metadata:
        name: datadir
      spec:
        accessModes: [ "ReadWriteOnce" ]
        resources:
          requests:
            storage: 1Gi
        storageClassName: standard
```
# Cluster Settings

### Master/TServer
Master & TServer are two essential components of a YugabyteDB cluster. Master is responsible for recording and maintaining system metadata & for admin activities. TServers are mainly responsible for data I/O.
Specify Master/TServer specific attributes under `master`/`tserver`. The valid attributes are `replicas`, `network` & `volumeClaimTemplate`.

### Replica Count
Specify replica count for `master` & `tserver` pods under `replicas` field. This is a **required** field.

### Network
`network` field accepts `NetworkSpec` to be specified which describes YugabyteDB network settings. This is an **optional** field. Default network settings will be used, if any or all of the acceptable values are absent.

A ClusterIP service will be created when `yb-tserver-ui` port is explicitly specified. If it is not specified, only StatefulSet & headless service will be created for TServer. ClusterIP service creation will be skipped. Whereas for Master, all 3 kubernetes objects will always be created.

The acceptable port names & their default values are as follows:

| Name | Default Value |
| ---- | ------------- |
| yb-master-ui | 7000 |
| yb-master-rpc | 7100 |
| yb-tserver-rpc | 9100 |
| ycql | 9042 |
| yedis | 6379 |
| ysql | 5433 |

### Volume Claim Templates
Specify a `PersistentVolumeClaim` template under the `volumeClaimTemplate` field for `master` & `tserver` each. This is a **required** field.


# Expected result
Above sample will result in creation on a 3 Master 3 TServer YugabyteDB cluster. The YugabyteDB cluster is backed by following Kubernetes objects, which will be managed by the Rook-YugabyteDB operator.

|Object|Count|
|------|-----|
|Master UI Service| 1 |
|TServer UI Service| 1 |
|Master Headless Service| 1 |
|TServer Headless Service| 1 |
|Master StatefulSet| 1 |
|TServer StatefulSet| 1 |
|Master Pods| 3 |
|TServer Pods| 3 |