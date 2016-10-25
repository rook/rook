# Starting your first castle cluster

### 1) Create a new cluster

The easiest way to create a new cluster is through the global discovery service:

```
curl -w "\n" 'https://discovery.castle.io/new?min-size=3&dns=true'
https://discovery.castle.io/6a28e078895c5ec737174db2419bb2f3
```

This will return a URL that acts as the logical "address" of the cluster, and enables
clients to easily find the cluster.

The minimum size parameter can be used to prevent the cluster from starting up until
the specified number of nodes join. 

The DNS parameter will cause the discovery service to publish a DNS name for 
the new cluster like:

```
6a28e078895c5ec737174db2419bb2f3.castle.io
```

You can create custom CNAME entries in your own DNS servers that point to these entries, 
for example:

```
mycluster.acme.com
```

### 2) Add nodes to the cluster

You now need to start castle on nodes which will automatically find each other through
the discovery service and form a cluster. You can run the castle nodes using a number
of different approaches.

#### Running manually

Download castle from http://github.com/rook/rook/releases

```
castle \
  --discovery-url=https://discovery.castle.io/6a28e078895c5ec737174db2419bb2f3 \
  --devices sda,sdb,sdc \
  --public-network 10.0.10.0/24 \
  --private-network 10.0.20.0/24
```

* TODO: other options: CPU affinity and limiting to support hyper-converged
* TODO: crush location for example, data-center=red,rack=17,server=blue23
* TODO: choices for selecting the kind of hardware? NVMe etc.? or can we autodetect?

#### Running from systemd

TODO:

#### Running from docker

TODO: privileged container with /dev mapped?

#### Running from rkt

TODO: rkt fly

#### Running on CoreOS

TODO: show how castle can be configured from cloud-init and started

#### Running from Ubuntu

#### Running from Centos

At this point the storage cluster is up and running.

### 3) Using castle

Now that the cluster is up and running, you can communicate with it using castletctl. First
you need export an environment variable for the cluster url:

```
export CASTLE_DISCOVERY_URL=https://discovery.castle.io/6a28e078895c5ec737174db2419bb2f3
```

or if you're using DNS:

```
export CASTLE_DISCOVERY_DNS=mycluster.acme.com
```

Let's list the nodes that are part of the cluster:

```
castle node ls
```

the output shows the nodes that are part of the cluster

```
ADDRESS           STATE       CLUSTER             SIZE     LOCATION                 UPDATED
10.0.10.1:70000   OK          mycluster.acme.com  30 TiB   dc=A,rack=17,server=a    1 sec ago
10.0.10.2:70000   OK          mycluster.acme.com  40 TiB   dc=A,rack=18,server=b    1 sec ago
10.0.10.3:70000   OK          mycluster.acme.com  50 TiB   dc=A,rack=19,server=c    1 sec ago
```

### 4) Creating a volume

To create a volume run the following:

```
castle volume create myvolume 100GiB
```

This creates a volume that can be used as a block device from client nodes.

### 5) Mount the volume on a client node

To mount the volume run the following on the client node:

```
export CASTLE_DISCOVERY_DNS=mycluster.acme.com
sudo castleblk rbd myvolume
```

You can mount the volume through NBD or RBD.
