---
title: Ceph Dashboard
---

The dashboard is a very helpful tool to give you an overview of the status of your Ceph cluster, including overall health,
status of the mon quorum, status of the mgr, osd, and other Ceph daemons, view pools and PG status, show logs for the daemons,
and more. Rook makes it simple to enable the dashboard.

![The Ceph dashboard](ceph-dashboard/ceph-dashboard.png)

## Enable the Ceph Dashboard

The [dashboard](https://docs.ceph.com/en/latest/mgr/dashboard/) can be enabled with settings in the CephCluster CRD. The CephCluster CRD must have the dashboard `enabled` setting set to `true`.
This is the default setting in the example manifests.

```yaml
[...]
spec:
  dashboard:
    enabled: true
```

The Rook operator will enable the ceph-mgr dashboard module. A service object will be created to expose that port inside the Kubernetes cluster. Rook will
enable port 8443 for https access.

This example shows that port 8443 was configured.

```console
$ kubectl -n rook-ceph get service
NAME                         TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)          AGE
rook-ceph-mgr                ClusterIP   10.108.111.192   <none>        9283/TCP         3h
rook-ceph-mgr-dashboard      ClusterIP   10.110.113.240   <none>        8443/TCP         3h
```

The first service is for reporting the [Prometheus metrics](ceph-monitoring.md), while the latter service is for the dashboard.
If you are on a node in the cluster, you will be able to connect to the dashboard by using either the
DNS name of the service at `https://rook-ceph-mgr-dashboard-https:8443` or by connecting to the cluster IP,
in this example at `https://10.110.113.240:8443`.

### Login Credentials

After you connect to the dashboard you will need to login for secure access. Rook creates a default user named
`admin` and generates a secret called `rook-ceph-dashboard-password` in the namespace where the Rook Ceph cluster is running.
To retrieve the generated password, you can run the following:

```console
kubectl -n rook-ceph get secret rook-ceph-dashboard-password -o jsonpath="{['data']['password']}" | base64 --decode && echo
```

## Configure the Dashboard

The following dashboard configuration settings are supported:

```yaml
spec:
  dashboard:
    urlPrefix: /ceph-dashboard
    port: 8443
    ssl: true
```

* `urlPrefix` If you are accessing the dashboard via a reverse proxy, you may
  wish to serve it under a URL prefix.  To get the dashboard to use hyperlinks
  that include your prefix, you can set the `urlPrefix` setting.
* `port` The port that the dashboard is served on may be changed from the
  default using the `port` setting. The corresponding K8s service exposing the
  port will automatically be updated.
* `ssl` The dashboard may be served without SSL (useful for when you deploy the
  dashboard behind a proxy already served using SSL) by setting the `ssl` option
  to be false.

## Visualization of 'Physical Disks' section in the dashboard

Information about physical disks is available only in [Rook host clusters](../../CRDs/Cluster/host-cluster.md).

The Rook manager module is required by the dashboard to obtain the information about physical disks, but it is disabled by default. Before it is enabled, the dashboard 'Physical Disks' section will show an error message.

To prepare the Rook manager module to be used in the dashboard, modify your Ceph Cluster CRD:

```yaml
  mgr:
    modules:
      - name: rook
        enabled: true
```

And apply the changes:

```console
$ kubectl apply -f cluster.yaml
```

Once the Rook manager module is enabled as the orchestrator backend, there are two settings required for showing disk information:

* `ROOK_ENABLE_DISCOVERY_DAEMON`: Set to `true` to provide the dashboard the information about physical disks. The default is `false`.
* `ROOK_DISCOVER_DEVICES_INTERVAL`: The interval for changes to be refreshed in the set of physical disks in the cluster. The default is `60` minutes.

Modify the operator.yaml, and apply the changes:

```console
$ kubectl apply -f operator.yaml
```

## Viewing the Dashboard External to the Cluster

Commonly you will want to view the dashboard from outside the cluster. For example, on a development machine with the
cluster running inside minikube you will want to access the dashboard from the host.

There are several ways to expose a service that will depend on the environment you are running in.
You can use an [Ingress Controller](https://kubernetes.io/docs/concepts/services-networking/ingress/) or [other methods](https://kubernetes.io/docs/concepts/services-networking/service/#publishing-services-service-types) for exposing services such as
NodePort, LoadBalancer, or ExternalIPs.

### Node Port

The simplest way to expose the service in minikube or similar environment is using the NodePort to open a port on the
VM that can be accessed by the host. To create a service with the NodePort, save this yaml as `dashboard-external-https.yaml`.

```yaml
apiVersion: v1
kind: Service
metadata:
  name: rook-ceph-mgr-dashboard-external-https
  namespace: rook-ceph
  labels:
    app: rook-ceph-mgr
    rook_cluster: rook-ceph
spec:
  ports:
  - name: dashboard
    port: 8443
    protocol: TCP
    targetPort: 8443
  selector:
    app: rook-ceph-mgr
    rook_cluster: rook-ceph
  sessionAffinity: None
  type: NodePort
```

Now create the service:

```console
kubectl create -f dashboard-external-https.yaml
```

You will see the new service `rook-ceph-mgr-dashboard-external-https` created:

```console
$ kubectl -n rook-ceph get service
NAME                                    TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)          AGE
rook-ceph-mgr                           ClusterIP   10.108.111.192   <none>        9283/TCP         4h
rook-ceph-mgr-dashboard                 ClusterIP   10.110.113.240   <none>        8443/TCP         4h
rook-ceph-mgr-dashboard-external-https  NodePort    10.101.209.6     <none>        8443:31176/TCP   4h
```

In this example, port `31176` will be opened to expose port `8443` from the ceph-mgr pod. Find the ip address
of the VM. If using minikube, you can run `minikube ip` to find the ip address.
Now you can enter the URL in your browser such as `https://192.168.99.110:31176` and the dashboard will appear.

### Load Balancer

If you have a cluster on a cloud provider that supports load balancers,
you can create a service that is provisioned with a public hostname.
The yaml is the same as `dashboard-external-https.yaml` except for the following property:

```yaml
spec:
[...]
  type: LoadBalancer
```

Now create the service:

```console
kubectl create -f dashboard-loadbalancer.yaml
```

You will see the new service `rook-ceph-mgr-dashboard-loadbalancer` created:

```console
$ kubectl -n rook-ceph get service
NAME                                     TYPE           CLUSTER-IP       EXTERNAL-IP                                                               PORT(S)             AGE
rook-ceph-mgr                            ClusterIP      172.30.11.40     <none>                                                                    9283/TCP            4h
rook-ceph-mgr-dashboard                  ClusterIP      172.30.203.185   <none>                                                                    8443/TCP            4h
rook-ceph-mgr-dashboard-loadbalancer     LoadBalancer   172.30.27.242    a7f23e8e2839511e9b7a5122b08f2038-1251669398.us-east-1.elb.amazonaws.com   8443:32747/TCP      4h
```

Now you can enter the URL in your browser such as `https://a7f23e8e2839511e9b7a5122b08f2038-1251669398.us-east-1.elb.amazonaws.com:8443` and the dashboard will appear.

### Ingress Controller

If you have a cluster with an [nginx Ingress Controller](https://kubernetes.github.io/ingress-nginx/)
and a Certificate Manager (e.g. [cert-manager](https://cert-manager.readthedocs.io/)) then you can create an
Ingress like the one below. This example achieves four things:

1. Exposes the dashboard on the Internet (using an reverse proxy)
2. Issues an valid TLS Certificate for the specified domain name (using [ACME](https://en.wikipedia.org/wiki/Automated_Certificate_Management_Environment))
3. Tells the reverse proxy that the dashboard itself uses HTTPS
4. Tells the reverse proxy that the dashboard itself does not have a valid certificate (it is self-signed)

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: rook-ceph-mgr-dashboard
  namespace: rook-ceph
  annotations:
    kubernetes.io/tls-acme: "true"
    nginx.ingress.kubernetes.io/backend-protocol: "HTTPS"
    nginx.ingress.kubernetes.io/server-snippet: |
      proxy_ssl_verify off;
spec:
  ingressClassName: "nginx"
  tls:
   - hosts:
     - rook-ceph.example.com
     secretName: rook-ceph.example.com
  rules:
  - host: rook-ceph.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: rook-ceph-mgr-dashboard
            port:
              name: https-dashboard
```

Customise the Ingress resource to match your cluster. Replace the example domain name `rook-ceph.example.com`
with a domain name that will resolve to your Ingress Controller (creating the DNS entry if required).

Now create the Ingress:

```console
kubectl create -f dashboard-ingress-https.yaml
```

You will see the new Ingress `rook-ceph-mgr-dashboard` created:

```console
$ kubectl -n rook-ceph get ingress
NAME                      HOSTS                      ADDRESS   PORTS     AGE
rook-ceph-mgr-dashboard   rook-ceph.example.com      80, 443   5m
```

And the new Secret for the TLS certificate:

```console
kubectl -n rook-ceph get secret rook-ceph.example.com
NAME                       TYPE                DATA      AGE
rook-ceph.example.com      kubernetes.io/tls   2         4m
```

You can now browse to `https://rook-ceph.example.com/` to log into the dashboard.
