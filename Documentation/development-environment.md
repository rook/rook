---
title: Multi-Node Test Environment
weight: 12100
indent: true
---

# Multi-Node Test Environment

* [Using KVM/QEMU and Kubespray](#using-kvmqemu-and-kubespray)
* [Using VirtualBox and k8s-vagrant-multi-node](#using-virtualbox-and-k8s-vagrant-multi-node)
* [Using Vagrant on Linux with libvirt](#using-vagrant-on-linux-with-libvirt)
* [Using CodeReady Containers for setting up single node openshift 4.x cluster](#using-codeready-containers-for-setting-up-single-node-openshift-4x-cluster)

## Using KVM/QEMU and Kubespray

### Setup expectation

There are a bunch of pre-requisites to be able to deploy the following environment. Such as:

* A Linux workstation (CentOS or Fedora)
* KVM/QEMU installation
* docker service allowing insecure local repository

For other Linux distribution, there is no guarantee the following will work.
However adapting commands (apt/yum/dnf) could just work.

### Prerequisites installation

On your host machine, execute `tests/scripts/multi-node/rpm-system-prerequisites.sh` (or
do the equivalent for your distribution)

Edit `/etc/docker/daemon.json` to add insecure-registries:

```json
{
        "insecure-registries":  ["172.17.8.1:5000"]
}
```

### Deploy Kubernetes with Kubespray

Clone it:

```console
git clone https://github.com/kubernetes-sigs/kubespray/
cd kubespray
```

Edit `inventory/sample/group_vars/k8s-cluster/k8s-cluster.yml` with:

```console
docker_options: {% raw %}"--insecure-registry=172.17.8.1:5000 --insecure-registry={{ kube_service_addresses }} --data-root={{ docker_daemon_graph }} {{ docker_log_opts }}"{% endraw %}
```

FYI: `172.17.8.1` is the libvirt bridge IP, so it's reachable from all your virtual machines.
This means a registry running on the host machine is reachable from the virtual machines running the Kubernetes cluster.

Create Vagrant's variable directory:

```console
mkdir vagrant/
```

Put `tests/scripts/multi-node/config.rb` in `vagrant/`. You can adapt it at will.
Feel free to adapt `num_instances`.

Deploy!

```console
vagrant up --no-provision ; vagrant provision
```

Go grab a coffee:

>```
>PLAY RECAP *********************************************************************
>k8s-01                     : ok=351  changed=111  unreachable=0    failed=0
>k8s-02                     : ok=230  changed=65   unreachable=0    failed=0
>k8s-03                     : ok=230  changed=65   unreachable=0    failed=0
>k8s-04                     : ok=229  changed=65   unreachable=0    failed=0
>k8s-05                     : ok=229  changed=65   unreachable=0    failed=0
>k8s-06                     : ok=229  changed=65   unreachable=0    failed=0
>k8s-07                     : ok=229  changed=65   unreachable=0    failed=0
>k8s-08                     : ok=229  changed=65   unreachable=0    failed=0
>k8s-09                     : ok=229  changed=65   unreachable=0    failed=0
>
>Friday 12 January 2018  10:25:45 +0100 (0:00:00.017)       0:17:24.413 ********
>===============================================================================
>download : container_download | Download containers if pull is required or told to always pull (all nodes) - 192.44s
>kubernetes/preinstall : Update package management cache (YUM) --------- 178.26s
>download : container_download | Download containers if pull is required or told to always pull (all nodes) - 102.24s
>docker : ensure docker packages are installed -------------------------- 57.20s
>download : container_download | Download containers if pull is required or told to always pull (all nodes) -- 52.33s
>kubernetes/preinstall : Install packages requirements ------------------ 25.18s
>download : container_download | Download containers if pull is required or told to always pull (all nodes) -- 23.74s
>download : container_download | Download containers if pull is required or told to always pull (all nodes) -- 18.90s
>download : container_download | Download containers if pull is required or told to always pull (all nodes) -- 15.39s
>kubernetes/master : Master | wait for the apiserver to be running ------ 12.44s
>download : container_download | Download containers if pull is required or told to always pull (all nodes) -- 11.83s
>download : container_download | Download containers if pull is required or told to always pull (all nodes) -- 11.66s
>kubernetes/node : install | Copy kubelet from hyperkube container ------ 11.44s
>download : container_download | Download containers if pull is required or told to always pull (all nodes) -- 11.41s
>download : container_download | Download containers if pull is required or told to always pull (all nodes) -- 11.00s
>docker : Docker | pause while Docker restarts >-------------------------- 10.22s
>kubernetes/secrets : Check certs | check if a cert already exists on node --- 6.05s
>kubernetes-apps/network_plugin/flannel : Flannel | Wait for flannel subnet.env file presence --- 5.33s
>kubernetes/master : Master | wait for kube-scheduler -------------------- 5.30s
>kubernetes/master : Copy kubectl from hyperkube container --------------- 4.77s
>```
```console
vagrant ssh k8s-01
```
>```
>Last login: Fri Jan 12 09:22:18 2018 from 192.168.121.1
>```
```console
kubectl get nodes
```
>```
>NAME      STATUS    ROLES         AGE       VERSION
>k8s-01    Ready     master,node   2m        v1.9.0+coreos.0
>k8s-02    Ready     node          2m        v1.9.0+coreos.0
>k8s-03    Ready     node          2m        v1.9.0+coreos.0
>k8s-04    Ready     node          2m        v1.9.0+coreos.0
>k8s-05    Ready     node          2m        v1.9.0+coreos.0
>k8s-06    Ready     node          2m        v1.9.0+coreos.0
>k8s-07    Ready     node          2m        v1.9.0+coreos.0
>k8s-08    Ready     node          2m        v1.9.0+coreos.0
>k8s-09    Ready     node          2m        v1.9.0+coreos.0
>```

### Running the Kubernetes Dashboard UI

kubespray sets up the Dashboard pod by default, but you must authenticate with a bearer token, even for localhost access with kubectl proxy.  To allow access, one possible solution is to:

1) Create an admin user by creating admin-user.yaml with these contents (and using kubectl -f create admin-user.yaml):

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: admin-user
  namespace: kube-system
```

2) Grant that user the ClusterRole authorization by creating and applying admin-user-cluster.role.yaml:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: admin-user
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: ServiceAccount
  name: admin-user
  namespace: kube-system
```

3) Find the admin-user token in the kube-system namespace:

```console
kubectl -n kube-system describe secret $(kubectl -n kube-system get secret | grep admin-user | awk '{print $1}')
```

and you can use that token to log into the UI at http://localhost:8001/ui.

(See [https://github.com/kubernetes/dashboard/wiki/Creating-sample-user](https://github.com/kubernetes/dashboard/wiki/Creating-sample-user))

### Development workflow on the host

Everything should happen on the host, your development environment will reside on the host machine NOT inside the virtual machines running the Kubernetes cluster.

Now, please refer to the [development flow](development-flow.md) to setup your development environment (go, git etc).

At this stage, Rook should be cloned on your host.

From your Rook repository (should be $GOPATH/src/github.com/rook) location execute `bash tests/scripts/multi-node/build-rook.sh`.
During its execution, `build-rook.sh` will purge all running Rook pods from the cluster, so that your latest container image can be deployed.
Furthermore, **all Ceph data and config will be purged** as well.
Ensure that you are done with all existing state on your test cluster before executing `build-rook.sh` as it will clear everything.

Each time you build and deploy with `build-rook.sh`, the virtual machines (k8s-0X) will pull the new container image and run your new Rook code.
You can run `bash tests/scripts/multi-node/build-rook.sh` as many times as you want to rebuild your new rook image and redeploy a cluster that is running your new code.

From here, resume your dev, change your code and test it by running `bash tests/scripts/multi-node/build-rook.sh`.

### Teardown

Typically, to flush your environment you will run the following from within kubespray's git repository.
This action will be performed on the host:

```console
vagrant destroy -f
```

Also, if you were using `kubectl` on that host machine, you can resurrect your old configuration by renaming `$HOME/.kube/config.before.rook.$TIMESTAMP` with `$HOME/.kube/config`.

If you were not using `kubectl`, feel free to simply remove `$HOME/.kube/config.rook`.

## Using VirtualBox and k8s-vagrant-multi-node

### Prerequisites

Be sure to follow the prerequisites here: https://github.com/galexrt/k8s-vagrant-multi-node/tree/master#prerequisites.

### Quickstart

To start up the environment just run `./tests/scripts/k8s-vagrant-multi-node.sh up`.
This will bring up one master and 2 workers by default.

To change the amount of workers to bring up and their resources, be sure to checkout the [galexrt/k8s-vagrant-multi-node project README Variables section](https://github.com/galexrt/k8s-vagrant-multi-node/tree/master#variables).
Just set or export the variables as you need on the script, e.g., either `NODE_COUNT=5 ./tests/scripts/k8s-vagrant-multi-node.sh up`, or `export NODE_COUNT=5` and then `./tests/scripts/k8s-vagrant-multi-node.sh up`.

For more information or if you are experiencing issues, please create an issue at [GitHub galexrt/k8s-vagrant-multi-node](https://github.com/galexrt/k8s-vagrant-multi-node).

## Using Vagrant on Linux with libvirt

See https://github.com/noahdesu/kubensis.

## Using CodeReady Containers for setting up single node openshift 4.x cluster

See https://code-ready.github.io/crc/
