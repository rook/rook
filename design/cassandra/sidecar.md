## Sidecar Design Proposal

### Consideration: REST API

When thinking about how our sidecar will communicate with our controller, a natural solution that comes to mind is though a REST API. The sidecar will run an HTTP Server which the other party will call. This is the approach used by [Netflix's Priam](https://github.com/Netflix/priam/wiki).

However, this includes some extra complications. Remember that, according to our goals we are designing a level-based system.
First of all, some operations just take a long time. Backup, for example, might take hours to complete. 
That means our operator must have an open TCP connection for all this time. If it gets interrupted, which we do expect to happen, this connection will be lost and we won't have any record that it ever happened. 


This doesn't seem like the Kubernetes way of doing things. Consider this example, as one could think of the kubelet as a sidecar for Pods: 

* **Question:** Does the scheduler ping the kubelet each time it schedules a Pod and then wait for an answer? 
* **Answer:** No, it writes the `nodeName` field on the PodSpec. In other words, it writes a record of intent. No matter how many times the kubelet or the scheduler crashes it doesn't matter. The record of intent is there. 

### Control-Loop Design

* Based on our observations above, we design a method of communication in line with the Kubernetes philosophy.
* When the controller wants to invoke a functionality in a Sidecar, it should write a record of intent in the Kubernetes API Objects (etcd). The sidecar will be watching the Kubernetes API and responding accordingly.
* There are two approaches to represent the record of intent:
  1. **Labels:**
    * When the controller wants to communicate with a sidecar, it will write a predefined label in the ClusterIP Service Object of the specific instance. For example, to communicate that we want an instance to decommission, we could write the label 'cassandra.rook.io/decommission`. The sidecar will see this and decommission the Cassandra instance. When it is done, it will change the label value to a predefined value. Then the controller will know to delete that instance.
    * **Advantages:**
      * Reuses Kubernetes built-in mechanisms
      * Labels are query-able
    * **Disadvantages:** 
      * Doesn't support nested fields
  2. **Member CRD**
    * Each sidecar will watch an instance of a newly defined Member CRD and have its own `Spec` and `Status`.
    * **Advantages:**
      * More expressive and natural. Supports nested fields.
      * Only our operator touches it. We don't expect it to happen often, but Pods are touched by pretty much everyone on the cluster. So if someone does stupid things, that affects us too.
    * **Disadvantages:**
      * Probably overkill to have for only a couple of fields.
      * Induces an extra burden on etcd.

### Decision

* Given the above advantages and disadvantages of each approach, we will start implementing the Cassandra operator without the extra complexity of the Member Object. If in the process of developing it becomes clear that it is needed, we will add it then.

### Example

Let's consider the case of creating a new Cassandra Cluster. It will look something like this:


1. *User* creates CRD for a Cassandra Cluster.

2. *Controller* sees the newly created CRD object and creates a StatefulSet for each Cassandra Rack and a ClusterIP Service for each member to serve as its static IP. Seed members have the label `cassandra.rook.io/seed` on their Service.

3. *Cassandra* container starts and our custom entrypoint is entered. It waits for config files to be written to a predefined location (shared volume - emptyDir), then copies them to the correct location and starts. 

4. *Sidecar* starts, syncs with the Kubernetes API and gets its corresponding Service ClusterIP Object.
   1. Retrieve the static ip from `spec.clusteIP`.
   2. Get seed addresses by querying for the label `cassandra.rook.io/seed` in Services.
   3. Generate config files with our custom options and start Cassandra.


