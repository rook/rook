# Starting your first castle cluster

### 1) Create a new cluster

To create a new cluster you must generate a new cluster token. This can easily be done using 
the publicly hosted castle discovery service:

```
curl -w "\n" 'https://discovery.castle.io/new?min-size=3'
https://discovery.castle.io/6a28e078895c5ec737174db2419bb2f3078895c5ec737174db2419bb2f3
```

The minimum size parameter can be used to prevent the cluster from starting up until
the specified number of nodes join.

### 2) Add nodes to the cluster

You now need to start castle on nodes which will automatically find each other through
the discovery service and form a cluster. You can run the castle nodes using a number
of different approaches.

#### Running manually

Download castle from http://github.com/quantum/castle/releases

```
castle \
  --discovery=https://discovery.castle.io/6a28e078895c5ec737174db2419bb2f3078895c5ec737174db2419bb2f3 \
  --address 10.0.10.1:70000 \
  --devices sda,sdb,sdc \
  --public-network 10.0.10.0/24 \
  --private-network 10.0.20.0/24
```

TODO: other options: CPU affinity and limiting to support hyper-converged
TODO: crush location for example, data-center=red,rack=17,server=blue23
TODO: choices for selecting the kind of hardware? NVMe etc.? or can we autodetect?

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

### 3) Using castlectl

Now that the cluster is up and running, you can communicate with it using castletctl. 

```
export CASTLE_DISCOVERY=https://discovery.castle.io/6a28e078895c5ec737174db2419bb2f3078895c5ec737174db2419bb2f3

castlectl node ls
```

the output shows the nodes that are part of the cluster

```
ADDRESS           STATE       CLUSTER         SIZE     LOCATION                 UPDATED
10.0.10.1:70000   OK          6a28e078895c5e  30 TiB   dc=A,rack=17,server=a    1 sec ago
10.0.10.2:70000   OK          6a28e078895c5e  40 TiB   dc=A,rack=18,server=b    1 sec ago
10.0.10.3:70000   OK          6a28e078895c5e  50 TiB   dc=A,rack=19,server=c    1 sec ago
```

### 4) Creating a volume

To create a volume run the following:

```
castlectl volume create myvolume 100GiB
```

This creates a volume that can be used as a block device from client nodes.

### 5) Mount the volume on a client node

To mount the volume run the following on the client node:

```
export CASTLE_DISCOVERY=https://discovery.castle.io/6a28e078895c5ec737174db2419bb2f3078895c5ec737174db2419bb2f3
sudo castleblk rbd myvolume
```

You can mount the volume through NBD or RBD.
