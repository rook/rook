# Multiple Storage Types Support

## Introduction

Rook is designed to provide orchestration and management for distributed storage systems to run in cloud-native environments, however only [Ceph](https://ceph.com/) is currently supported.
Rook could be beneficial to a wider audience if support for orchestrating more storage backends was implemented.
For instance, in addition to the existing support for Ceph, we should consider support for [Minio](https://www.minio.io/) as the next storage backend supported by Rook.
Minio is a popular distributed object storage solution that could benefit from a custom controller to orchestrate, configure and manage it in cloud-native environments.

This design document aims to describe how this can be accomplished through common storage abstractions and custom controllers for more types of storage providers.

## Goals

* To design and describe how Rook will support multiple storage backends
* Consider options and recommend architecture for hosting multiple controllers/operators
* Provide basic guidance on how new storage controllers would integrate with Rook
* Define common abstractions and types across storage providers

## Resources

* [Custom Resources in Kubernetes](https://kubernetes.io/docs/concepts/api-extension/custom-resources/)
* [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
* [Use CRDs Whenever Possible](https://docs.google.com/presentation/d/1IiKOIBbw7oaD4uZ-kNE-mA3cliFjx9FuFLd7pj-dj1Y/edit#slide=id.p)
* [Writing controllers dev guide](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-api-machinery/controllers.md)
* [A Deep Drive Into Kubernetes Controllers](https://engineering.bitnami.com/articles/a-deep-dive-into-kubernetes-controllers.html)
* [Metacontroller](https://github.com/metacontroller/metacontroller): Kubernetes extension that enables lightweight lambda (functional) controllers

## Design

### Kubernetes Extension Options

To provide a native experience in Kubernetes, Rook has so far defined new storage types with [Custom Resource Definitions (CRDs)](https://kubernetes.io/docs/concepts/api-extension/custom-resources/#customresourcedefinitions)
and implemented the [operator pattern](https://coreos.com/blog/introducing-operators.html) to manage the instances of those types that users create.
When deciding how to expand the experience provided by Rook, we should reevaluate the most current options for extending Kubernetes in order to be confident in our architecture.
This is especially important because changing the architecture later on will be more difficult the more storage types we have integrated into Rook.

#### API server aggregation

[API server aggregation](https://kubernetes.io/docs/concepts/api-extension/custom-resources/#api-server-aggregation) is the most feature rich and complete extension option for Kubernetes, but it is also the most complicated to deploy and manage.
Basically, it allows you to extend the Kubernetes API with your own API server that behaves just like the core API server does.
This approach offers a complete and powerful feature set such as rich validation, API versioning, custom business logic, etc.
However, using an extension apiserver has some disruptive drawbacks:

* etcd must also be deployed for storage of its API objects, increasing the complexity and adding another point of failure.  This can be avoided with using a CRD to store its API objects but that is awkward and exposes internal storage to the user.
* development cost would be significant to get the deployment working reliably in supported Rook environments and to port our existing CRDs to extension types.
* breaking change for Rook users with no clear migration path, which would be very disruptive to our current user base.

#### CustomResourceDefinitions (CRDS)

CRDs are what Rook is already using to extend Kubernetes.  They are a limited extension mechanism that allows the definition of custom types, but lacks the rich features of API aggregation.  For example, validation of a users CRD is only at the schema level and simple property value checks that are available via the [OpenAPI v3 schema](https://github.com/OAI/OpenAPI-Specification/blob/master/versions/3.0.0.md#schemaObject).
Also, there is currently no versioning (conversion) support.
However, CRDs are being actively supported by the community and more features are being added to CRDs going forward (e.g., a [versioning proposal](https://github.com/kubernetes/features/issues/544)).
Finally, CRDs are not a breaking change for Rook users since they are already in use today.

### Controller Options

The controllers are the entities that will perform orchestration and deployment tasks to ensure the users desired state is made a reality within their cluster.
There are a few options for deploying and hosting the controllers that will perform this work, as explained in the following sections.

#### In-proc Controllers

Multiple controllers could be deployed within a single process.
For instance, Rook could run one controller per domain of expertise, e.g., ceph-controller, minio-controller, etc.
Controllers would all watch the same custom resource types via a `SharedInformer` and respond to events via a `WorkQueue` for efficiency and reduced burden on core apiserver.
Even though all controllers are watching, only the applicable controller responds to and handles an event.
For example, the user runs `kubectl create cluster.yaml` to create a `cluster.rook.io` instance that has Ceph specific properties.
All controllers will receive the created event via the `SharedInformer`, but only the Ceph controller will queue and handle it.
We can consider only loading the controllers the user specifically asks for, perhaps via an environment variable in `operator.yaml`.

Note that this architecture can be used with either API aggregation or CRDs.

##### PROS

* Slightly easier developer burden for new storage backends since there is no need to create a new deployment to host their controller.
* Less resource burden on K8s cluster since watchers/caches are being shared.
* Easier migration path to API aggregation in the future, if CRDs usage is continued now.

##### CONS

* All controllers must use same base image since they are all running in the same process.
* If a controller needs to access a backend specific tool then they will have to schedule a job that invokes the appropriate image.  This is similar to exec’ing a new process but at the cluster level.
  * Note this only applies to the controller, not to the backend's daemons.  Those daemons will be running a backend specific image and can directly `exec` to their tools.

#### Operator pods

Each storage backend could have their own operator pod that hosts only that backend's controller, e.g., `ceph-operator.yaml`, `minio-operator.yaml`, etc.
The user would decide which operators to deploy based on what storage they want to use.
Each operator pod would watch the same custom resource types with their own individual watchers.

##### PROS

* Each operator can use their own image, meaning they have direct access (through `exec`) to their backend specific tools.
* Runtime isolation, one flaky backend does not impact or cause downtime for other backends.
* Privilege isolation, each backend could define their own service account and RBAC that is scoped to just their needs.

##### CONS

* More difficult migration path to API aggregation in the future.
* Potentially more resource usage and load on Kubernetes API since watchers will not be shared, but this is likely not an issue since users will deploy only the operator they need.
* Slightly more developer burden as all backends have to write their own deployment/host to manage their individual pod.

#### Metacontroller

For storage backends that fit the patterns that [Metacontroller](https://github.com/GoogleCloudPlatform/metacontroller) supports (`CompositeController` and `DecoratorController`), this could be an option to incorporate into Rook.
Basically, a storage backend defines their custom types and the parent/child relationships between them.
The metacontroller handles all the K8s API interactions and regularly calls into storage backend defined "hooks".
The storage backend is given JSON representing the current state in K8s types and then returns JSON defining in K8s types what the desired state should be.
The metacontroller then makes that desired state a reality via the K8s API.
This pattern does allow for fairly complicated stateful apps (e.g. [Vitess](https://github.com/GoogleCloudPlatform/metacontroller/tree/master/examples/vitess)) that have well defined parent/children hierarchies, and can allow for the storage backend operator to perform "imperative" operations to manipulate cluster state by launching Jobs.

### Recommendation

**CRDs with an operator pod per backend**: This will not be a breaking change for our current users and does not come with the deployment complexity of API aggregation.
It would provide each backend's operator the freedom to easily invoke their own tools that are packaged in their own specific image, avoiding unnecessary image bloat.
It also provides both resource and privilege isolation for each backend.
We would accept the burden of limited CRD functionality (which is improving in the future though).

We should also consider integrating metacontroller's functionality for storage backends that are compatible and can benefit from its patterns.
Each storage backend can make this decision independently.

### Custom Types

Custom resources in Kubernetes use the following naming and versioning convention:

* Group: A collection of several related types that are versioned together and can be enabled/disabled as a unit in the API (e.g., `ceph.rook.io`)
* Version: The API version of the group (e.g., `v1alpha1`)
* Kind: The specific type within the API group (e.g., `cluster`)

Putting this together with an example, the `cluster` kind from the `ceph.rook.io` API group with a version of `v1alpha1` would be referred to in full as `cluster.ceph.rook.io/v1alpha1`.

#### Resource Design Properties

Versioning of custom resources defined by Rook is important, and we should carefully consider a design that allows resources to be versioned in a sensible way.
Let's first review some properties of Rook's resources and versioning scheme that are desirable and we should aim to satisfy with this design:

1. Storage backends should be independently versioned, so their maturity can be properly conveyed.  For example, the initial implementation of a new storage backend should not be forced to start at a stable `v1` version.
1. CRDs should mostly be defined only for resources that can be instantiated.  If the user can't create an instance of the resource, then it's likely better off as a `*Spec` type that can potentially be reused across many types.
1. Reuse of common types is a very good thing since it unifies the experience across storage types and it reduces the duplication of effort and code.  Commonality and sharing of types and implementations is important and is another way Rook provides value to the greater storage community beyond the operators that it implements.

Note that it is **not a goal** to define a common abstraction that applies to the top level storage backends themselves, for instance a single `Cluster` type that covers both Ceph and Minio.
We should not be trying to force each backend to look the same to storage admins, but instead we should focus on providing the common abstractions and implementations that storage providers can build on top of.
This idea will become more clear in the following sections of this document.

#### Proposed Custom Resources

With the intent for Rook's resources to fulfill the desirable properties mentioned above, we propose the following API groups:

* `rook.io`: common abstractions and implementations, in the form of `*Spec` types, that have use across multiple storage backends and types.  For example, storage, network information, placement, and resource usage.
* `ceph.rook.io`: Ceph specific `Cluster` CRD type that the user can instantiate to have the Ceph controller deploy a Ceph cluster or Ceph resources for them. This Ceph specific API group allows Ceph types to be versioned independently.
* `nexenta.rook.io`: Similar, but for Nexenta.

With this approach, the user experience to create a cluster would look like the following in `yaml`, where they are declaring and configuring a Ceph specific CRD type (from the `ceph.rook.io` API group), but with many common `*Spec` types that provide configuration and logic that is reusable across storage providers.

`ceph-cluster.yaml`:

```yaml
apiVersion: ceph.rook.io/v1
kind: Cluster
spec:
  mon:
    count: 3
    allowMultiplePerNode: false
  network:
  placement:
  resources:
  storage:
    deviceFilter: "^sd."
    config:
      storeType: "bluestore"
      databaseSizeMB: "1024"
```

Our `golang` strongly typed definitions would look like the following, where the Ceph specific `Cluster` CRD type has common `*Spec` fields.

`types.go`:

```go
package v1alpha1 // "github.com/rook/rook/pkg/apis/ceph.rook.io/v1alpha1"

import (
  metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Cluster struct {
  metav1.TypeMeta   `json:",inline"`
  metav1.ObjectMeta `json:"metadata"`
  Spec              ClusterSpec   `json:"spec"`
  Status            ClusterStatus `json:"status"`
}

type ClusterSpec struct {
  Storage     rook.StorageScopeSpec `json:"storage"`
  Network     rook.NetworkSpec      `json:"network"`
  Placement   rook.PlacementSpec    `json:"placement"`
  Resources   rook.ResourceSpec     `json:"resources"`
  Mon         rook.MonSpec          `json:"mon"`
}
```

#### Provider Storage Concepts

Similar to how we will not try to force commonality by defining a single `Cluster` type across all backends,
we will also not define single types that define the deployment and configuration of a backend's storage concepts.
For example, both Ceph and Minio present object storage.
Both Ceph and Nexenta present shared file systems.
However, the implementation details for what components and configuration comprise these storage presentations is very provider specific.
Therefore, it is not reasonable to define a common CRD that attempts to normalize how all providers deploy their object or file system presentations.
Any commonality that can be reasonably achieved should be in the form of reusable `*Spec` types and their associated libraries.

Each provider can make a decision about how to expose their storage concepts.
They could be defined as instantiable top level CRDs or they could be defined as collections underneath the top level storage provider CRD.
Below are terse examples to demonstrate the two options.

Top-level CRDs:

```yaml
apiVersion: ceph.rook.io/v1
kind: Cluster
spec:
  ...
---
apiVersion: ceph.rook.io/v1
kind: Pool
spec:
   ...
---
apiVersion: ceph.rook.io/v1
kind: Filesystem
spec:
   ...
```

Collections under storage provider CRD:

```yaml
apiVersion: ceph.rook.io/v1
kind: Cluster
spec:
  pools:
  - name: replicaPool
    replicated:
      size: 1
  - name: ecPool
    erasureCoded:
      dataChunks: 2
      codingChunks: 1
  filesystems:
  - name: filesystem1
    metadataServer:
      activeCount: 1
```

#### StorageScopeSpec

The `StorageScopeSpec` type defines the boundaries or "scope" of the resources that comprise the backing storage substrate for a cluster.
This could be devices, filters, directories, nodes, [persistent volumes](https://github.com/rook/rook/issues/919), and others.
There are user requested means of selecting storage that Rook doesn't currently support that could be included in this type, such as the ability to select a device by path instead of by name, e.g. `/dev/disk/by-id/`.
Also, wildcards/patterns/globbing should be supported on multiple resource types, removing the need for the current `useAllNodes` and `useAllDevices` boolean fields.

By encapsulating this concept as its own type, it can be reused within other custom resources of Rook.
For instance, this would enable Rook to support storage of other types that could benefit from orchestration in cloud-native environments beyond distributed storage systems.

`StorageScopeSpec` could also provide an abstraction from such details as device name changes across reboots.

##### Node and device level config for storage backends

Most storage backends have a need to specify configuration options at the node and device level.
Since the `StorageScopeSpec` type is already defining a node/device hierarchy for the cluster, it would be desirable for storage backends to include their configuration options within this same hierarchy, as opposed to having to repeat the hierarchy again elsewhere in the spec.

However, this isn't completely straight forward because the `StorageScopeSpec` type is a common abstraction and does not have knowledge of specific storage backends.
A solution for this would be to allow backend specific properties to be defined inline within a `StorageScopeSpec` as key/value pairs.
This allows for arbitrary backend properties to be inserted at the node and device level while still reusing the single StorageScopeSpec abstraction, but it means that during deserialization these properties are not strongly typed.
They would be deserialized into a golang `map[string]string`.
However, an operator with knowledge of its specific backend's properties could then take that map and deserialize it into a strong type.

The yaml would like something like this:

```yaml
nodes:
- name: nodeA
  config:
    storeType: "bluestore"
  devices:
  - name: "sda"
    config:
      storeType: "filestore"
```

Note how the Ceph specific properties at the node and device level are string key/values and would be deserialized that way instead of to strong types.
For example, this is what the golang struct would look like:

```go
type StorageScopeSpec struct {
  Nodes []Node `json:"nodes"`
}

type Node struct {
  Name string `json:"name"`
  Config map[string]string `json:"config"`
  Devices []Device `json:"devices"`
}

type Device struct {
  Name string `json:"name"`
  Config map[string]string `json:"config"`
}
```

After Kubernetes has done the general deserialization of the `StorageScopeSpec` into a strong type with weakly typed maps of backend specific config properties, the Ceph operator could easily convert this map into a strong config type that is has knowledge of.
Other backend operators could do a similar thing for their node/device level config.

#### Placement, Resources, Network

As previously mentioned, the `rook.io` API group will also define some other useful `*Spec` types:

* `PlacementSpec`: Defines placement requirements for components of the storage provider, such as node and pod affinity.  This is similar to the existing [Ceph focused `PlacementSpec`](https://github.com/rook/rook/blob/release-0.7/pkg/apis/rook.io/v1alpha1/types.go#L141), but in a generic way that is reusable by all storage providers.  A `PlacementSpec` will essentially be a map of placement information structs that are indexed by component name.
* `NetworkSpec`: Defines the network configuration for the storage provider, such as `hostNetwork`.
* `ResourceSpec`: Defines the resource usage of the provider, allowing limits on CPU and memory, similar to the existing [Ceph focused `ResourceSpec`](https://github.com/rook/rook/blob/release-0.7/pkg/apis/rook.io/v1alpha1/types.go#L85).

#### Additional Types

Rook and the greater community would also benefit from additional types and abstractions.
We should work on defining those further, but it is out of scope for this design document that is focusing on support for multiple storage backends.
Some potential ideas for additional types to support in Rook:

* [Snapshots, backup and policy](https://github.com/rook/rook/issues/1552)
* Quality of Service (QoS), resource consumption (I/O and storage limits)

### Source code and container images

As more storage backends are integrated into Rook, it is preferable that all source code lives within the single `rook/rook` repository.
This has a number of benefits such as easier sharing of build logic, developer iteration when updating shared code, and readability of the full source.
Multiple container images can easily be built from the single source repository, similar to how `rook/rook` and `rook/toolbox` are currently built from the same repository.

* `rook/rook` image:  defines all custom resource types, generated clients, general cluster discovery information (disks), and any storage operators that do not have special tool dependencies.
* backend specific images: to avoid image bloat in the main `rook/rook` image, each backend will have their own image that contains all of their daemons and tools.  These images will be used for the various daemons/components of each backend, e.g. `rook/ceph`, `rook/minio`, etc.

#### Layout

Each storage provider has its own Rook repo.
- [Cassandra](https://github.com/rook/cassandra)
- [Ceph](https://github.com/rook/rook)
- [NFS](https://github.com/rook/nfs)

## Summary

* Rook will enable storage providers to integrate their solutions with cloud-native environments by providing a framework of common abstractions and implementations that helps providers efficiently build reliable and well tested storage controllers.
  * `StorageScopeSpec`, `PlacementSpec`, `ResourceSpec`, `NetworkSpec`, etc.
* Each storage provider will be versioned independently with its own API Group (e.g., `ceph.rook.io`) and its own instantiable CRD type(s).
* Each storage provider will have its own operator pod that performs orchestration and management of the storage resources.  This operator will use a provider specific container image with any special tools needed.

## Examples

This section contains concrete examples of storage clusters as a user would define them using yaml.
In addition to distributed storage clusters, we will be considering support for additional storage types in the near future.

`ceph-cluster.yaml`:

```yaml
apiVersion: ceph.rook.io/v1alpha1
kind: Cluster
metadata:
  name: ceph
  namespace: rook-ceph
spec:
  mon:
    count: 3
    allowMultiplePerNode: false
  network:
    hostNetwork: false
  placement:
  - name: "mon"
    nodeAffinity:
    podAffinity:
    podAntiAffinity:
    tolerations:
  resources:
  - name: osd
    limits:
      cpu: "500m"
      memory: "1024Mi"
    requests:
      cpu: "500m"
      memory: "1024Mi"
  storage:
    deviceFilter: "^sd."
    location:
    config:
      storeConfig:
      storeType: bluestore
      databaseSizeMB: "1024"
      metadataDevice: nvme01
    directories:
    - path: /rook/storage-dir
    nodes:
    - name: "nodeA"
      directories:
      - path: "/rook/storage-dir"
        config:   # ceph specific config at the directory level via key/value pairs
          storeType: "filestore"
    - name: "nodeB"
      devices:
      - name: "vdx"
      - fullpath: "/dev/disk/by-id/abc-123"
    - name: "machine*"  # wild cards are supported
      volumeClaimTemplates:
      - metadata:
        name: my-pvc-template
        spec:
          accessModes: [ "ReadWriteOnce" ]
          storageClassName: "my-storage-class"
          resources:
            requests:
              storage: "1Gi"
```
