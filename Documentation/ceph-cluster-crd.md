# API Docs

This Document documents the types introduced by the Rook Ceph Operator to be consumed by users.

> **NOTE**: This document is generated from code comments. When contributing a change to this document please do so by changing the code comments.

## Table of Contents
* [Table of Contents](#table-of-contents)
* [CephCluster](#cephcluster)
* [CephClusterList](#cephclusterlist)
* [CephHealthMessage](#cephhealthmessage)
* [CephStatus](#cephstatus)
* [CephVersionSpec](#cephversionspec)
* [ClusterSpec](#clusterspec)
* [ClusterStatus](#clusterstatus)
* [DashboardSpec](#dashboardspec)
* [DisruptionManagementSpec](#disruptionmanagementspec)
* [ExternalSpec](#externalspec)
* [MgrSpec](#mgrspec)
* [Module](#module)
* [MonSpec](#monspec)
* [MonitoringSpec](#monitoringspec)
* [NetworkSpec](#networkspec)
* [RBDMirroringSpec](#rbdmirroringspec)

## CephCluster



| Field    | Description | Scheme                                                                                                       | Required |
| -------- | ----------- | ------------------------------------------------------------------------------------------------------------ | -------- |
| metadata |             | [metav1.ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.11/#objectmeta-v1-meta) | true     |
| spec     |             | [ClusterSpec](#clusterspec)                                                                                  | true     |
| status   |             | [ClusterStatus](#clusterstatus)                                                                              | false    |

[Back to TOC](#table-of-contents)

## CephClusterList



| Field    | Description | Scheme                                                                                                   | Required |
| -------- | ----------- | -------------------------------------------------------------------------------------------------------- | -------- |
| metadata |             | [metav1.ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.11/#listmeta-v1-meta) | true     |
| items    |             | [][CephCluster](#cephcluster)                                                                            | true     |

[Back to TOC](#table-of-contents)

## CephHealthMessage



| Field    | Description | Scheme | Required |
| -------- | ----------- | ------ | -------- |
| severity |             | string | true     |
| message  |             | string | true     |

[Back to TOC](#table-of-contents)

## CephStatus



| Field          | Description | Scheme                                             | Required |
| -------------- | ----------- | -------------------------------------------------- | -------- |
| health         |             | string                                             | false    |
| details        |             | map[string][CephHealthMessage](#cephhealthmessage) | false    |
| lastChecked    |             | string                                             | false    |
| lastChanged    |             | string                                             | false    |
| previousHealth |             | string                                             | false    |

[Back to TOC](#table-of-contents)

## CephVersionSpec

VersionSpec represents the settings for the Ceph version that Rook is orchestrating.

| Field            | Description                                                                                                  | Scheme | Required |
| ---------------- | ------------------------------------------------------------------------------------------------------------ | ------ | -------- |
| image            | Image is the container image used to launch the ceph daemons, such as ceph/ceph:v13.2.6 or ceph/ceph:v14.2.2 | string | false    |
| allowUnsupported | Whether to allow unsupported versions (do not set to true in production)                                     | bool   | false    |

[Back to TOC](#table-of-contents)

## ClusterSpec



| Field                          | Description                                                                                                                                                 | Scheme                                                | Required |
| ------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------- | -------- |
| cephVersion                    | The version information that instructs Rook to orchestrate a particular version of Ceph.                                                                    | [CephVersionSpec](#cephversionspec)                   | false    |
| storage                        | A spec for available storage in the cluster and how it should be used                                                                                       | rook.StorageScopeSpec                                 | false    |
| annotations                    | The annotations-related configuration to add/set on each Pod related object.                                                                                | rook.AnnotationsSpec                                  | false    |
| placement                      | The placement-related configuration to pass to kubernetes (affinity, node selector, tolerations).                                                           | rook.PlacementSpec                                    | false    |
| network                        | Network related configuration                                                                                                                               | [NetworkSpec](#networkspec)                           | false    |
| resources                      | Resources set resource requests and limits                                                                                                                  | rook.ResourceSpec                                     | false    |
| dataDirHostPath                | The path on the host where config and data can be persisted.                                                                                                | string                                                | false    |
| skipUpgradeChecks              | SkipUpgradeChecks defines if an upgrade should be forced even if one of the check fails                                                                     | bool                                                  | false    |
| disruptionManagement           | A spec for configuring disruption management.                                                                                                               | [DisruptionManagementSpec](#disruptionmanagementspec) | false    |
| mon                            | A spec for mon related options                                                                                                                              | [MonSpec](#monspec)                                   | false    |
| rbdMirroring                   | A spec for rbd mirroring                                                                                                                                    | [RBDMirroringSpec](#rbdmirroringspec)                 | true     |
| dashboard                      | Dashboard settings                                                                                                                                          | [DashboardSpec](#dashboardspec)                       | false    |
| monitoring                     | Prometheus based Monitoring settings                                                                                                                        | [MonitoringSpec](#monitoringspec)                     | false    |
| external                       | Whether the Ceph Cluster is running external to this Kubernetes cluster mon, mgr, osd, mds, and discover daemons will not be created for external clusters. | [ExternalSpec](#externalspec)                         | true     |
| mgr                            | A spec for mgr related options                                                                                                                              | [MgrSpec](#mgrspec)                                   | false    |
| removeOSDsIfOutAndSafeToRemove | Remove the OSD that is out and safe to remove only if this option is true                                                                                   | bool                                                  | true     |

[Back to TOC](#table-of-contents)

## ClusterStatus



| Field   | Description | Scheme                     | Required |
| ------- | ----------- | -------------------------- | -------- |
| state   |             | ClusterState               | false    |
| message |             | string                     | false    |
| ceph    |             | *[CephStatus](#cephstatus) | false    |

[Back to TOC](#table-of-contents)

## DashboardSpec

DashboardSpec represents the settings for the Ceph dashboard

| Field     | Description                                                     | Scheme | Required |
| --------- | --------------------------------------------------------------- | ------ | -------- |
| enabled   | Whether to enable the dashboard                                 | bool   | false    |
| urlPrefix | A prefix for all URLs to use the dashboard with a reverse proxy | string | false    |
| port      | The dashboard webserver port                                    | int    | false    |
| ssl       | Whether SSL should be used                                      | bool   | false    |

[Back to TOC](#table-of-contents)

## DisruptionManagementSpec

DisruptionManagementSpec configures management of daemon disruptions

| Field                            | Description                                                                                                                                                                       | Scheme        | Required |
| -------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------- | -------- |
| managePodBudgets                 | This enables management of poddisruptionbudgets                                                                                                                                   | bool          | false    |
| osdMaintenanceTimeout            | OSDMaintenanceTimeout sets how many additional minutes the DOWN/OUT interval is for drained failure domains it only works if managePodBudgetss is true. the default is 30 minutes | time.Duration | false    |
| manageMachineDisruptionBudgets   | This enables management of machinedisruptionbudgets                                                                                                                               | bool          | false    |
| machineDisruptionBudgetNamespace | Namespace to look for MDBs by the machineDisruptionBudgetController                                                                                                               | string        | false    |

[Back to TOC](#table-of-contents)

## ExternalSpec

ExternalSpec represents the options supported by an external cluster

| Field  | Description | Scheme | Required |
| ------ | ----------- | ------ | -------- |
| enable |             | bool   | true     |

[Back to TOC](#table-of-contents)

## MgrSpec

MgrSpec represents options to configure a ceph mgr

| Field   | Description | Scheme              | Required |
| ------- | ----------- | ------------------- | -------- |
| modules |             | [][Module](#module) | false    |

[Back to TOC](#table-of-contents)

## Module

Module represents mgr modules that the user wants to enable or disable

| Field   | Description | Scheme | Required |
| ------- | ----------- | ------ | -------- |
| name    |             | string | false    |
| enabled |             | bool   | true     |

[Back to TOC](#table-of-contents)

## MonSpec



| Field                | Description | Scheme                                                                                                                          | Required |
| -------------------- | ----------- | ------------------------------------------------------------------------------------------------------------------------------- | -------- |
| count                |             | int                                                                                                                             | false    |
| allowMultiplePerNode |             | bool                                                                                                                            | false    |
| volumeClaimTemplate  |             | *[v1.PersistentVolumeClaim](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.11/#persistentvolumeclaim-v1-core) | false    |

[Back to TOC](#table-of-contents)

## MonitoringSpec

MonitoringSpec represents the settings for Prometheus based Ceph monitoring

| Field          | Description                                                                                                                      | Scheme | Required |
| -------------- | -------------------------------------------------------------------------------------------------------------------------------- | ------ | -------- |
| enabled        | Whether to create the prometheus rules for the ceph cluster. If true, the prometheus types must exist or the creation will fail. | bool   | false    |
| rulesNamespace | The namespace where the prometheus rules and alerts should be created. If empty, the same namespace as the cluster will be used. | string | false    |

[Back to TOC](#table-of-contents)

## NetworkSpec

NetworkSpec for Ceph includes backward compatibility code

| Field       | Description                        | Scheme | Required |
| ----------- | ---------------------------------- | ------ | -------- |
| hostNetwork | HostNetwork to enable host network | bool   | true     |

[Back to TOC](#table-of-contents)

## RBDMirroringSpec



| Field   | Description | Scheme | Required |
| ------- | ----------- | ------ | -------- |
| workers |             | int    | true     |

[Back to TOC](#table-of-contents)
