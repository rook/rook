---
title: Common Issues
weight: 11400
indent: true
---

# Common Issues

Many of these problem cases are hard to summarize down to a short phrase that adequately describes the problem. Each problem will start with a bulleted list of symptoms. Keep in mind that all symptoms may not apply depending upon the configuration of the Rook. If the majority of the symptoms are seen there is a fair chance you are experiencing that problem.

If after trying the suggestions found on this page and the problem is not resolved, the Rook team is very happy to help you troubleshoot the issues in their Slack channel. Once you have [registered for the Rook Slack](https://slack.rook.io), proceed to the General channel to ask for assistance.

## Table of Contents
- [Troubleshooting Techniques](#troubleshooting-techniques)
- [Pod using Rook storage is not running](#pod-using-rook-storage-is-not-running)
- [Cluster failing to service requests](#cluster-failing-to-service-requests)
- [Monitors are the only pods running](#monitors-are-the-only-pods-running)
- [OSD pods are failing to start](#osd-pods-are-failing-to-start)
- [OSDs are not created on my devices](#osd-pods-are-not-created-on-my-devices)
- [Node hangs after reboot](#node-hangs-after-reboot)
- [Rook Agent modprobe exec format error](#rook-agent-modprobe-exec-format-error)
- [Rook Agent rbd module missing error](#rook-agent-rbd-module-missing-error)
- [Using multiple shared filesystem (CephFS) is attempted on a kernel version older than 4.7](#using-multiple-shared-filesystem-cephfs-is-attempted-on-a-kernel-version-older-than-47)
- [Activate log to file for a particular Ceph daemon](#activate-ceph-log-on-file)

# Troubleshooting Techniques
Kubernetes status and logs are the the main resources needed to investigate issues in any Rook cluster.

## Kubernetes Tools
Kubernetes status is the first line of investigating when something goes wrong with the cluster. Here are a few artifacts that are helpful to gather:
- Rook pod status:
  - `kubectl get pod -n <cluster-namespace> -o wide`
    - e.g., `kubectl get pod -n rook-ceph -o wide`
- Logs for Rook pods
  - Logs for the operator: `kubectl logs -n <cluster-namespace> -l app=<storage-backend-operator>`
    - e.g., `kubectl logs -n rook-ceph -l app=rook-ceph-operator`
  - Logs for a specific pod: `kubectl logs -n <cluster-namespace> <pod-name>`, or a pod using a label such as mon1: `kubectl logs -n <cluster-namespace> -l <label-matcher>`
    - e.g., `kubectl logs -n rook-ceph -l mon=a`
  - Logs on a specific node to find why a PVC is failing to mount:
    - Connect to the node, then get kubelet logs (if your distro is using systemd): `journalctl -u kubelet`
  - Pods with multiple containers
    - For all containers, in order: `kubectl -n <cluster-namespace> logs <pod-name> --all-containers`
    - For a single container: `kubectl -n <cluster-namespace> logs <pod-name> -c <container-name>`
  - Logs for pods which are no longer running: `kubectl -n <cluster-namespace> logs --previous <pod-name>`

Some pods have specialized init containers, so you may need to look at logs for different containers
within the pod.
- `kubectl -n <namespace> logs <pod-name> -c <container-name>`
- Other Rook artifacts: `kubectl -n <cluster-namespace> get all`
