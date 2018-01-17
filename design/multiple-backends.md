# Multiple Backends

Rook is a storage platform for Kubernetes clusters and is designed to hide the complexity of the underlying platform
so that the client does not need to understand the details. When there are too many configuration options,
it becomes a burden for the admin and is too easy to misconfigure. Rook exposes storage concepts and configurations that are
common across storage technologies to simplify deploying storage in Kubernetes.

Still, there is a need to expose some of the details of a storage platform. Otherwise, characteristics of the platform could
be lost or watered down such as high availability, high performance, durability, and other critical attributes.
- Administrators need the ability to tune and configure the storage
- Storage consumers may need settings for a specific storage platform

Rook currently implements Ceph as the storage backend. In the future, other backends can also be added for block, file, and object
storage as the following design principles are followed.

## Custom Resource Definitions
The CRDs are the public interface for configuring Rook storage in the cluster. The majority of settings will apply to multiple 
backends, but some settings will only apply to a specific backend. All settings settings specific to a backend will be found
under a property with that backend's name.

### General Settings
Settings that apply to all backends in the Cluster CRD include the following. The first setting indicates which backend is in use
for this configuration. 
```yaml
spec:
  backend: ceph
  dataDirHostPath: /var/lib/rook
  hostNetwork: false
  storage: 
    useAllNodes: false
    useAllDevices: true
    nodes:
    - nodeA
    - nodeB
```

The `backend` setting cannot be changed after the cluster is configured.

### Ceph Settings
Settings specific to Ceph will be found under the `ceph` property at various locations in the CRD.

The overall `mon`, `mgr`, and `osd` settings are found under the top-level `ceph` property. Here are a few simple
settings as an example:
```yaml
  ceph:
    mon:
      count: 3
    osd:
      storeType: bluestore
    mgr:
      resources: # resource limits can be set
```

Configuring storage directories and devices will also allow configuration of backend-specific properties.
In this example, a node can specify `osd` properties specific to the devices on that node. Any property that could be set
at the cluster level under the `ceph`, `osd` node can be overridden at the node level, such as `storeType`, `databaseSizeMB`,
`journalSizeMB`, or `resources`. The only setting that cannot be overridden is the `placement` since that decision must be made
centrally by the operator.
```yaml
      nodes:
        - name: "172.17.4.101"
          devices:
          - name: "sdb"
          - name: "sdc"
          osd:  # any settings found under the overall cluster ceph osd settings can be overridden on a per-node basis here
            storeType: bluestore
```


## Source tree
To follow the pattern in the CRDs, some Rook code will apply to all backends, and some will apply to specific backends.
Most code is expected to apply to all backends so it is safe to assume that by default code will apply generally.
Code specific to a backend will be placed under a folder with its name. This means that currently the subfolder `ceph`
is found in a number of places. Code in the `ceph` subfolders will implement code specific to the orchestration of 
the `mon`, `osd`, `mgr`, `rgw`, and `mds` orchestrators and daemons.

All Rook code is written in `Go` and all Rook code, both general and specific to backends, is compiled into a 
single `Go` binary. 

## Deployment
Rook is deployed to Kubernetes in a container. The Rook Operator deployment is created first, followed by an
orchestration of other containers to run daemons depending on the backend. The backend that will be 
used will depend on the docker image that is running. The operator running in the Ceph image will 
know to create a Ceph backend. 

### CONFLICTING CONCLUSION 
The decision to contain one backend per docker image leads us to a significant conclusion:
- A separate operator must be running for each backend supported in the cluster
- A separate CRD must be defined for each backend since only one operator can watch each CRD type

This defeats the goal of using the same CRD for multiple backends. To support multiple backends from the same
CRD, you would need to include all backends in the same docker image.

### Docker Image
When the Rook operator or daemons run, Rook will execute commands to configure the specific backend. 
For example, the operator will execute commands to detect whether Ceph mons are in quorum, and the `mon`
daemon will shell out to the `ceph-mon` daemon to run the Ceph monitor. The tools that are required 
to run a backend must be found in the docker image. 

A docker image is created for each backend.
- A single image for all backends could get extremely bloated
- Only one backend can be run in a cluster

The image is published to dockerhub.com under `rook/rook-ceph`. If Gluster were added as a backend, it
would be published to `rook/rook-gluster`.

#### Ceph
The Ceph docker image will contain the `rook` Go binary as well as the Ceph tools such as 
`ceph`, `ceph-authtool`, `ceph-mon`, `ceph-mgr`, `ceph-osd`, and a number of others that are needed to configure
and run all of the Ceph daemons.

### Entrypoints
Rook implements the entrypoint to both the operator and the daemons. Whether Rook starts the operator or a daemon
depends on the command line arguments.

To run the operator in the Rook container:
```
rook operator
```

To run a Ceph mon:
```
rook mon
```
