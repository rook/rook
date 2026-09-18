---
title: Custom Images
---

CSI drivers are now managed by the [ceph-csi-operator](https://github.com/ceph/ceph-csi-operator).

### **Override the default images**

For scenarios that require custom images (e.g. downstream releases or air-gapped environments),
create an `ImageSet` ConfigMap in the operator namespace and reference it from the `OperatorConfig`
CR. The ConfigMap is optional; if it is not present, the ceph-csi-operator uses its code defaults.

1. Create the ConfigMap with the desired images:

    ```yaml
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: my-custom-csi-images
      namespace: rook-ceph # operator namespace
    data:
      plugin: "quay.io/cephcsi/cephcsi:v3.17.1"
      provisioner: "registry.k8s.io/sig-storage/csi-provisioner:v6.2.0"
      attacher: "registry.k8s.io/sig-storage/csi-attacher:v4.12.0"
      resizer: "registry.k8s.io/sig-storage/csi-resizer:v2.1.0"
      snapshotter: "registry.k8s.io/sig-storage/csi-snapshotter:v8.5.0"
      registrar: "registry.k8s.io/sig-storage/csi-node-driver-registrar:v2.17.0"
      addons: "quay.io/csiaddons/k8s-sidecar:v0.14.0"
    ```

2. Reference the ConfigMap in the `OperatorConfig`:

    ```yaml
    apiVersion: csi.ceph.io/v1
    kind: OperatorConfig
    metadata:
      name: ceph-csi-operator-config
      namespace: rook-ceph
    spec:
      driverSpecDefaults:
        imageSet:
          name: my-custom-csi-images
    ```

For Helm installs, set the `imageSet` in the `ceph-csi-drivers` chart values:

```yaml
operatorConfig:
  driverSpecDefaults:
    imageSet:
      name: my-custom-csi-images
```

### **Use default images**

To use the default upstream images, do not create an `ImageSet` ConfigMap. The ceph-csi-operator
will use the images built into its release.

### **Verifying updates**

Use the below command to see the CSI images currently being used in the cluster.
Not all images may be present depending on which CSI features are enabled.

```console
kubectl --namespace rook-ceph get pod -o jsonpath='{range .items[*]}{range .spec.containers[*]}{.image}{"\n"}' -l 'app.kubernetes.io/part-of in (ceph-csi-rbd, ceph-csi-cephfs, ceph-csi-nfs)' | sort | uniq
```
