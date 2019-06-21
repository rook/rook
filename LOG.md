# Log

```sh
# Remove prev installetion
$ kubectl -n test delete pvc vol1
$ kubectl delete ns test
$ kubectl delete storageclass rook-ceph-block rook-ceph-block-r2
$ kubectl -n rook-ceph delete cephcluster rook-ceph
$ kubectl -n rook-ceph delete deployment rook-ceph-operator
$ kubectl -n rook-ceph delete daemonset rook-ceph-agent rook-discover
$ kubectl -n rook-ceph get volumes,cephblockpools,cephclusters,cephfilesystems,cephnfses,cephobjectstores,cephobjectstoreusers
$ kubectl -n rook-ceph get pods
```

```sh
# Running Ceph CSI drivers with Rook

# Prerequisites:
# - a Kubernetes v1.13+ is needed in order to support CSI Spec 1.0.
# - --allow-privileged flag set to true in kubelet and your API server
# - An up and running Rook instance (see Rook - Ceph quickstart guide)

# Create RBAC used by CSI drivers in the same namespace as Rook Ceph Operator
# create rbac. Since rook operator is not permitted to create rbac rules,
# these rules have to be created outside of operator
$ kubectl apply -f cluster/examples/kubernetes/ceph/csi/rbac/rbd/
serviceaccount/rook-csi-rbd-plugin-sa created
clusterrole.rbac.authorization.k8s.io/rbd-csi-nodeplugin created
clusterrole.rbac.authorization.k8s.io/rbd-csi-nodeplugin-rules created
clusterrolebinding.rbac.authorization.k8s.io/rbd-csi-nodeplugin created
serviceaccount/rook-csi-rbd-provisioner-sa created
clusterrole.rbac.authorization.k8s.io/rbd-external-provisioner-runner created
clusterrole.rbac.authorization.k8s.io/rbd-external-provisioner-runner-rules created
clusterrolebinding.rbac.authorization.k8s.io/rbd-csi-provisioner-role created

$ kubectl apply -f cluster/examples/kubernetes/ceph/csi/rbac/cephfs/
serviceaccount/rook-csi-cephfs-plugin-sa created
clusterrole.rbac.authorization.k8s.io/cephfs-csi-nodeplugin created
clusterrole.rbac.authorization.k8s.io/cephfs-csi-nodeplugin-rules created
clusterrolebinding.rbac.authorization.k8s.io/cephfs-csi-nodeplugin created
serviceaccount/rook-csi-cephfs-provisioner-sa created
clusterrole.rbac.authorization.k8s.io/cephfs-external-provisioner-runner created
clusterrole.rbac.authorization.k8s.io/cephfs-external-provisioner-runner-rules created
clusterrolebinding.rbac.authorization.k8s.io/cephfs-csi-provisioner-role created

# Start Rook Ceph Operator
$ kubectl apply -f cluster/examples/kubernetes/ceph/operator-with-csi.yaml

# Verify CSI drivers and Operator are up and running
$ kubectl get all -n rook-ceph
```

```yaml
# core@server31 ~ $ kubectl get storageclass rook-ceph-block -oyaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  creationTimestamp: "2019-06-13T12:14:41Z"
  name: rook-ceph-block
  resourceVersion: "270716"
  selfLink: /apis/storage.k8s.io/v1/storageclasses/rook-ceph-block
  uid: d2678c9e-8dd4-11e9-a101-0e4bd1f2a3d0
parameters:
  blockPool: replicapool
  clusterNamespace: rook-ceph
  fstype: xfs
provisioner: ceph.rook.io/block
reclaimPolicy: Delete
volumeBindingMode: Immediate
# core@server31 ~ $ kubectl get storageclass rook-ceph-block-r2 -oyaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  creationTimestamp: "2019-06-13T12:27:53Z"
  name: rook-ceph-block-r2
  resourceVersion: "273488"
  selfLink: /apis/storage.k8s.io/v1/storageclasses/rook-ceph-block-r2
  uid: aa72987f-8dd6-11e9-a101-0e4bd1f2a3d0
parameters:
  blockPool: replicapool-r2
  clusterNamespace: rook-ceph
  fstype: xfs
provisioner: ceph.rook.io/block
reclaimPolicy: Delete
volumeBindingMode: Immediate
# core@server31 ~ $ kubectl -n rook-ceph get cephcluster rook-ceph -oyaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"ceph.rook.io/v1","kind":"CephCluster","metadata":{"annotations":{},"name":"rook-ceph","namespace":"rook-ceph"},"spec":{"annotations":null,"cephVersion":{"allowUnsupported":false,"image":"ceph/ceph:v13.2.6-20190604"},"dashboard":{"enabled":true},"dataDirHostPath":"/var/lib/rook","mon":{"allowMultiplePerNode":false,"count":3},"network":{"hostNetwork":false},"rbdMirroring":{"workers":0},"resources":null,"storage":{"config":{"databaseSizeMB":"1024","journalSizeMB":"1024","osdsPerDevice":"1"},"deviceFilter":"^sd.","location":null,"useAllDevices":false,"useAllNodes":true}}}
  creationTimestamp: "2019-06-12T15:56:53Z"
  finalizers:
  - cephcluster.ceph.rook.io
  generation: 12409
  name: rook-ceph
  namespace: rook-ceph
  resourceVersion: "2597972"
  selfLink: /apis/ceph.rook.io/v1/namespaces/rook-ceph/cephclusters/rook-ceph
  uid: b2a7f759-8d2a-11e9-a101-0e4bd1f2a3d0
spec:
  cephVersion:
    image: ceph/ceph:v13.2.6-20190604
  dashboard:
    enabled: true
  dataDirHostPath: /var/lib/rook
  mon:
    allowMultiplePerNode: false
    count: 3
    preferredCount: 0
  network:
    hostNetwork: false
  rbdMirroring:
    workers: 0
  storage:
    config:
      databaseSizeMB: "1024"
      journalSizeMB: "1024"
      osdsPerDevice: "1"
    deviceFilter: ^sd.
    useAllDevices: false
    useAllNodes: true
status:
  ceph:
    health: HEALTH_OK
    lastChanged: "2019-06-19T09:42:36Z"
    lastChecked: "2019-06-21T10:50:56Z"
    previousHealth: HEALTH_WARN
  state: Created
```
