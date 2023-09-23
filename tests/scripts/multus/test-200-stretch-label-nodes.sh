#!/usr/bin/env bash
set -xeuo pipefail

kubectl label node kind-control-plane topology.kubernetes.io/zone=arbiter
kubectl label node kind-worker topology.kubernetes.io/zone=dc1 "storage-node=true"
kubectl label node kind-worker2 topology.kubernetes.io/zone=dc2 "storage-node=true"
kubectl label node kind-worker3 topology.kubernetes.io/zone=dc2
