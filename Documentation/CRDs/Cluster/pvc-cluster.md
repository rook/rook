---
title: PVC Storage Cluster
---

In a "PVC-based cluster", the Ceph persistent data is stored on volumes requested from a storage class of your choice.
This type of cluster is recommended in a cloud environment where volumes can be dynamically created and also
in clusters where a local PV provisioner is available.

## AWS Storage Example

In this example, the mon and OSD volumes are provisioned from the AWS `gp2` storage class. This storage class can be replaced by any storage class that provides `file` mode (for mons) and `block` mode (for OSDs).

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: quay.io/ceph/ceph:v18.2.4
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: false
    volumeClaimTemplate:
      spec:
        storageClassName: gp2
        resources:
          requests:
            storage: 10Gi
  storage:
    storageClassDeviceSets:
    - name: set1
      count: 3
      portable: false
      encrypted: false
      volumeClaimTemplates:
      - metadata:
          name: data
        spec:
          resources:
            requests:
              storage: 10Gi
          # IMPORTANT: Change the storage class depending on your environment (e.g. local-storage, gp2)
          storageClassName: gp2
          volumeMode: Block
          accessModes:
            - ReadWriteOnce
    onlyApplyOSDPlacement: false
```

### Local Storage Example

In the CRD specification below, 3 OSDs (having specific placement and resource values) and 3 mons with each using a 10Gi PVC, are created by Rook using the `local-storage` storage class.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: false
    volumeClaimTemplate:
      spec:
        storageClassName: local-storage
        resources:
          requests:
            storage: 10Gi
  cephVersion:
    image: quay.io/ceph/ceph:v18.2.4
    allowUnsupported: false
  dashboard:
    enabled: true
  network:
    hostNetwork: false
  storage:
    storageClassDeviceSets:
    - name: set1
      count: 3
      portable: false
      resources:
        limits:
          memory: "4Gi"
        requests:
          cpu: "500m"
          memory: "4Gi"
      placement:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: "rook.io/cluster"
                  operator: In
                  values:
                    - cluster1
              topologyKey: "topology.kubernetes.io/zone"
      volumeClaimTemplates:
      - metadata:
          name: data
        spec:
          resources:
            requests:
              storage: 10Gi
          storageClassName: local-storage
          volumeMode: Block
          accessModes:
            - ReadWriteOnce
```

## PVC storage only for monitors

In the CRD specification below three monitors are created each using a 10Gi PVC
created by Rook using the `local-storage` storage class. Even while the mons consume PVCs,
the OSDs in this example will still consume raw devices on the host.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: quay.io/ceph/ceph:v18.2.4
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: false
    volumeClaimTemplate:
      spec:
        storageClassName: local-storage
        resources:
          requests:
            storage: 10Gi
  dashboard:
    enabled: true
  storage:
    useAllNodes: true
    useAllDevices: true
```

## Dedicated metadata and wal device for OSD on PVC

In the simplest case, Ceph OSD BlueStore consumes a single (primary) storage device.
BlueStore is the engine used by the OSD to store data.

The storage device is normally used as a whole, occupying the full device that is managed directly by BlueStore.
It is also possible to deploy BlueStore across additional devices such as a DB device.
This device can be used for storing BlueStore’s internal metadata.
BlueStore (or rather, the embedded RocksDB) will put as much metadata as it can on the DB device to improve performance.
If the DB device fills up, metadata will spill back onto the primary device (where it would have been otherwise).
Again, it is only helpful to provision a DB device if it is faster than the primary device.

You can have multiple `volumeClaimTemplates` where each might either represent a device or a metadata device.
An example of the `storage` section when specifying the metadata device is:

```yaml
  storage:
   storageClassDeviceSets:
    - name: set1
      count: 3
      portable: false
      volumeClaimTemplates:
      - metadata:
          name: data
        spec:
          resources:
            requests:
              storage: 10Gi
          # IMPORTANT: Change the storage class depending on your environment (e.g. local-storage, gp2)
          storageClassName: gp2
          volumeMode: Block
          accessModes:
            - ReadWriteOnce
      - metadata:
          name: metadata
        spec:
          resources:
            requests:
              # Find the right size https://docs.ceph.com/docs/master/rados/configuration/bluestore-config-ref/#sizing
              storage: 5Gi
          # IMPORTANT: Change the storage class depending on your environment (e.g. local-storage, io1)
          storageClassName: io1
          volumeMode: Block
          accessModes:
            - ReadWriteOnce
```

!!! note
    Note that Rook only supports three naming convention for a given template:

* "data": represents the main OSD block device, where your data is being stored.
* "metadata": represents the metadata (including block.db and block.wal) device used to store the Ceph Bluestore database for an OSD.
* "wal": represents the block.wal device used to store the Ceph Bluestore database for an OSD. If this device is set, "metadata" device will refer specifically to block.db device.
It is recommended to use a faster storage class for the metadata or wal device, with a slower device for the data.
Otherwise, having a separate metadata device will not improve the performance.

The bluestore partition has the following reference combinations supported by the ceph-volume utility:

* A single "data" device.

    ```yaml
        storage:
        storageClassDeviceSets:
        - name: set1
            count: 3
            portable: false
            volumeClaimTemplates:
            - metadata:
                name: data
            spec:
                resources:
                requests:
                    storage: 10Gi
                # IMPORTANT: Change the storage class depending on your environment (e.g. local-storage, gp2)
                storageClassName: gp2
                volumeMode: Block
                accessModes:
                - ReadWriteOnce
    ```

* A "data" device and a "metadata" device.

    ```yaml
        storage:
        storageClassDeviceSets:
        - name: set1
            count: 3
            portable: false
            volumeClaimTemplates:
            - metadata:
                name: data
            spec:
                resources:
                requests:
                    storage: 10Gi
                # IMPORTANT: Change the storage class depending on your environment (e.g. local-storage, gp2)
                storageClassName: gp2
                volumeMode: Block
                accessModes:
                - ReadWriteOnce
            - metadata:
                name: metadata
            spec:
                resources:
                requests:
                    # Find the right size https://docs.ceph.com/docs/master/rados/configuration/bluestore-config-ref/#sizing
                    storage: 5Gi
                # IMPORTANT: Change the storage class depending on your environment (e.g. local-storage, io1)
                storageClassName: io1
                volumeMode: Block
                accessModes:
                - ReadWriteOnce
    ```

* A "data" device and a "wal" device.
A WAL device can be used for BlueStore’s internal journal or write-ahead log (block.wal), it is only useful to use a WAL device if the device is faster than the primary device (data device).
There is no separate "metadata" device in this case, the data of main OSD block and block.db located in "data" device.

    ```yaml
        storage:
        storageClassDeviceSets:
        - name: set1
            count: 3
            portable: false
            volumeClaimTemplates:
            - metadata:
                name: data
            spec:
                resources:
                requests:
                    storage: 10Gi
                # IMPORTANT: Change the storage class depending on your environment (e.g. local-storage, gp2)
                storageClassName: gp2
                volumeMode: Block
                accessModes:
                - ReadWriteOnce
            - metadata:
                name: wal
            spec:
                resources:
                requests:
                    # Find the right size https://docs.ceph.com/docs/master/rados/configuration/bluestore-config-ref/#sizing
                    storage: 5Gi
                # IMPORTANT: Change the storage class depending on your environment (e.g. local-storage, io1)
                storageClassName: io1
                volumeMode: Block
                accessModes:
                - ReadWriteOnce
    ```

* A "data" device, a "metadata" device and a "wal" device.

    ```yaml
        storage:
        storageClassDeviceSets:
        - name: set1
            count: 3
            portable: false
            volumeClaimTemplates:
            - metadata:
                name: data
            spec:
                resources:
                requests:
                    storage: 10Gi
                # IMPORTANT: Change the storage class depending on your environment (e.g. local-storage, gp2)
                storageClassName: gp2
                volumeMode: Block
                accessModes:
                - ReadWriteOnce
            - metadata:
                name: metadata
            spec:
                resources:
                requests:
                    # Find the right size https://docs.ceph.com/docs/master/rados/configuration/bluestore-config-ref/#sizing
                    storage: 5Gi
                # IMPORTANT: Change the storage class depending on your environment (e.g. local-storage, io1)
                storageClassName: io1
                volumeMode: Block
                accessModes:
                - ReadWriteOnce
            - metadata:
                name: wal
            spec:
                resources:
                requests:
                    # Find the right size https://docs.ceph.com/docs/master/rados/configuration/bluestore-config-ref/#sizing
                    storage: 5Gi
                # IMPORTANT: Change the storage class depending on your environment (e.g. local-storage, io1)
                storageClassName: io1
                volumeMode: Block
                accessModes:
                - ReadWriteOnce
    ```

To determine the size of the metadata block follow the [official Ceph sizing guide](https://docs.ceph.com/docs/master/rados/configuration/bluestore-config-ref/#sizing).

With the present configuration, each OSD will have its main block allocated a 10GB device as well a 5GB device to act as a bluestore database.
