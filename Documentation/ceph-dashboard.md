---
title: Dashboard
weight: 24
indent: true
---

# Ceph Dashboard

The dashboard is a very helpful tool to give you an overview of the status of your cluster, including overall health,
status of the mon quorum, status of the mgr, osd, and other Ceph daemons, view pools and PG status, show logs for the daemons,
and more. Rook makes it simple to enable the dashboard.

![The Ceph dashboard](media/ceph-dashboard.png)

## Enable the Dashboard

The [dashboard](http://docs.ceph.com/docs/mimic/mgr/dashboard/) can be enabled with settings in the cluster CRD. The cluster CRD must have the dashboard `enabled` setting set to `true`.
This is the default setting in the example manifests.
```yaml
  spec:
    dashboard:
      enabled: true
```

The Rook operator will enable the ceph-mgr dashboard module. A K8s service will be created to expose that port inside the cluster. The ports enabled by Rook will depend
on the version of Ceph that is running:
- Luminous: Port 7000 on http
- Mimic and newer: Port 8443 on https

This example shows that port 8443 was configured for Mimic or newer.
```bash
kubectl -n rook-ceph get service
NAME                         TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)          AGE
rook-ceph-mgr                ClusterIP   10.108.111.192   <none>        9283/TCP         3h
rook-ceph-mgr-dashboard      ClusterIP   10.110.113.240   <none>        8443/TCP         3h
```

The first service is for reporting the [Prometheus metrics](ceph-monitoring.md), while the latter service is for the dashboard.
If you are on a node in the cluster, you will be able to connect to the dashboard by using either the
DNS name of the service at `https://rook-ceph-mgr-dashboard-https:8443` or by connecting to the cluster IP,
in this example at `https://10.110.113.240:8443`.

### Credentials

After you connect to the dashboard you will need to login for secure access. Rook creates a default user named
`admin` and generates a secret called `rook-ceph-dashboard-admin-password` in the namespace where rook is running.
To retrieve the generated password, you can run the following:
```
kubectl -n rook-ceph get secret rook-ceph-dashboard-password -o yaml | grep "password:" | awk '{print $2}' | base64 --decode
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
  to be false. Note that the ssl setting will be ignored in Luminous as well as
  Mimic 13.2.2 or older where it is not supported

## Viewing the Dashboard External to the Cluster

Commonly you will want to view the dashboard from outside the cluster. For example, on a development machine with the
cluster running inside minikube you will want to access the dashboard from the host.

There are several ways to expose a service that will depend on the environment you are running in.
You can use an [Ingress Controller](https://kubernetes.io/docs/concepts/services-networking/ingress/) or [other methods](https://kubernetes.io/docs/concepts/services-networking/service/#publishing-services-service-types) for exposing services such as
NodePort, LoadBalancer, or ExternalIPs.

The simplest way to expose the service in minikube or similar environment is using the NodePort to open a port on the
VM that can be accessed by the host. To create a service with the NodePort, save this yaml as `dashboard-external-https.yaml`.
(For Luminous you will need to set the `port` and `targetPort` to 7000 and connect via `http`.)

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
```bash
$ kubectl create -f dashboard-external.yaml
```

You will see the new service `rook-ceph-mgr-dashboard-external` created:
```bash
$ kubectl -n rook-ceph get service
NAME                                    TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)          AGE
rook-ceph-mgr                           ClusterIP   10.108.111.192   <none>        9283/TCP         4h
rook-ceph-mgr-dashboard                 ClusterIP   10.110.113.240   <none>        8443/TCP         4h
rook-ceph-mgr-dashboard-external-https  NodePort    10.101.209.6     <none>        8443:31176/TCP   4h
```

In this example, port `31176` will be opened to expose port `8443` from the ceph-mgr pod. Find the ip address
of the VM. If using minikube, you can run `minikube ip` to find the ip address.
Now you can enter the URL in your browser such as `https://192.168.99.110:31176` and the dashboard will appear.
