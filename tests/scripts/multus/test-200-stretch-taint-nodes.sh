#!/usr/bin/env bash
set -xeuo pipefail

kubectl taint node kind-worker kind-worker2 "storage-node=true:NoSchedule"
