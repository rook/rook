---
title: Standalone
weight: 19
---

# Rook Standalone

- [Directly on Linux Host](#directly-on-linux-host)
- [Vagrant](#vagrant)
- [Design](#design)

## Directly on Linux Host

Rook can be deployed as a standalone service directly on any modern Linux host by running the following:

1. Start a one-node Rook cluster
   ```bash
   docker run -it --net=host rook/rook:master
   ```

2. Download the latest `rookctl` binary
   ```bash
   wget https://release.rook.io/alpha/v0.5.0/bin/linux_amd64/rookctl
   chmod 755 rookctl
   ```

3. Verify the Rook cluster is ready
   ```bash
   ./rookctl status
   ```

At this point, you can use the `rookctl` tool along with some [simple steps to create and manage block, file and object storage](client.md).


## Vagrant

Rook is also easy to run in virutal machines with `vagrant` using the standalone [Vagrantfile](/demo/standalone/Vagrantfile).  When instructed, `vagrant` will start up CoreOS virtual machines and launch `rook` via `rkt`.  Note that you can make configuration changes as desired in the provided [cloud config file](/demo/standalone/cloud-config.yml.in).

```
cd demo/standalone
vagrant up
```

## Rook Client Tool

Once the Rook cluster in Vagrant is running and initialized, you can use the `rookctl` client tool to manage the cluster and consume the storage.  First, either use a locally built `rookctl` tool or download the latest release from github:
```
wget https://release.rook.io/alpha/v0.5.0/bin/linux_amd64/rookctl
chmod 755 rookctl
```

Verify the cluster is up and running:
```
export ROOK_API_SERVER_ENDPOINT="172.20.20.10:8124"
./rookctl status
```

At this point, you can use the `rookctl` tool along with some [simple steps to create and manage block, file and object storage](client.md).

## Clean up

When you are all done with your Rook cluster in Vagrant, you can clean everything up by destroying the VMs with:
```
vagrant destroy -f
rm -f .discovery-token
```

## Design

Rook supports an environment where there is no orchestration platform. Rook comes with its own basic orchestration
engine based on Etcd that is activated when not running in Kubernetes.

![Standalone Rook Architecture](media/standalone.png)

The Rook daemon `rook` is a Docker container that has all that is needed to bootstrap, scale
and manage a storage cluster. Each machine in the cluster must run the Rook daemon.

`rook` embeds Etcd to store configuration and coordinate cluster-wide management operations. `rook` will automatically
bootstrap Etcd, manage it, and scale it as the cluster grows. It's also possible to use an external Etcd instead of the embedded one
if needed.

Standalone Rook still requires some key orchestration features such as resource management, health monitoring, failover, and upgrades.
If this scenario is interesting to you, contributions are welcome!
