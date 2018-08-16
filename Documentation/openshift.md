---
title: OpenShift
weight: 17
indent: true
---

# OpenShift

[OpenShift](https://www.openshift.com/) adds a number of security and other enhancements to Kubernetes. In particular, [security context constraints](https://blog.openshift.com/understanding-service-accounts-sccs/) allow the cluster admin to define exactly which permissions are allowed to pods running in the cluster. You will need to define those permissions that allow the Rook pods to run. 

## Rook Privileges

To orchestrate the storage platform, Rook requires the following access in the cluster:
- Create `hostPath` volumes, for persistence by the Ceph mon and osd pods
- Run pods in `privileged` mode, for access to `/dev` and `hostPath` volumes
- Host networking for the Rook agent and clusters that require host networking
- Ceph OSDs require host PIDs for communication on the same node

## Security Context Constraints

Before starting the Rook operator or cluster, create the security context constraints needed by the Rook pods. The following yaml is found in `scc.yaml` under `/cluster/examples/kubernetes/ceph`.

```yaml
kind: SecurityContextConstraints
apiVersion: v1
metadata:
  name: rook-ceph
allowPrivilegedContainer: true
allowHostNetwork: true
allowHostDirVolumePlugin: true
priority: 
allowedCapabilities: []
allowHostPorts: false
allowHostPID: true
allowHostIPC: false
readOnlyRootFilesystem: false
requiredDropCapabilities: []
defaultAddCapabilities: []
runAsUser:
  type: RunAsAny
seLinuxContext:
  type: MustRunAs
fsGroup:
  type: MustRunAs
supplementalGroups:
  type: RunAsAny
allowedFlexVolumes: 
  - driver: "rook.io/rook"
volumes:
  - configMap
  - downwardAPI
  - emptyDir
  - flexVolume
  - hostPath
  - persistentVolumeClaim
  - projected
  - secret
users:
  # A user needs to be added for each rook service account. 
  # This assumes running in the default sample "rook-ceph" and "rook-ceph-system" namespaces.
  # If other namespaces or service accounts are configured, they need to be updated here.
  - system:serviceaccount:rook-ceph-system:rook-ceph-system
  - system:serviceaccount:rook-ceph:default
  - system:serviceaccount:rook-ceph:rook-ceph-cluster
```

Important to note is that if you plan on running Rook in namespaces other than the defaults of `rook-ceph-system` and `rook-ceph`, the example scc will need to be modified to accommodate for your namespaces where the Rook pods are running.

To create the scc you will need a privileged account:
```bash
oc login -u system:admin
```

Now create the scc:
```bash
oc create -f scc.yaml
```

## Rook Settings

There are some Rook settings that also need to be adjusted to work in OpenShift.

### Operator Settings
There is an environment variable that needs to be set in the operator spec that will allow Rook to run in OpenShift clusters.
- `ROOK_HOSTPATH_REQUIRES_PRIVILEGED`: Must be set to `true`. Writing to the hostPath is required for the Ceph mon and osd pods. Given the restricted permissions in OpenShift with SELinux, the pod must be running privileged in order to write to the hostPath volume.

```yaml
- name: ROOK_HOSTPATH_REQUIRES_PRIVILEGED
    value: "true"
```

### Cluster Settings
The cluster settings in `cluster.yaml` are largely isolated from the differences in OpenShift. There is perhaps just one to take note of:
- `dataDirHostPath`: Ensure that it points to a valid, writable path on the host systems.


### Object Store Settings
In OpenShift, ports less than 1024 cannot be bound. In the [object store CRD](object.md), ensure the port is modified to meet this requirement.
```yaml
gateway:
    port: 8080
```

You can expose a different port such as `80` by creating a service.

## MiniShift
There is a known issue in MiniShift that does not allow Rook to be tested in some common end-to-end scenarios. Flex drivers are not currently supported, which means that block and file volumes cannot be mounted. See this [tracking issue](https://github.com/minishift/minishift/issues/2387).
