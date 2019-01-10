---
title: EdgeFS iSCSI Target CRD
weight: 46
indent: true
---

# EdgeFS iSCSI Target CRD

Rook allows creation and customization of High Performance iSCSI Target compatible services through the custom resource definitions (CRDs).
The following settings are available for customization of iSCSI Target services.

## Sample

```yaml
apiVersion: edgefs.rook.io/v1alpha1
kind: ISCSI
metadata:
  name: iscsi01
  namespace: rook-edgefs
spec:
  placement:
  #  nodeAffinity:
  #    requiredDuringSchedulingIgnoredDuringExecution:
  #      nodeSelectorTerms:
  #      - matchExpressions:
  #        - key: role
  #          operator: In
  #          values:
  #          - iscsi-node
  #  tolerations:
  #  - key: iscsi-node
  #    operator: Exists
  #  podAffinity:
  #  podAntiAffinity:
  resources:
  #  limits:
  #    cpu: "500m"
  #    memory: "1024Mi"
  #  requests:
  #    cpu: "500m"
  #    memory: "1024Mi"
```

### Metadata

- `name`: The name of the iSCSI target service to create, which must match existing EdgeFS service.
- `namespace`: The namespace of the Rook cluster where the iSCSI Target service is created.
- `targetName`: The name for iSCSI target name. Default is iqn.2018-11.edgefs.io.
- `targetParams`: If specified, then some of iSCSI target protocol parameters can be overriden.
  - `MaxRecvDataSegmentLength`: Value in range value range 512..16777215. Default is 524288.
  - `DefaultTime2Retain`: Value in range 0..3600. Default is 60.
  - `DefaultTime2Wait`: Value in range 0..3600. Default is 30.
  - `FirstBurstLength`: Value in range 512..16777215. Default is 524288.
  - `MaxBurstLength`: Value in range 512..16777215. Default is 1048576.
  - `MaxQueueCmd`: Value in range 1..128. Default is 64.
- `placement`: The iSCSI pods can be given standard Kubernetes placement restrictions with `nodeAffinity`, `tolerations`, `podAffinity`, and `podAntiAffinity` similar to placement defined for daemons configured by the [cluster CRD](/cluster/examples/kubernetes/edgefs/cluster.yaml).
- `resources`: Set resource requests/limits for the iSCSI pods, see [Resource Requirements/Limits](edgefs-cluster-crd.md#resource-requirementslimits).

### Setting up EdgeFS namespace and tenant

For more detailed instructions please refer to [EdgeFS Wiki](https://github.com/Nexenta/edgefs/wiki).

Simple procedure to get things initialized and configured:

## Setting up FlexHash and Site root object

Before new local namespace (or local site) can be used, it has to be initialized with FlexHash and special purpose root object.

FlexHash consists of dynamically discovered configuration and checkpoint of accepted distribution table. FlexHash is responsible for I/O direction and plays important role in dynamic load balancing logic. It defines so-called Negotiating Groups (typically across zoned 8-24 disks) and final table distribution across all the participating components, e.g. data nodes, service gateways and tools.

Root object holds system information and table of namespaces registered to a local site. Root object is always local and never shared between the sites.

To initialize system and prepare logical definitions, login to the toolbox as shown in this example:

<pre>
kubectl get po --all-namespaces | grep edgefs-mgr
kubectl exec -it -n rook-edgefs rook-edgefs-mgr-6cb9598469-czr7p -- env COLUMNS=$COLUMNS LINES=$LINES TERM=linux toolbox
</pre>

Assumption at this point is that nodes are all configured and can be seen via the following command:

<pre>
efscli system status
</pre>

1. Initialize cluster

Verify that HW (or better say emulated in this case) configuration look normal and accept it

<pre>
efscli system init
</pre>

At this point new dynamically discovered configuration checkpoint will be created at $NEDGE_HOME/var/run/flexhash-checkpoint.json
This will also create system "root" object, holding Site's Namespace. Namespace may consist of more then single region.

2. Create new local namespace (or we also call it "Region" or "Segment")

<pre>
efscli cluster create Hawaii
</pre>

3. Create logical tenants of cluster namespace "Hawaii", also buckets if needed

<pre>
efscli tenant create Hawaii/Cola
efscli bucket create Hawaii/Cola/bk1
efscli tenant create Hawaii/Pepsi
efscli bucket create Hawaii/Pepsi/bk1
</pre>

Now cluster is setup, services can be now created.

4. Create iSCSI Target services objects for tenants

<pre>
efscli service create iscsi iscCola
efscli service serve iscCola Hawaii/Cola/bk1/lun1 X-volsize=10G,ccow-chunkmap-chunk-size=16384
efscli service serve iscCola Hawaii/Cola/bk1/lun2 X-volsize=20G,ccow-chunkmap-chunk-size=131072
efscli service create iscsi iscPepsi
efscli service serve iscPepsi Hawaii/Pepsi/bk1/lun1 X-volsize=20G
</pre>

5. Create ISCSI CRDs

```yaml
apiVersion: edgefs.rook.io/v1alpha1
kind: ISCSI
metadata:
  name: iscCola
  namespace: rook-edgefs
```

```yaml
apiVersion: edgefs.rook.io/v1alpha1
kind: ISCSI
metadata:
  name: iscPepsi
  namespace: rook-edgefs
```

At this point two iSCSI Target services should be available and listening on default port 3260.
