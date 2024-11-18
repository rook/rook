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

If `spec.adminGateway` is specified, Rook will remove `admin` from `rgw_enable_apis` option for user-facing `my-store` RGW instances to disable admin admin-ops API on it.
Additionally, Rook will create a separate deployment and service of `my-store-admin` RGW instance with all options and configs inherited from `my-store` except of following:
- `rgw_enable_apis=admin` - admin instance will expose only admin API
- rgw admin instance will use a separate certificate if TLS is enabled.
- separate k8s service will be used

## Alternative proposal

Alternative approach is to have a separate `CephObjectStore` CR for each RGW instance. It involves more changes to `CephObjectStore` architecture but it will allow cover more use-cases like:
- separate RGW instance to host SWIFT and S3 APIs on different domains
- separate RGW instance to run garbage collection

To run different RGW daemons for the same storage but with different configs these daemons should refer to the same `realm`, `zone`, `zonegroup` and use the same pools.

The following changes are required to support flexible RGW deployments with Rook:
1. Set custom `realm`, `zone`, `zonegroup` names to `CephObjectStore` CR. Currencly, [Rook is using CephObjectStore name as realm/zone/zonegroup for single-site setup](https://github.com/rook/rook/blob/master/pkg/operator/ceph/object/controller.go#L510-L513). In this case it is not possible to create multiple `CephObjectStore` CRs with different configs (e.g. `rgw_enable_apis`).
2. Set link to admin RGW instance. If admin ops api is disable for given `CephObjectStore` Rook should be able to figure out admin endpoint address.
3. Expose more instance-level `[client.radosgw.{instance-name}]` configurations.
4. Adjust Rook pool `metadataPool`, `dataPool`, `sharedPools` creation logic when `realm`, `zone`, `zonegroup` are set to use the same pools across multiple `CephObjectStore` with same zones.

### Implementation details for multi-CR ObjectStorage

Changes to `CephObjectStore` CRD:

```diff yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStore
metadata:
  name: my-store
spec:
...
+  # [OPTIONAL] Required if admin api is disabled for given RGW instance and served by different CephObjectStore
+  adminOpsInstanceName: "my-store-admin"
+  # [OPTIONAL] 
+  multiInstance:
+    # [REQUIRED] - main rgw instance. This instance will inherit all configs and pools from it
+    mainInstanceName: "my-store"
+    [OPTIONAL] - rgw config override in ceph config [client.radosgw.{instance-name}]
+    configOverride:
+      rgw_enable_apis: "swift" 
+      <rgw config param name>: "<string value>" 
+    # [OPTIONAL] overrides spec.gateway of main instance
+    gatewayOverride:
+      port: 80
+      securePort: 443
+      # Provide secrets with TLS certificates if secure port is used
+      sslCertificateRef:
+      caBundleRef:
+      annotations:
+      labels:
+      # [OPTIONAL] if not set, will be obtained from spec.gateway.resources
+      resources:
+        limits:
+          cpu: "1"
+          memory: "2Gi"
+        requests:
+          cpu: "500m"
+          memory: "1Gi"
```

Usage example:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStore
metadata:
  name: my-store
spec:
  metadataPool:
    replicated:
      size: 1
  dataPool:
    replicated:
      size: 1
  gateway:
    port: 80
    instances: 1
  config:
    rgw_enable_apis: "s3, s3website, sts, iam, notifications" 
    rgw_enable_apis: "s3, s3website, swift, swift_auth, admin, sts, iam, notifications" 
  adminOpsInstanceName: "my-store-admin"
---
apiVersion: ceph.rook.io/v1
kind: CephObjectStore
metadata:
  name: my-store-admin
spec:
  multiInstance:
    mainInstanceName: my-store
    configOverride:
      rgw_enable_apis: "admin" 
    gatewayOverride:
      securePort: 443
      sslCertificateRef: "my-cert"
      caBundleRef: "my-ca-bundle"
---
apiVersion: ceph.rook.io/v1
kind: CephObjectStore
metadata:
  name: my-store-swift
spec:
  adminOpsInstanceName: "my-store-admin"
  multiInstance:
    mainInstanceName: my-store
    configOverride:
      rgw_enable_apis: "swift, swift_auth" 
    gatewayOverride:
      securePort: 443
      sslCertificateRef: "my-cert"
      caBundleRef: "my-ca-bundle"
```

Resulted Ceph config:

```shell
% ceph config dump
WHO                           MASK  LEVEL     OPTION                    VALUE                                   RO
client.rgw.my.store.a               advanced  rgw_enable_apis           s3, s3website, sts, iam, notifications  *
client.rgw.my.store.a               advanced  rgw_zone                  my-store                                *
client.rgw.my.store.a               advanced  rgw_zonegroup             my-store                                *
client.rgw.my.store.a.admin         advanced  rgw_enable_apis           admin                                   *
client.rgw.my.store.a.swift         advanced  rgw_enable_apis           swift, swift_auth                       *
```

Resulted k8s services:

```shell
% kubectl get svc
NAME                           TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)             AGE
rook-ceph-rgw-my-store         ClusterIP   10.106.98.25     <none>        80/TCP              66m
rook-ceph-rgw-my-store-admin   ClusterIP   10.106.98.25     <none>        443/TCP             66m
rook-ceph-rgw-my-store-swift   ClusterIP   10.106.98.25     <none>        80/TCP              66m
```
