---
title: AWS S3 CRD
weight: 4500
indent: true
---

# EdgeFS AWS S3 CRD

Rook allows creation and customization of AWS S3 compatible services through the custom resource definitions (CRDs).
The following settings are available for customization of S3 services.

## Sample

```yaml
apiVersion: edgefs.rook.io/v1
kind: S3
metadata:
  name: s301
  namespace: rook-edgefs
spec:
  instances: 3
  #s3type: s3s
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
  #          - s3-node
  #  tolerations:
  #  - key: s3-node
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

* `name`: The name of the S3 system to create, which must match existing EdgeFS service.
* `namespace`: The namespace of the Rook cluster where the S3 service is created.
* `s3type`: The type of S3 service to be created. It can be one of the following: `s3` (default, path style) or `s3s` (buckets as DNS style)
* `sslCertificateRef`: If the certificate is not specified, SSL will use default crt and key files. If specified, this is the name of the Kubernetes secret that contains the SSL certificate to be used for secure connections. Please see [secret YAML file example](/cluster/examples/kubernetes/edgefs/sslKeyCertificate.yaml) on how to setup Kuberenetes secret. Notice that base64 encoding is required.
* `port`: The port on which the S3 pods and the S3 service will be listening (not encrypted). Default port is 9982 for `s3` and 9983 for `s3s`.
* `securePort`: The secure port on which S3 pods will be listening. If not defined then default SSL certificates will be used. Default port is 8443 for `s3` and 8444 for `s3s`.
* `chunkCacheSize`: Limit amount of memory allocated for dynamic chunk cache. By default S3 pod uses up to 75% of available memory as chunk caching area. This option can influence this allocation strategy.
* `instances`: The number of active S3 service instances. For load balancing we recommend to use nginx and the like solutions.
* `annotations`: Key value pair list of annotations to add.
* `placement`: The S3 pods can be given standard Kubernetes placement restrictions with `nodeAffinity`, `tolerations`, `podAffinity`, and `podAntiAffinity` similar to placement defined for daemons configured by the [cluster CRD](/cluster/examples/kubernetes/edgefs/cluster.yaml).
* `resourceProfile`: S3 pod resource utilization profile (Memory and CPU). Can be `embedded` or `performance` (default). In case of `performance` an S3 pod trying to increase amount of internal I/O resources that results in higher performance at the cost of additional memory allocation and more CPU load. In `embedded` profile case, S3 pod gives preference to preserving memory over I/O and limiting chunk cache (see `chunkCacheSize` option). The `performance` profile is the default unless cluster wide `embedded` option is defined.
* `resources`: Set resource requests/limits for the S3 pods, see [Resource Requirements/Limits](edgefs-cluster-crd.md#resource-requirementslimits).

## Setting up EdgeFS namespace and tenant

For more detailed instructions please refer to [EdgeFS Wiki](https://github.com/Nexenta/edgefs/wiki).

Below is an exampmle procedure to get things initialized and configured.

Before new local namespace (or local site) can be used, it has to be initialized with FlexHash and special purpose root object.

FlexHash consists of dynamically discovered configuration and checkpoint of accepted distribution table. FlexHash is responsible for I/O direction and plays important role in dynamic load balancing logic. It defines so-called Negotiating Groups (typically across zoned 8-24 disks) and final table distribution across all the participating components, e.g. data nodes, service gateways and tools.

Root object holds system information and table of namespaces registered to a local site. Root object is always local and never shared between the sites.

To initialize system and prepare logical definitions, login to the toolbox as shown in this example:

```console
kubectl get po --all-namespaces | grep edgefs-mgr
kubectl exec -it -n rook-edgefs rook-edgefs-mgr-6cb9597469-czr7p -- env COLUMNS=$COLUMNS LINES=$LINES TERM=linux toolbox
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

```console
efscli tenant create Hawaii/Cola
efscli bucket create Hawaii/Cola/bk1
efscli tenant create Hawaii/Pepsi
efscli bucket create Hawaii/Pepsi/bk1
```

Now cluster is setup, services can be now created.

4. Create S3 services objects for tenants:

```console
efscli service create s3 s3-cola
efscli service serve s3-cola Hawaii/Cola
efscli service create s3 s3-pepsi
efscli service serve s3-pepsi Hawaii/Pepsi
```

In case of s3type set to `s3`, do not forget to configure default domain name:

```console
efscli service config s3-cola X-Domain cola.com
efscli service config s3-pepsi X-Domain pepsi.com
```

5. Create EdgeFS S3 objects:

```yaml
apiVersion: edgefs.rook.io/v1
kind: S3
metadata:
  name: s3-cola
  namespace: rook-edgefs
spec:
  instances: 1
```

```yaml
apiVersion: edgefs.rook.io/v1
kind: S3
metadata:
  name: s3-pepsi
  namespace: rook-edgefs
spec:
  instances: 1
```

At this point two S3 services should be available and listening on default ports.
