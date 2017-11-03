# Running Rook on tectonic

## Prequisites
- A fully running Tectonic cluster
- Some unused, unformatted disks on the nodes

## Preparations

*Run the following commands on EVERY node of your Tectonic cluster*

```
$ ssh core@DNS-OR-IP-OF-YOUR-NODE
$ sudo mkdir -p /var/lib/kubelet/volumeplugins 
$ sudo mkdir -p /var/lib/rook
```

Update the `kubelet.service` to use the new volumeplugin direcory:

```
$ sudo vi /etc/systemd/system/kubelet.service
```

And add the following line as a kubelet argement (in the *ExecStart* section)
`--volume-plugin-dir=/var/lib/kubelet/volumeplugins`

Save and apply (:wq to quit vim)

```
$ sudo systemctl daemon-reload
$ sudo systemctl restart kubelet
```

## Start the rook configuration

Follow the instructions as stated in [the installation docs](https://rook.github.io/docs/rook/master/kubernetes.html). Best thing is to provide the disks to use in the [rook-cluster.yaml](https://rook.github.io/docs/rook/master/cluster-crd.html#storage-configuration-settings)

TODO:: Integrate the prometheus with the prometheus from Tectonic
