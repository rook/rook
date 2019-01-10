---
title: Edge-X S3 CRD
weight: 44
indent: true
---

# Edge-X S3 CRD

The API provides access to advanced EdgeFS Object interfaces, such as access to Key-Value store, S3 Object Append mode, S3 Object RW mode and S3 Object Stream Session (POSIX compatible) mode. A Stream Session encompasses a series of edits to one object made by one source that are saved as one or more versions during a specific finite time duration. A Stream Session must be isolated while it is open. That is, users working through this session will not see updates to this object from other sessions.Stream Session allows high-performance POSIX-style access to an object and thus it is beneficial for client applications to use HTTP/1.1 Persistent Connection extensions, to minimize latency between updates or reads.

For more detailes on API please refer to [Edge-S3 API](https://edgex.docs.apiary.io/).

Rook allows creation and customization of Edge-X S3 services through the custom resource definitions (CRDs).
The following settings are available for customization of Edge-S3 services.

## Sample

```yaml
apiVersion: edgefs.rook.io/v1alpha1
kind: S3X
metadata:
  name: s3x01
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
  #          - s3x-node
  #  tolerations:
  #  - key: s3x-node
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

- `name`: The name of the Edge-X S3 system to create, which must match existing EdgeFS service.
- `namespace`: The namespace of the Rook cluster where the Edge-X S3 service is created.
- `sslCertificateRef`: If the certificate is not specified, SSL will use default crt and key files. If specified, this is the name of the Kubernetes secret that contains the SSL certificate to be used for secure connections. Please see [secret YAML file example](/cluster/examples/kubernetes/edgefs/sslKeyCertificate.yaml) on how to setup Kuberenetes secret. Notice that base64 encoding is required.
- `port`: The port on which the Edge-X S3 pods and the Edge-X S3 service will be listening (not encrypted). Default port is 3000.
- `securePort`: The secure port on which Edge-X S3 pods will be listening. If not defined then default SSL certificates will be used. Default port is 3001.
- `instances`: The number of active Edge-X S3 service instances. For load balancing we recommend to use nginx and the like solutions.
- `placement`: The Edge-X S3 PODs can be given standard Kubernetes placement restrictions with `nodeAffinity`, `tolerations`, `podAffinity`, and `podAntiAffinity` similar to placement defined for daemons configured by the [cluster CRD](/cluster/examples/kubernetes/edgefs/cluster.yaml).
- `resources`: Set resource requests/limits for the Edge-X S3 Pod(s), see [Resource Requirements/Limits](edgefs-cluster-crd.md#resource-requirementslimits).

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

4. Create Edge-X S3 services objects for tenants

<pre>
efscli service create s3x s3xCola
efscli service serve s3xCola Hawaii/Cola
efscli service create s3x s3xPepsi
efscli service serve s3xPepsi Hawaii/Pepsi/bk1
</pre>

5. Create S3X CRDs

```yaml
apiVersion: edgefs.rook.io/v1alpha1
kind: S3X
metadata:
  name: s3xCola
  namespace: rook-edgefs
spec:
  instances: 1
```

```yaml
apiVersion: edgefs.rook.io/v1alpha1
kind: S3X
metadata:
  name: s3xPepsi
  namespace: rook-edgefs
spec:
  instances: 1
```

At this point two Edge-X S3 services should be available and listening on default ports.
