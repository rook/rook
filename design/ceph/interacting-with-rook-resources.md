# Labels added to Rook-Ceph resources

[Recommended Labels](https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/) are a common set of labels that allows tools to work interoperably, describing objects in a common manner that all tools can understand.

## Labels added to all Resources created by Rook

* `app.kubernetes.io/name`: Is the name of the binary running in a container(combination of "ceph-"+daemonType).

* `app.kubernetes.io/instance`: A unique name identifying the instance of an application. Due to the nature of how resources are named in Rook, this is guaranteed to be unique per CephCluster namespace but not unique within the entire Kubernetes cluster.

* `app.kubernetes.io/component`: This is populated with the Kind of the resource controlling this application. For example, `cephclusters.ceph.rook.io` or `cephfilesystems.ceph.rook.io`.

* `app.kubernetes.io/part-of`: This is populated with the Name of the resource controlling this application.

* `app.kubernetes.io/managed-by`: `rook-ceph-operator` is the tool being used to manage the operation of an application

* `app.kubernetes.io/created-by`: `rook-ceph-operator` is the controller/user who created this resource

* `rook.io/operator-namespace`: The namespace in which rook-ceph operator is running.

An Example of Recommended Labels on Ceph mon with ID=a will look like:
```
	app.kubernetes.io/name       : "ceph-mon"
	app.kubernetes.io/instance   : "a"
	app.kubernetes.io/component  : "cephclusters.ceph.rook.io"
	app.kubernetes.io/part-of    : "rook-ceph"
	app.kubernetes.io/managed-by : "rook-ceph-operator"
	app.kubernetes.io/created-by : "rook-ceph-operator"
	rook.io/operator-namespace   : "rook-ceph"
```

Another example on CephFilesystem with ID=a:
```
	app.kubernetes.io/name       : "ceph-mds"
	app.kubernetes.io/instance   : "myfs-a"
	app.kubernetes.io/component  : "cephfilesystems.ceph.rook.io"
	app.kubernetes.io/part-of    : "myfs"
	app.kubernetes.io/managed-by : "rook-ceph-operator"
	app.kubernetes.io/created-by : "rook-ceph-operator"
	rook.io/operator-namespace   : "rook-ceph"
``` 

**NOTE** : A totally unique string for an application can be built up from (a) app.kubernetes.io/component, (b) app.kubernetes.io/part-of, (c) the resource's namespace, (d) app.kubernetes.io/name, and (e) app.kubernetes.io/instance fields. For the example above, we could join those fields with underscore connectors like this: cephclusters.ceph.rook.io_rook-ceph_rook-ceph_ceph-mon_a. Note that this full spec can easily exceed the 64-character limit imposed on Kubernetes labels.