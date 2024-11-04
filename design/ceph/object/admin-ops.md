---
title: deploy a separate RGW instance to serve admin-ops API
target-version: release-1.16
---

# Feature Name
Separate RGW deployment to serve admin-ops API.

## Summary

RGW provides an [admin-ops API](https://docs.ceph.com/en/latest/radosgw/adminops/) to manage users, usage, quotas, and other sensitive information. This API is hosted on the same port as the object storage APIs (S3 and Swift), meaning that object store users have physical access to the Admin API, which is only secured by authorization controls.

This configuration presents a security risk: obtaining access to admin credentials could allow unauthorized access to the Admin API. A more secure approach would be to hide the Admin API behind a firewall. The most straightforward solution would involve hosting the Admin API on a separate port; however, [this feature](https://tracker.ceph.com/issues/68484) is not yet implemented in RGW.

Currently, RGW offers the [`rgw_enable_apis` option](https://docs.ceph.com/en/reef/radosgw/config-ref/#confval-rgw_enable_apis), which allows disabling the Admin API for a given RGW instance. However, the Admin API cannot be entirely disabled, as Rook relies on it for critical operations.

The proposed solution involves disabling the Admin Ops API on user-facing RGW instances and deploying a separate RGW instance with only the Admin Ops API enabled. This separate deployment would allow configuring a dedicated Kubernetes service and using a separate domain for the Admin API, enhancing overall security.

## Proposal details

Add admin ops RGW instance options to CephObjectStore CRD:

```diff yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStore
metadata:
  name: my-store
spec:
...
+  # [OPTIONAL] Deploys a separate RGW instance to host admin ops API
+  adminGateway:
+    port: 80
+    securePort: 443
+    # Provide secrets with TLS certificates if secure port is used
+    sslCertificateRef:
+    caBundleRef:
+    annotations:
+    labels:
+    # [OPTIONAL] if not set, will be obtained from spec.gateway.resources
+    resources:
+      limits:
+        cpu: "1"
+        memory: "2Gi"
+      requests:
+        cpu: "500m"
+        memory: "1Gi"
# Placement and other configurations will be inheritet from user-facing RGW instance
  gateway:
    # The affinity rules to apply to the rgw deployment.
    placement:
    ...
```

If `spec.extAdminOps.enabled` set to `true`, Rook will remove `admin` from `rgw_enable_apis` option for user-facing `my-store` RGW instances to disable admin admin-ops API on it.
Additionally, Rook will create a separate deployment and service of `my-store-admin` RGW instance with all options and configs inherited from `my-store` except of following:
- `rgw_enable_apis=admin` - admin instance will expose only admin API
- rgw admin instance will use a separate certificate if TLS is enabled.
- separate k8s service will be used
