---
title: EdgeFS Scale-Out NFS CRD
weight: 43
indent: true
---

# EdgeFS Scale-Out NFS CRD

Rook allows creation and customization of EdgeFS NFS file systems through the custom resource definitions (CRDs).
The following settings are available for customization of EdgeFS NFS services.

## Sample

```yaml
apiVersion: edgefs.rook.io/v1alpha1
kind: NFS
metadata:
  name: nfs01
  namespace: rook-edgefs
spec:
  instances: 3
  placement:
  #  nodeAffinity:
  #    requiredDuringSchedulingIgnoredDuringExecution:
  #      nodeSelectorTerms:
  #      - matchExpressions:
  #        - key: role
  #          operator: In
  #          values:
  #          - nfs-node
  #  tolerations:
  #  - key: nfs-node
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

- `name`: The name of the NFS system to create, which must match existing EdgeFS service.
- `namespace`: The namespace of the Rook cluster where the NFS service is created.
- `instances`: The number of active NFS service instances. EdgeFS NFS service is Multi-Head capable, such so that multiple PODs can mount same tenant's buckets via different endpoints. [EdgeFS CSI provisioner](edgefs-csi.md) orchestrates distribution and load balancing across NFS service instances in round-robin or random policy ways.
- `placement`: The NFS PODs can be given standard Kubernetes placement restrictions with `nodeAffinity`, `tolerations`, `podAffinity`, and `podAntiAffinity` similar to placement defined for daemons configured by the [cluster CRD](/cluster/examples/kubernetes/edgefs/cluster.yaml).
- `resources`: Set resource requests/limits for the NFS Pod(s), see [Resource Requirements/Limits](edgefs-cluster-crd.md#resource-requirementslimits).

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

Now cluster is setup, services can be now created and attached to CSI provisioner.

4. Create NFS service objects for tenants

<pre>
efscli service create nfs nfsCola
efscli service serve nfsCola Hawaii/Cola/bk1
efscli service create nfs nfsPepsi
efscli service serve nfsPepsi Hawaii/Pepsi/bk1
</pre>

5. Create NFS CRDs

```yaml
apiVersion: edgefs.rook.io/v1alpha1
kind: NFS
metadata:
  name: nfsCola
  namespace: rook-edgefs
spec:
  instances: 1
```

```yaml
apiVersion: edgefs.rook.io/v1alpha1
kind: NFS
metadata:
  name: nfsPepsi
  namespace: rook-edgefs
spec:
  instances: 1
```

At this point two NFS services should be available. Verify that showmount command can see service (substitue CLUSTERIP with corresponding entry from `kubectl get svc` command):

<pre>
kubectl get svc --all-namespaces
showmount -e CLUSTERIP
</pre>
</pre>
