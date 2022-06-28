---
title: Object Store Backend
target-version: release-1.10
---

# Overview

Ceph RADOS Gateway is undergoing a major overhaul with the Zipper project. The goal of Project
Zipper is to create an API that can be used to separate the RGW top end (represented by Swift and
S3) from bottom end providers (represented currently by RADOS, and eventually by Cloud, File, and
Memory backends). This should allow the front ends to access any implemented back end
transparently.

The secondary objective is to allow multiple layers of these API providers to stack up, allowing
transformations of operations (such as redirection to different backends), or caching layers that
cut across multiple backends.

The primary Store is the current RadosStore, which provides access to a Ceph cluster via RADOS.
However, new Stores are possible that store the data in any desired platform. One such Store, called
DBStore, has been developed that stores data in SQL, and specifically in a local SQLite database.

## Goals

We want to start trying to deploy non-RADOS backends when deploying a CephObjectStore. However, we
still want Rook-Ceph to be used to deploy the RADOS backend, **only**.
Rook-Ceph has a tremendous support for the Ceph Object Store and we want to leverage that. All the
features like TLS configuration, encryption, multisite need to be re-used as much as possible.
Especially the various controllers that are reconciling ObjectStore, Multisite configurations and
Bucket provisioning.

## Non-goals

The goal is **NOT** to have Rook-Ceph becoming the Operator to deploy Zipper. Zipper will support
multiple backends, each will have their own deployment methods. Especially proprietary backends
might not have any Kubernetes objects but simply endpoints.
Also, we don't Rook-Ceph to accept code from external backends, this will complicate the
maintenance and code contributions. Who can potentially review a backend code PR from a different
driver no one is familiar with.
Ultimately we will build a new Operator for Zipper, which will accepts endpoints provided/deployed
by Operator responsible for their own backend. For instance, a proprietary backend will deploy its
own configuration (perhaps just a Kubernetes Service) and provide an endpoint to access the data.
We want Rook-Ceph to continue to deploy the RADOS backend, and we want to use the same features but
with a different Operator that deploys a non-RADOS backend.
For this, small API changes are required in `ObjectStoreSpec` and are highlighted below.

# Proposal details

The `CephObjectStore` API will start exposing a backend capability.

```go
type GatewaySpec struct {
...
	// Backend refers to the backend to use for the object store
	// +optional
	// If left empty, RADOS is used as the default backend
	Backend *ObjectStoreBackend `json:"backend,omitempty"`
...
}
```

Where the `ObjectStoreBackend` is represented as such:

```go
type ObjectStoreBackend struct {
	// The name of the backend
	// +optional
	// +kubebuilder:validation:Enum={"rados","dbstore"}
	Name string `json:"name,omitempty"`

	// Mutation will mutate the object store Deployment resource
	// The mutate function is marked optional but the non-RADOS backends are expected to fully implement it.
	// Backends such as "dbstore" will need to implement this.
	// Things like Image, ServiceAccount and Deployment specs are expected to be mutated.
	// +optional
	Mutation *MutationSpec `json:"mutate,omitempty"`

	// Config is the configuration for the backend
	// +optional
	// At the time of the writing, it's unclear how this could be used
	// but somehow feels like this could be needed to allow passing simple configuration to the backend.
	Config map[string]string `json:"config,omitempty"`
}
```

Where the `MutationSpec` type will look like:

```go
// Mutation defines how resource are modified.
type MutationSpec struct {
	// PatchesJSON6902 is a list of RFC 6902 JSON Patch declarations used to modify resources.
	// See https://tools.ietf.org/html/rfc6902 and https://jsonpatch.com/.
	// +optional
	PatchesJSON6902 string `json:"patchesJson6902,omitempty"`
}
```

With the mutate function, the Controller does not need any code changes if the backend evolves. It
also reduces the amount of code that needs to be written for new backends.
For instance, a non-RADOS backend such as "dbstore" will need a PVC store the sqlite database, so
the Deployment will need to be mutated to include the Volumes and VolumeMounts.
New init containers could be added if necessary. The Controller will treat this as a normal CRD
change and thus reconcile all the objects if anything changes. After generating its internal
Deployment the mutate function will be called and then the Deployment will be created/updated.

To avoid confusion, we **don't** want to expose the ability to configure multiple backends from
Rook-Ceph, see the non-goal section above for more details.
The configuration will be limited to a single backend.
Rook-Ceph Operator will print out a message if `CephObjectStore` backend is configured with a non-RADOS
backend. This scenario is out of scope of what's supported since the non-RADOS backends are expected
to be deployed with their own Operator.

## Risks and Mitigation

We still need to determine what the best approach between using the Rook-Ceph operator as is or
building a new operator.

Some pros to use the Rook-Ceph operator:
* It is easier to use
* No need to change the various controller packages
* Code is maintained at a single location

Some cons to use the Rook-Ceph operator:
* More resources (controllers) are started than needed
* The name Rook-Ceph implies that Ceph is being deployed which can be confusing if a "standalone"
  version of RGW is being used.
* Bloat Rook-Ceph's codebase and responsibility

Some pros to use a new Operator:
* It's much lighter
* The base image most likely does not need Ceph packages installed so we can have minimal image. One
  technique for this is to forward all the radosgw commands (only one to create the RGW admin Op
  user) to the RGW pod directly.
* clear separation of concerns

Some cons to use a new Operator:
* New Operator to write
* Refactor a little bit object/lib-bucket-prov controllers packages to avoid using internal struct
  that don't make sense like `ClusterInfo` (from the standalone rgw perspective)
