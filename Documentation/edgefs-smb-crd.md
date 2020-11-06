---
title: Scale-Out SMB CRD
weight: 4350
indent: true
---

# EdgeFS Scale-Out SMB CRD

[Deprecated](https://github.com/rook/rook/issues/5823#issuecomment-703834989)

Rook allows creation and customization of EdgeFS SMB/CIFS filesystems through the custom resource definitions (CRDs).
The following settings are available for customization of EdgeFS SMB services.

## Sample

```yaml
apiVersion: edgefs.rook.io/v1
kind: SMB
metadata:
  name: smb01
  namespace: rook-edgefs
spec:
  instances: 3
  #ads:
  #  domainName: "corp.example.com"
  #  dcName: "localdc"
  #  serverName: "edgefs-smb"
  #  userSecret: "corp.example.com"
  #  nameservers: "10.200.1.19"
  #relaxedDirUpdates: true
  #chunkCacheSize: 1Gi
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
  #          - smb-node
  #  tolerations:
  #  - key: smb-node
  #    operator: Exists
  #  podAffinity:
  #  podAntiAffinity:
  #resourceProfile: embedded
  resources:
  #  limits:
  #    cpu: "500m"
  #    memory: "1024Mi"
  #  requests:
  #    cpu: "500m"
  #    memory: "1024Mi"
```

## Metadata

* `name`: The name of the SMB system to create, which must match existing EdgeFS service.
* `namespace`: The namespace of the Rook cluster where the SMB service is created.
* `instances`: The number of active SMB service instances. EdgeFS SMB service is Multi-Head capable, such so that multiple PODs can mount same tenant's buckets via different endpoints.
* `relaxedDirUpdates`: If set to `true` then it will significantly improve performance of directory operations by deferring updates, guaranteeing eventual directory consistency. This option is recommended when a bucket exported via single SMB instance and it is not a destination for ISGW Link synchronization.
* `chunkCacheSize`: Limit amount of memory allocated for dynamic chunk cache. By default SMB pod uses up to 75% of available memory as chunk caching area. This option can influence this allocation strategy.
* `annotations`: Key value pair list of annotations to add.
* `placement`: The SMB PODs can be given standard Kubernetes placement restrictions with `nodeAffinity`, `tolerations`, `podAffinity`, and `podAntiAffinity` similar to placement defined for daemons configured by the [cluster CRD](/cluster/examples/kubernetes/edgefs/cluster.yaml).
* `resourceProfile`: SMB pod resource utilization profile (Memory and CPU). Can be `embedded` or `performance` (default). In case of `performance` an SMB pod trying to increase amount of internal I/O resources that results in higher performance at the cost of additional memory allocation and more CPU load. In `embedded` profile case, SMB pod gives preference to preserving memory over I/O and limiting chunk cache (see `chunkCacheSize` option). The `performance` profile is the default unless cluster wide `embedded` option is defined.
* `resources`: Set resource requests/limits for the SMB Pod(s), see [Resource Requirements/Limits](edgefs-cluster-crd.md#resource-requirementslimits).
* `ads`: Set Active Directory service join parameters. If not defined, SMB gateway will start in WORKGROUP mode.

## Joining Windows Active Directory service

Before you begin, please make sure that external IP can be properly provisioned to passthrough ports 445 (smb) and 139 (netbios), pointing to SMB service.
If for some reason external IP is difficult or impossible to provision, you can use NodePort and setup redirect rules on the node where SMB gateway will be running.

Define "ads" metadata section with the following parameters and reference a secret object:

* `domainName`: AD Domain Name in form of DNS record, like `corp.example.com`.
* `dcName`: Preferred Domain Controller Name. Could be short name like `localdc`.
* `serverName`: NetBIOS Name of our SMB Gateway. This name will be used to during SMB share mapping, e.g. `\\edgefs-smb\bk1`.
* `userSecret`: The name of secret holding username and password keys. Secret object has to be pre-created in the same namespace.
* `nameservers`: The comma separated list of DNS name server IPs, e.g. `10.3.40.16,10.3.40.17`

Secret object can look like this:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: corp.example.com
  namespace: rook-edgefs
type: Opaque
stringData:
  username: "Administrator"
  password: "Password!"
```

## Setting up EdgeFS namespace and tenant

For more detailed instructions please refer to [EdgeFS Wiki](https://github.com/Nexenta/edgefs/wiki).

Below is an exampmle procedure to get things initialized and configured.

Before new local namespace (or local site) can be used, it has to be initialized with FlexHash and special purpose root object.

FlexHash consists of dynamically discovered configuration and checkpoint of accepted distribution table. FlexHash is responsible for I/O direction and plays important role in dynamic load balancing logic. It defines so-called Negotiating Groups (typically across zoned 8-24 disks) and final table distribution across all the participating components, e.g. data nodes, service gateways and tools.

Root object holds system information and table of namespaces registered to a local site. Root object is always local and never shared between the sites.

To initialize system and prepare logical definitions, login to the toolbox as shown in this example:

```console
kubectl get po --all-namespaces | grep edgefs-mgr
kubectl exec -it -n rook-edgefs rook-edgefs-mgr-6cb9598469-czr7p -- env COLUMNS=$COLUMNS LINES=$LINES TERM=linux toolbox
```

Assumption at this point is that nodes are all configured and can be seen via the following command:

```console
efscli system status
```

1. Initialize EdgeFS cluster:

Verify that HW (or better say emulated in this case) configuration look normal and accept it:

```console
efscli system init
```

At this point new dynamically discovered configuration checkpoint will be created at $NEDGE_HOME/var/run/flexhash-checkpoint.json
This will also create system "root" object, holding Site's Namespace. Namespace may consist of more then single region.

2. Create new local namespace (or we also call it "Region" or "Segment"):

```console
efscli cluster create Hawaii
```

3. Create logical tenants of cluster namespace "Hawaii", also buckets if needed:

```console
efscli tenant create Hawaii/Cola
efscli bucket create Hawaii/Cola/bk1
efscli tenant create Hawaii/Pepsi
efscli bucket create Hawaii/Pepsi/bk1
```

Now cluster is setup, services can be now created and attached to CSI provisioner.

4. Create SMB service objects for tenants:

```console
efscli service create smb smb-cola
efscli service serve smb-cola Hawaii/Cola/bk1
efscli service config smb-cola X-SMB-OPTS-Cola-bk1 "force user = root;public = yes;directory mask = 777;create mask = 666"
efscli service create smb smb-pepsi
efscli service serve smb-pepsi Hawaii/Pepsi/bk1
efscli service config smb-pepsi X-SMB-OPTS-Pepsi-bk1 "force user = root;public = yes;directory mask = 777;create mask = 666"
```

Also, notice that we setting password-less configuration for bk1 share.

5. Create your EdgeFS SMB objects:

```yaml
apiVersion: edgefs.rook.io/v1
kind: SMB
metadata:
  name: smb-cola
  namespace: rook-edgefs
spec:
  instances: 1
```

```yaml
apiVersion: edgefs.rook.io/v1
kind: SMB
metadata:
  name: smb-pepsi
  namespace: rook-edgefs
spec:
  instances: 1
```

At this point two SMB services should be available. Install cifs-utils package and verify that you can mount it (substitute CLUSTERIP with corresponding entry from `kubectl get svc` command):

```console
kubectl get -o rook-edgefs svc | grep smb-pepsi
mount -t cifs -o vers=3.0,sec=none //CLUSTERIP/bk1 /tmp/bk1
```
