---
title: Custom Images
---

By default, Rook will deploy the latest stable version of the Ceph CSI driver.
Commonly, there is no need to change this default version that is deployed.
For scenarios that require deploying a custom image (e.g. downstream releases),
the defaults can be overridden with the following settings.

The CSI configuration variables are found in the `rook-ceph-operator-config` ConfigMap.
These settings can also be specified as environment variables on the operator deployment, though
the configmap values will override the env vars if both are specified.

```console
kubectl -n $ROOK_OPERATOR_NAMESPACE edit configmap rook-ceph-operator-config
```

The default upstream images are included below, which you can change to your desired images.

```yaml
ROOK_CSI_CEPH_IMAGE: "quay.io/cephcsi/cephcsi:v3.6.2"
ROOK_CSI_REGISTRAR_IMAGE: "k8s.gcr.io/sig-storage/csi-node-driver-registrar:v2.5.1"
ROOK_CSI_PROVISIONER_IMAGE: "k8s.gcr.io/sig-storage/csi-provisioner:v3.1.0"
ROOK_CSI_ATTACHER_IMAGE: "k8s.gcr.io/sig-storage/csi-attacher:v3.4.0"
ROOK_CSI_RESIZER_IMAGE: "k8s.gcr.io/sig-storage/csi-resizer:v1.4.0"
ROOK_CSI_SNAPSHOTTER_IMAGE: "k8s.gcr.io/sig-storage/csi-snapshotter:v5.0.1"
CSI_VOLUME_REPLICATION_IMAGE: "quay.io/csiaddons/volumereplication-operator:v0.3.0"
ROOK_CSIADDONS_IMAGE: "quay.io/csiaddons/k8s-sidecar:v0.4.0"
```

### **Use default images**

If you would like Rook to use the default upstream images, then you may simply remove all
variables matching `ROOK_CSI_*_IMAGE` from the above ConfigMap and/or the operator deployment.

### **Verifying updates**

You can use the below command to see the CSI images currently being used in the cluster. Note that
not all images (like `volumereplication-operator`) may be present in every cluster depending on
which CSI features are enabled.

```console
kubectl --namespace rook-ceph get pod -o jsonpath='{range .items[*]}{range .spec.containers[*]}{.image}{"\n"}' -l 'app in (csi-rbdplugin,csi-rbdplugin-provisioner,csi-cephfsplugin,csi-cephfsplugin-provisioner)' | sort | uniq
```

The default images can also be found with each release in the [images list](https://github.com/rook/rook/blob/master/deploy/examples/images.txt)
