---
title: Toolbox
weight: 11100
indent: true
---

# Rook Toolbox

The Rook toolbox is a container with common tools used for rook debugging and testing.
The toolbox is based on CentOS, so more tools of your choosing can be easily installed with `yum`.

The toolbox can be run in two modes:
1. [Interactive](#interactive-toolbox): Start a toolbox pod where you can connect and execute Ceph commands from a shell
2. [One-time job](#toolbox-job): Run a script with Ceph commands and collect the results from the job log

> Prerequisite: Before running the toolbox you should have a running Rook cluster deployed (see the [Quickstart Guide](quickstart.md)).

## Interactive Toolbox

The rook toolbox can run as a deployment in a Kubernetes cluster where you can connect and
run arbitrary Ceph commands.

Save the tools spec as `toolbox.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rook-ceph-tools
  namespace: rook-ceph
  labels:
    app: rook-ceph-tools
spec:
  replicas: 1
  selector:
    matchLabels:
      app: rook-ceph-tools
  template:
    metadata:
      labels:
        app: rook-ceph-tools
    spec:
      dnsPolicy: ClusterFirstWithHostNet
      containers:
      - name: rook-ceph-tools
        image: rook/ceph:master
        command: ["/tini"]
        args: ["-g", "--", "/usr/local/bin/toolbox.sh"]
        imagePullPolicy: IfNotPresent
        env:
          - name: ROOK_CEPH_USERNAME
            valueFrom:
              secretKeyRef:
                name: rook-ceph-mon
                key: ceph-username
          - name: ROOK_CEPH_SECRET
            valueFrom:
              secretKeyRef:
                name: rook-ceph-mon
                key: ceph-secret
        volumeMounts:
          - mountPath: /etc/ceph
            name: ceph-config
          - name: mon-endpoint-volume
            mountPath: /etc/rook
      volumes:
        - name: mon-endpoint-volume
          configMap:
            name: rook-ceph-mon-endpoints
            items:
            - key: data
              path: mon-endpoints
        - name: ceph-config
          emptyDir: {}
      tolerations:
        - key: "node.kubernetes.io/unreachable"
          operator: "Exists"
          effect: "NoExecute"
          tolerationSeconds: 5
```

Launch the rook-ceph-tools pod:

```console
kubectl create -f toolbox.yaml
```

Wait for the toolbox pod to download its container and get to the `running` state:

```console
kubectl -n rook-ceph rollout status deploy/rook-ceph-tools
```

Once the rook-ceph-tools pod is running, you can connect to it with:

```console
kubectl -n rook-ceph exec -it deploy/rook-ceph-tools -- bash
```

All available tools in the toolbox are ready for your troubleshooting needs.

**Example**:

* `ceph status`
* `ceph osd status`
* `ceph df`
* `rados df`

When you are done with the toolbox, you can remove the deployment:

```console
kubectl -n rook-ceph delete deploy/rook-ceph-tools
```

## Toolbox Job

If you want to run Ceph commands as a one-time operation and collect the results later from the
logs, you can run a script as a Kubernetes Job. The toolbox job will run a script that is embedded
in the job spec. The script has the full flexibility of a bash script.

In this example, the `ceph status` command is executed when the job is created.

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: rook-ceph-toolbox-job
  namespace: rook-ceph
  labels:
    app: ceph-toolbox-job
spec:
  template:
    spec:
      initContainers:
      - name: config-init
        image: rook/ceph:master
        command: ["/usr/local/bin/toolbox.sh"]
        args: ["--skip-watch"]
        imagePullPolicy: IfNotPresent
        env:
        - name: ROOK_CEPH_USERNAME
          valueFrom:
            secretKeyRef:
              name: rook-ceph-mon
              key: ceph-username
        - name: ROOK_CEPH_SECRET
          valueFrom:
            secretKeyRef:
              name: rook-ceph-mon
              key: ceph-secret
        volumeMounts:
        - mountPath: /etc/ceph
          name: ceph-config
        - name: mon-endpoint-volume
          mountPath: /etc/rook
      containers:
      - name: script
        image: rook/ceph:master
        volumeMounts:
        - mountPath: /etc/ceph
          name: ceph-config
          readOnly: true
        command:
        - "bash"
        - "-c"
        - |
          # Modify this script to run any ceph, rbd, radosgw-admin, or other commands that could
          # be run in the toolbox pod. The output of the commands can be seen by getting the pod log.
          #
          # example: print the ceph status
          ceph status
      volumes:
      - name: mon-endpoint-volume
        configMap:
          name: rook-ceph-mon-endpoints
          items:
          - key: data
            path: mon-endpoints
      - name: ceph-config
        emptyDir: {}
      restartPolicy: Never
```

Create the toolbox job:

```console
kubectl create -f toolbox-job.yaml
```

After the job completes, see the results of the script:

```console
kubectl -n rook-ceph logs -l job-name=rook-ceph-toolbox-job
```
