---
title: Cassandra Cluster CRD
weight: 5000
---

# Cassandra Cluster CRD

Cassandra database clusters can be created and configuring using the `clusters.cassandra.rook.io` custom resource definition (CRD).

Please refer to the the [user guide walk-through](cassandra.md) for complete instructions.
This page will explain all the available configuration options on the Cassandra CRD.

## Sample

```yaml
apiVersion: cassandra.rook.io/v1alpha1
kind: Cluster
metadata:
  name: rook-cassandra
  namespace: rook-cassandra
spec:
  version: 3.11.1
  repository: my-private-repo.io/cassandra
  mode: cassandra
  # A key/value list of annotations
  annotations:
  #  key: value
  datacenter:
    name: us-east-1
    racks:
      - name: us-east-1a
        members: 3
        storage:
          volumeClaimTemplates:
            - metadata:
                name: rook-cassandra-data
              spec:
                storageClassName: my-storage-class
                resources:
                  requests:
                    storage: 200Gi
        resources:
          requests:
            cpu: 8
            memory: 32Gi
          limits:
            cpu: 8
            memory: 32Gi
        # A key/value list of annotations
        annotations:
        #  key: value
        placement:
          nodeAffinity:
            requiredDuringSchedulingIgnoredDuringExecution:
              nodeSelectorTerms:
                - matchExpressions:
                  - key: failure-domain.beta.kubernetes.io/region
                    operator: In
                    values:
                      - us-east-1
                  - key: failure-domain.beta.kubernetes.io/zone
                    operator: In
                    values:
                      - us-east-1a
```

## Settings Explanation

### Cluster Settings

* `version`: The version of Cassandra to use. It is used as the image tag to pull.
* `repository`: Optional field. Specifies a custom image repo. If left unset, the official docker hub repo is used.
* `mode`: Optional field. Specifies if this is a Cassandra or Scylla cluster. If left unset, it defaults to cassandra. Values: {scylla, cassandra}
* `annotations`: Key value pair list of annotations to add.

In the Cassandra model, each cluster contains datacenters and each datacenter contains racks. At the moment, the operator only supports single datacenter setups.

### Datacenter Settings

* `name`: Name of the datacenter. Usually, a datacenter corresponds to a region.
* `racks`: List of racks for the specific datacenter.

### Rack Settings

* `name`: Name of the rack. Usually, a rack corresponds to an availability zone.
* `members`: Number of Cassandra members for the specific rack. (In Cassandra documentation, they are called nodes. We don't call them nodes to avoid confusion as a Cassandra Node corresponds to a Kubernetes Pod, not a Kubernetes Node).
* `storage`: Defines the volumes to use for each Cassandra member. Currently, only 1 volume is supported.
* `resources`: Defines the CPU and RAM resources for the Cassandra Pods.
* `annotations`: Key value pair list of annotations to add.
* `placement`: Defines the placement of Cassandra Pods. Has the following subfields:
  * [`nodeAffinity`](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#affinity-and-anti-affinity)
  * [`podAffinity`](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#affinity-and-anti-affinity)
  * [`podAntiAffinity`](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#affinity-and-anti-affinity)
  * [`tolerations`](https://kubernetes.io/docs/concepts/configuration/taint-and-toleration/)
