---
title: Custom Images
---

CSI drivers are now managed by the [ceph-csi-operator](https://github.com/ceph/ceph-csi-operator).

Default CSI container images are built into the ceph-csi-operator. Rook ships
the `rook-csi-operator-image-set-configmap` configmap with empty values so the
ceph-csi-operator uses its built-in defaults. If needed, the defaults can be overridden with custom images.

### **Override the default images**

To use custom images (e.g. downstream releases or air-gapped environments),
set the desired values in the configmap. Only non-empty values override defaults.

**Manifest installs:**

```console
kubectl -n $ROOK_OPERATOR_NAMESPACE edit configmap rook-csi-operator-image-set-configmap
```

**Helm installs:** set the image values under the `csi` section of the `rook-ceph` chart.
For example, to customize the ceph-csi image:

```yaml
csi:
  cephcsi:
    repository: quay.io/cephcsi/cephcsi
    tag: v3.18.0
```

The chart renders these values into the `rook-csi-operator-image-set-configmap`.
By default, the `repository` and `tag` are empty, which will indicate to the ceph-csi-operator to configure the default images.

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
