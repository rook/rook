# Monitoring
Each Rook cluster has some built in metrics collectors/exporters for monitoring with [Prometheus](https://prometheus.io/).  To enable monitoring of Rook in your Kubernetes cluster, you can follow the steps below.
Note that these steps work best with a local Kubernetes cluster running in `Vagrant`.

## Prometheus Operator
First the Prometheus operator needs to be started in the cluster so it can watch for our requests to start monitoring Rook and respond by deploying the correct Prometheus pods and configuration.
A full explanation can be found in the [Prometheus operator repository on github](https://github.com/coreos/prometheus-operator), but the quick instructions can be found here:
```bash
kubectl apply -f https://raw.githubusercontent.com/coreos/prometheus-operator/release-0.8/bundle.yaml
```
This will start the Prometheus operator, but before moving on, wait until the operator is in the `Running` state:
```bash
kubectl get pod
```
Once the Prometheus operator is in the `Running` state, proceed to the next section.

## Prometheus Instances
With the Prometheus operator running, we can create a service monitor that will watch the Rook cluster and collect metrics regularly.
From the root of your locally cloned Rook repo, go the monitoring directory:
```bash
cd demo/kubernetes/monitoring
```

Create the service monitor as well as the Prometheus server pod and service:
```bash
kubectl create -f service-monitor.yaml
kubectl create -f prometheus.yaml
kubectl create -f prometheus-service.yaml
```

Ensure that the Prometheus server pod gets created and advances to the `Running` state before moving on:
```bash
kubectl -n rook get pod prometheus-rook-prometheus-0
```

## Prometheus Web Console
Once the Prometheus server is running, you can open a web browser and go to the URL that is output from this command:
```bash
echo "http://$(kubectl -n rook -o jsonpath={.status.hostIP} get pod prometheus-rook-prometheus-0):30900"
```

You should now see the Prometheus monitoring website.  Click on `Graph` in the top navigation bar.  In the dropdown that says ` - insert metric at cursor - `,
select any metric you would like to see, for example `ceph_cluster_used_bytes`, followed by clicking on the `Execute` button.  Below the `Execute` button, ensure
the `Graph` tab is selected and you should now see a graph of your chosen metric over time.

## Teardown
To clean up all the artifacts created by the monitoring walkthrough, copy/paste the entire block below (note that errors about resources "not found" can be ignored):
```bash
kubectl delete -f service-monitor.yaml
kubectl delete -f prometheus.yaml
kubectl delete -f prometheus-service.yaml
kubectl -n rook delete statefulset prometheus-rook-prometheus
kubectl delete -f https://raw.githubusercontent.com/coreos/prometheus-operator/release-0.8/bundle.yaml
```
Then the rest of the instructions in the [Prometheus Operator docs](https://github.com/coreos/prometheus-operator#removal) can be followed to finish cleaning up.
