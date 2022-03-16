# v1.9 Pending Release Notes

## Breaking Changes

* The MDS liveness and startup probes are now configured by the CephFilesystem CR instead of the
  CephCluster CR. To apply the MDS probes, they need to be specified in the CephFilesystem CR. See the
  [CephFilesystem doc](Documentation/ceph-filesystem-crd.md#metadata-server-settings) for more details. See #9550
* In the Helm charts, all Ceph components now have default values for the pod resources. The values
  can be modified or removed in values.yaml depending on cluster requirements.
* Prometheus rules are installed by the Helm chart. If you were relying on the cephcluster setting
  `monitoring.enabled` to create the prometheus rules, they now  need to be enabled by setting
  `monitoring.createPrometheusRules` in the Helm chart values.
* Remove the obsolete cross build container, now unused by the CI

## Features

* The number of mgr daemons for example clusters is increased to 2 from 1, resulting in a standby
  mgr daemon. If the active mgr goes down, Ceph will update the passive mgr to be active, and rook
  will update all the services with the label app=rook-ceph-mgr to direct traffic to the new active
  mgr.
* Network encryption is configurable with settings in the CephCluster CR. Requires the 5.11 kernel or newer.
* Network compression is configurable with settings in the CephCluster CR. Requires Ceph Quincy (v17) or newer.
* Added support for custom ceph.conf for csi pods. See #9567
* Added and updated many Ceph prometheus rules as recommended the main Ceph project.
* Added service account rook-ceph-rgw for the RGW pods.
* Added new RadosNamespace resource: create rados namespaces in a CephBlockPool. See #9733
