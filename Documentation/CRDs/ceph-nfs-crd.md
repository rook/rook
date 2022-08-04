---
title: NFS Server CRD
---

Rook allows exporting NFS shares of a CephFilesystem or CephObjectStore through the CephNFS custom
resource definition.

## Example

```yaml
apiVersion: ceph.rook.io/v1
kind: CephNFS
metadata:
  name: my-nfs
  namespace: rook-ceph
spec:
  # Settings for the NFS server
  server:
    active: 1

    placement:
      nodeAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          nodeSelectorTerms:
          - matchExpressions:
            - key: role
              operator: In
              values:
              - nfs-node
      topologySpreadConstraints:
      tolerations:
      - key: nfs-node
        operator: Exists
      podAffinity:
      podAntiAffinity:

    annotations:
      my-annotation: something

    labels:
      my-label: something

    resources:
      limits:
        cpu: "3"
        memory: "8Gi"
      requests:
        cpu: "3"
        memory: "8Gi"

    priorityClassName: ""

    logLevel: NIV_INFO

  security:
    sssd:
      sidecar:
        image: registry.access.redhat.com/rhel7/sssd:latest

        sssdConfigFile:
          configMap:
            name: my-nfs-sssd-config

        debugLevel: 0
```

## NFS Settings

### Server

The `server` spec sets configuration for Rook-created NFS-Ganesha server pods.

* `active`: The number of active NFS servers. Rook supports creating more than one active NFS
  server, but cannot guarantee high availability. For values greater than 1, see the
  [known issue](#serveractive-count-greater-than-1) below.
* `placement`: Kubernetes placement restrictions to apply to NFS server Pod(s). This is similar to
  placement defined for daemons configured by the
  [CephCluster CRD](https://github.com/rook/rook/blob/master/deploy/examples/cluster.yaml).
* `annotations`: Kubernetes annotations to apply to NFS server Pod(s)
* `labels`: Kubernetes labels to apply to NFS server Pod(s)
* `resources`: Kubernetes resource requests and limits to set on NFS server containers
* `priorityClassName`: Set priority class name for the NFS server Pod(s)
* `logLevel`: The log level that NFS-Ganesha servers should output.</br>
  Default value: NIV_INFO</br>
  Supported values: NIV_NULL | NIV_FATAL | NIV_MAJ | NIV_CRIT | NIV_WARN | NIV_EVENT | NIV_INFO | NIV_DEBUG | NIV_MID_DEBUG | NIV_FULL_DEBUG | NB_LOG_LEVEL

### Security

The `security` spec sets security configuration for the NFS cluster.

* `sssd`: SSSD enables integration with System Security Services Daemon (SSSD). See also:
  [ID mapping via SSSD](../Storage-Configuration/NFS/nfs-security.md#id-mapping-via-sssd).
  * `sidecar`: Specifying this configuration tells Rook to run SSSD in a sidecar alongside the NFS
    server in each NFS pod.
    * `image`: defines the container image that should be used for the SSSD sidecar.
    * `sssdConfigFile`: defines where the SSSD configuration should be sourced from. The
      config file will be placed into `/etc/sssd/sssd.conf`. If this is left empty, Rook will not
      add the file. This allows you to inject your own, for example by using
      [Vault's agent injector](https://www.vaultproject.io/docs/platform/k8s/injector).
      * `volumeSource`: this is a standard Kubernetes
        [VolumeSource](https://pkg.go.dev/k8s.io/api/core/v1#VolumeSource) like what is normally
        used to configure Volumes for a Pod. For example, a ConfigMap, Secret, or HostPath.
        There are two requirements for the source's content:
        1. The config file must be mountable via `subPath: sssd.conf`. For example, in a ConfigMap,
           the data item must be named `sssd.conf`, or `items` must be defined to select the key and
           give it path `sssd.conf`. A HostPath directory must have the `sssd.conf` file.
        2. The volume or config file must have mode 0600.
    * `debugLevel`: sets the debug level for SSSD. If unset or `0`, Rook does nothing. Otherwise,
      this may be a value between 1 and 10. See the
      [SSSD docs](https://sssd.io/troubleshooting/basics.html#sssd-debug-logs) for more info.


## Scaling the active server count

It is possible to scale the size of the cluster up or down by modifying the `spec.server.active`
field. Scaling the cluster size up can be done at will. Once the new server comes up, clients can be
assigned to it immediately.

The CRD always eliminates the highest index servers first, in reverse order from how they were
started. Scaling down the cluster requires that clients be migrated from servers that will be
eliminated to others. That process is currently a manual one and should be performed before reducing
the size of the cluster.

!!! warning
    See the [known issue](#serveractive-count-greater-than-1) below about setting this
    value greater than one.


## Known issues

### server.active count greater than 1

* Active-active scale out does not work well with the NFS protocol. If one NFS server in a cluster
  is offline, other servers may block client requests until the offline server returns, which may
  not always happen due to the Kubernetes scheduler.
  * Workaround: It is safest to run only a single NFS server, but we do not limit this if it
    benefits your use case.

### Ceph v17.2.1

* Ceph NFS management with the Rook mgr module enabled has a breaking regression with the Ceph
  Quincy v17.2.1 release.
  * Workaround: Leave Ceph's Rook orchestrator mgr module disabled. If you have enabled it, you must
    disable it using the snippet below from the toolbox.

    ```console
    ceph orch set backend ""
    ceph mgr module disable rook
    ```
