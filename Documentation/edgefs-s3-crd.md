---
title: EdgeFS AWS S3 CRD
weight: 45
indent: true
---

# EdgeFS AWS S3 CRD

Rook allows creation and customization of AWS S3 compatible services through the custom resource definitions (CRDs).
The following settings are available for customization of S3 services.

## Sample

```yaml
apiVersion: edgefs.rook.io/v1alpha1
kind: S3
metadata:
  name: s301
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
  #          - s3-node
  #  tolerations:
  #  - key: s3-node
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

- `name`: The name of the S3 system to create, which must match existing EdgeFS service.
- `namespace`: The namespace of the Rook cluster where the S3 service is created.
- `sslCertificateRef`: If the certificate is not specified, SSL will use default crt and key files. If specified, this is the name of the Kubernetes secret that contains the SSL certificate to be used for secure connections. Please see [secret YAML file example](/cluster/examples/kubernetes/edgefs/sslKeyCertificate.yaml) on how to setup Kuberenetes secret. Notice that base64 encoding is required.
- `port`: The port on which the S3 pods and the S3 service will be listening (not encrypted). Default port is 9982.
- `securePort`: The secure port on which S3 pods will be listening. If not defined then default SSL certificates will be used. Default port is 8443.
- `instances`: The number of active S3 service instances. For load balancing we recommend to use nginx and the like solutions.
- `placement`: The S3 pods can be given standard Kubernetes placement restrictions with `nodeAffinity`, `tolerations`, `podAffinity`, and `podAntiAffinity` similar to placement defined for daemons configured by the [cluster CRD](/cluster/examples/kubernetes/edgefs/cluster.yaml).
- `resources`: Set resource requests/limits for the S3 pods, see [Resource Requirements/Limits](edgefs-cluster-crd.md#resource-requirementslimits).

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

4. Create S3 services objects for tenants

<pre>
efscli service create s3 s3Cola
efscli service serve s3Cola Hawaii/Cola
efscli service create s3 s3Pepsi
efscli service serve s3Pepsi Hawaii/Pepsi/bk1
</pre>

5. Create S3X CRDs

```yaml
apiVersion: edgefs.rook.io/v1alpha1
kind: S3
metadata:
  name: s3Cola
  namespace: rook-edgefs
spec:
  instances: 1
```

```yaml
apiVersion: edgefs.rook.io/v1alpha1
kind: S3
metadata:
  name: s3Pepsi
  namespace: rook-edgefs
spec:
  instances: 1
```

At this point two S3 services should be available and listening on default ports.
