---
title: Custom Images
---

CSI drivers are now managed by the [ceph-csi-operator](https://github.com/ceph/ceph-csi-operator).
The admin configures CSI container images through an `ImageSet` ConfigMap that is
referenced by the `OperatorConfig` CR. By default, Rook's Helm chart creates this
ConfigMap with the latest stable versions. Commonly, there is no need to change
the defaults. For scenarios that require custom images (e.g. downstream releases),
edit the `ImageSet` ConfigMap directly.

```console
kubectl -n $ROOK_OPERATOR_NAMESPACE edit configmap rook-csi-operator-image-set-configmap
```

The default upstream images are included below, which you can change to your desired images.

```yaml
plugin: "quay.io/cephcsi/cephcsi:v3.16.2"
provisioner: "registry.k8s.io/sig-storage/csi-provisioner:v6.1.1"
attacher: "registry.k8s.io/sig-storage/csi-attacher:v4.11.0"
resizer: "registry.k8s.io/sig-storage/csi-resizer:v2.1.0"
snapshotter: "registry.k8s.io/sig-storage/csi-snapshotter:v8.5.0"
registrar: "registry.k8s.io/sig-storage/csi-node-driver-registrar:v2.16.0"
addons: "quay.io/csiaddons/k8s-sidecar:v0.14.0"
```

When using Helm, the images are configured under the `csi` section of `values.yaml`
(e.g. `csi.cephcsi.repository`, `csi.cephcsi.tag`) and are rendered into the
`ImageSet` ConfigMap automatically.

### **Use default images**

If you would like to use the default upstream images, leave the `ImageSet` ConfigMap
unchanged or, for Helm installs, do not override any `csi.*` image values.

### **Verifying updates**

You can use the below command to see the CSI images currently being used in the cluster.
Not all images may be present depending on which CSI features are enabled.

```console
kubectl --namespace rook-ceph get pod -o jsonpath='{range .items[*]}{range .spec.containers[*]}{.image}{"\n"}' -l 'app.kubernetes.io/part-of in (ceph-csi-rbd, ceph-csi-cephfs, ceph-csi-nfs)' | sort | uniq
```

You can also inspect the `ImageSet` ConfigMap directly:

```console
kubectl -n $ROOK_OPERATOR_NAMESPACE get configmap rook-csi-operator-image-set-configmap -o yaml
```
