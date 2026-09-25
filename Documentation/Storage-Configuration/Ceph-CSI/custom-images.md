---
title: Custom Images
---

CSI drivers are now managed by the [ceph-csi-operator](https://github.com/ceph/ceph-csi-operator).

Default CSI container images are built into the ceph-csi-operator. Rook ships
the `rook-csi-operator-image-set-configmap` configmap with empty values so the
ceph-csi-operator uses its built-in defaults. Rook no longer pins CSI sidecar/plugin versions.

### **Override the default images**

To use custom images (e.g. downstream releases or air-gapped environments),
set the desired values in the configmap. Only non-empty values override defaults.

**Manifest installs:**

```console
kubectl -n $ROOK_OPERATOR_NAMESPACE edit configmap rook-csi-operator-image-set-configmap
```

Configmap keys:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: rook-csi-operator-image-set-configmap
  namespace: rook-ceph # operator namespace
data:
  plugin: "quay.io/cephcsi/cephcsi:v3.17.1"
  provisioner: "registry.k8s.io/sig-storage/csi-provisioner:v6.3.0"
  attacher: "registry.k8s.io/sig-storage/csi-attacher:v4.13.0"
  resizer: "registry.k8s.io/sig-storage/csi-resizer:v2.2.1"
  snapshotter: "registry.k8s.io/sig-storage/csi-snapshotter:v8.6.0"
  registrar: "registry.k8s.io/sig-storage/csi-node-driver-registrar:v2.18.0"
  addons: "quay.io/csiaddons/k8s-sidecar:v0.15.1"
```

**Helm installs:** set the image values under the `csi` section of the `rook-ceph` chart:

```yaml
csi:
  cephcsi:
    repository: quay.io/cephcsi/cephcsi
    tag: v3.17.1
  provisioner:
    repository: registry.k8s.io/sig-storage/csi-provisioner
    tag: v6.3.0
```

The chart renders these values into the `rook-csi-operator-image-set-configmap`.
tag are empty (the default), the configmap value is
empty and the ceph-csi-operator uses its built-in image.

### **Use default images**

Leave the configmap values empty (the default). The ceph-csi-operator
uses the images built into its release.

### **Verifying updates**

List the CSI images currently in use:

```console
kubectl --namespace rook-ceph get pod -o jsonpath='{range .items[*]}{range .spec.containers[*]}{.image}{"\n"}' -l 'app.kubernetes.io/part-of in (ceph-csi-rbd, ceph-csi-cephfs, ceph-csi-nfs)' | sort | uniq
```

To inspect the `ImageSet` ConfigMap directly:

```console
kubectl -n $ROOK_OPERATOR_NAMESPACE get configmap rook-csi-operator-image-set-configmap -o yaml
```
