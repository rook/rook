---
title: OpenStack/SWIFT CRD
weight: 4550
indent: true
---

# EdgeFS OpenStack/SWIFT CRD

Rook allows creation and customization of OpenStack/SWIFT compatible services through the custom resource definitions (CRDs).
The following settings are available for customization of SWIFT services.

## Sample

```yaml
apiVersion: edgefs.rook.io/v1
kind: SWIFT
metadata:
  name: swift01
  namespace: rook-edgefs
spec:
  instances: 3
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
  #          - swift-node
  #  tolerations:
  #  - key: swift-node
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

* `name`: The name of the SWIFT service to create, which must match existing EdgeFS service.
* `namespace`: The namespace of the Rook cluster where the SWIFT service is created.
* `sslCertificateRef`: If the certificate is not specified, SSL will use default crt and key files. If specified, this is the name of the Kubernetes secret that contains the SSL certificate to be used for secure connections. Please see [secret YAML file example](/cluster/examples/kubernetes/edgefs/sslKeyCertificate.yaml) on how to setup Kuberenetes secret. Notice that base64 encoding is required.
* `port`: The port on which the SWIFT pods and the SWIFT service will be listening (not encrypted). Default port is 9981.
* `securePort`: The secure port on which SWIFT pods will be listening. If not defined then default SSL certificates will be used. Default port is 443.
* `instances`: The number of active SWIFT service instances. For load balancing we recommend to use nginx and the like solutions.
* `chunkCacheSize`: Limit amount of memory allocated for dynamic chunk cache. By default SWIFT pod uses up to 75% of available memory as chunk caching area. This option can influence this allocation strategy.
* `annotations`: Key value pair list of annotations to add.
* `placement`: The SWIFT pods can be given standard Kubernetes placement restrictions with `nodeAffinity`, `tolerations`, `podAffinity`, and `podAntiAffinity` similar to placement defined for daemons configured by the [cluster CRD](/cluster/examples/kubernetes/edgefs/cluster.yaml).
* `resourceProfile`: SWIFT pod resource utilization profile (Memory and CPU). Can be `embedded` or `performance` (default). In case of `performance` an SWIFT pod trying to increase amount of internal I/O resources that results in higher performance at the cost of additional memory allocation and more CPU load. In `embedded` profile case, SWIFT pod gives preference to preserving memory over I/O and limiting chunk cache (see `chunkCacheSize` option). The `performance` profile is the default unless cluster wide `embedded` option is defined.
* `resources`: Set resource requests/limits for the SWIFT pods, see [Resource Requirements/Limits](edgefs-cluster-crd.md#resource-requirementslimits).

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

Verify that HW (or better say emulated in this case) configuration look normal and accept it

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

```
efscli tenant create Hawaii/Cola
efscli bucket create Hawaii/Cola/bk1
efscli tenant create Hawaii/Pepsi
efscli bucket create Hawaii/Pepsi/bk1
```

Now cluster is setup, services can be now created.

4. Create SWIFT services objects for tenants:

```console
efscli service create swift swift-cola
efscli service serve swift-cola Hawaii
efscli service create swift swiftPepsi
efscli service serve swift-pepsi Hawaii
```

5. Create EdgeFS SWIFT objects:

```yaml
apiVersion: edgefs.rook.io/v1
kind: SWIFT
metadata:
  name: swiftCola
  namespace: rook-edgefs
spec:
  instances: 1
```

```yaml
apiVersion: edgefs.rook.io/v1
kind: SWIFT
metadata:
  name: swiftPepsi
  namespace: rook-edgefs
spec:
  instances: 1
```

At this point two SWIFT services should be available and listening on default ports.
