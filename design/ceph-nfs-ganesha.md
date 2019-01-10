# Ceph NFS-Ganesha CRD

[NFS-Ganesha] is a user space NFS server that is well integrated with [CephFS]
and [RGW] backends. It can export Ceph's filesystem namespaces and Object
gateway namespaces over NFSv4 protocol.

Rook already orchestrates Ceph filesytem and Object store (or RGW) on
Kubernetes (k8s). It can be extended to orchestrate NFS-Ganesha server daemons
as highly available and  scalable NFS gateway pods to the Ceph filesytem and
Object Store. This will allow NFS client applications to use the Ceph filesytem
and object store setup by rook.

This feature mainly differs from the feature to add NFS as an another
storage backend for rook (the general NFS solution) in the following ways:

* It will use the rook's Ceph operator and not a separate NFS operator to
  deploy the NFS server pods.

* The NFS server pods will be directly configured with CephFS or RGW
  backend setup by rook, and will not require CephFS or RGW to be mounted
  in the NFS server pod with a PVC.

## Design of Ceph NFS-Ganesha CRD

The NFS-Ganesha server and its exports' settings will be exposed to rook as a
Custom Resource Definition (CRD). Creating the nfs-ganesha CRD will launch
a cluster of NFS-Ganesha server pods that will be configured with a rook CephFS
filesystem or a RGW object store backend. The servers' exports will be stored
in [k8s ConfigMaps]; the NFS client recovery data will be stored in a Ceph RADOS
pool; and the servers will have stable IP addresses by using [k8s Service].
This allows the NFS-Ganesha server cluster to be scalable and highly available.

### Prerequisities

- A running rook Ceph filesystem or object store, whose namespaces will be
  exported by the NFS-Ganesha server cluster.
  e.g.,
  ```
  kubectl create -f cluster/examples/kubernetes/ceph/filesystem.yaml
  ```

- An existing RADOS pool (e.g., CephFS's data pool) or a pool created with a
  [Ceph Pool CRD] to store NFS client recovery data.


### Ceph NFS-Ganesha CRD

The NFS-Ganesha CRD will specify the following:

- Ceph filesystem or object store (RGW) that will be exported

- Number of active Ganesha servers in the cluster

- Placement of the Ganesha servers

- Resource limits (memory, CPU) of the Ganesha server pods

- NFS-Ganesha server export settings

- RADOS pool and namespace where the NFS client recovery data will be stored


Below is an example NFS-Ganesha CRD, `nfs-ganesha.yaml`

```yaml
apiVersion: ceph.rook.io/v1alpha1
kind: NFSGanesha
metadata:
  # The name of Ganesha server cluster to create. It will be reflected in
  # the name(s) of the ganesha server pod(s)
  name: mynfs
  # The namespace of the Rook cluster where the Ganesha server cluster is
  # created.
  namespace: rook-ceph
spec:
  store:
    # The name of the filesystem or object store CRD that ganesha will export
    name: myfs
    # Either "file" for a filesystem or "object" for the object store
    type: file

  # NFS client recovery storage settings
  clientRecovery:
    # RADOS pool where NFS client recovery data is stored.
    # In this example the data pool for the "myfs" filesystem is used.
    # If using the object store example, the data pool would be
    # "my-store.rgw.buckets.data".
    pool: myfs-data0
    # RADOS namespace where NFS client recovery data is stored.
    namespace: ganesha-ns

  # Export settings
  exports:
    # The pseudoroot path. This is where the export will appear in the
    # NFSv4 pseudoroot namespace.
  - pseudoPath: /cephfs
    # The directory in the exported file system this export is rooted on
    path: /
    accessType: ReadWrite
    squash: No_root_squash
    # List of allowed clients and their settings
    #allowedClients:
    #- clients: 192.168.0.0/16, minikube
    #  accessType: ReadOnly
    #  squash: none

  # Settings for the ganesha server
  server:
    # the number of active ganesha servers
    active: 3
    # where to run the nfs ganesha server
    placement:
    #  nodeAffinity:
    #    requiredDuringSchedulingIgnoredDuringExecution:
    #      nodeSelectorTerms:
    #      - matchExpressions:
    #        - key: role
    #          operator: In
    #          values:
    #          - mds-node
    #  tolerations:
    #  - key: mds-node
    #    operator: Exists
    #  podAffinity:
    #  podAntiAffinity:
    # The requests and limits set here allow the ganesha pod(s) to use half of
    # one CPU core and 1 gigabyte of memory
    resources:
    #  limits:
    #    cpu: "500m"
    #    memory: "1024Mi"
    #  requests:
    #    cpu: "500m"
    #    memory: "1024Mi"
```

When the  nfs-ganesha.yaml is created the following will happen:

- Rook's Ceph operator sees the creation of the NFS-Ganesha CRD. It
  generates exports specified in the CRD and stores them as ConfigMaps.

- The operator creates as many [k8s Deployments] as the number of active
  Ganesha servers mentioned in the CRD. Each deployment brings up a Ganesha
  server pod, a replicaset of size 1.

- The ganesha servers, each running in a separate pod, use identical ganesha
  config (ganesha.conf) that refer to exports stored as ConfigMaps.

- The operator creates a k8s service for each of the ganesha server pods
  to allow each of the them to have a stable IP address.

The ganesha server pods constitute an active-active high availability NFS
server cluster. If one of the active Ganesha server pods goes down, k8s brings
up a replacment ganesha server pod with the same configuration and IP address.
The NFS server cluster can be scaled up or down by updating the
number of the active Ganesha servers in the CRD (using `kubectl edit` or
modifying the original CRD and running `kubectly apply -f <CRD yaml file>`).

### Dynamically adding, updating, and  removing exports of running Ganesha servers

To dynamically update the exports of a Ganesha cluster you will have to update
its CRD. On updating the CRD, the operator  will update the exports stored in
the ConfigMaps, and will send DBus signals to the Ganesha servers to refresh
their exports.

### Consuming the NFS shares

An external consumer will fetch the ganesha server IPs by querying the k8s
services of the Ganesha server pods. It should have network access to the
Ganesha pods to manually mount the shares using a NFS client. Later, support
will be added to allow user pods to easily consume the NFS shares via PVCs.

## Example use-case

The NFS shares exported by rook's ganesha server pods can be consumed by
[OpenStack] cloud's user VMs. To do this, OpenStack's shared file system
service, [Manila] will provision NFS shares backed by CephFS using rook.
Manila's [CephFS driver] will create NFS-Ganesha CRDs to launch ganesha server
pods. The driver will dynamically add or remove exports of the ganesha server
pods based on OpenStack users' requests. The OpenStack user VMs will have
network connectivity to the ganesha server pods, and manually mount the shares
using NFS clients.

[NFS-Ganesha]: https://github.com/nfs-ganesha/nfs-ganesha/wiki
[CephFS]: http://docs.ceph.com/docs/master/cephfs/nfs/
[RGW]: http://docs.ceph.com/docs/master/radosgw/nfs/
[Rook toolbox]: (/Documentation/ceph-toolbox.md)
[Ceph manager]: (http://docs.ceph.com/docs/master/mgr/)
[OpenStack]: (https://www.openstack.org/software/)
[Manila]: (https://wiki.openstack.org/wiki/Manila)
[CephFS driver]: (https://github.com/openstack/manila/blob/master/doc/source/admin/cephfs_driver.rst)
[k8s ConfigMaps]: (https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-configmap/)
[k8s Service]: (https://kubernetes.io/docs/concepts/services-networking/service)
[Ceph Pool CRD]: (https://github.com/rook/rook/blob/master/Documentation/ceph-pool-crd.md)
[k8s Deployments]: (https://kubernetes.io/docs/concepts/workloads/controllers/deployment/)
