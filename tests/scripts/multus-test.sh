#!/usr/bin/env bash
set -eEuo pipefail

##
## This script will install a configurable number of multus-networked daemonsets to test that your
## multus configuration and network hardware can provide enough IP addresses for Ceph.
## Configuration options are commented below.
##
## TODO: in the future, this test should also check network stability under a reasonable amount of
##       load to/from daemons.
##

## A comma-delimited list of multus networks to use for test pods
: "${MULTUS_NETWORKS:="public-net,cluster-net"}"

## The number of daemons to run per node as a test. This should reflect the maximum number of
## daemons that could run on a single node for your Ceph install. The default value assumes that one
## daemon type will run on one node, but that there will be 3 OSDs on said node. As an easy
## guideline, add one for each additional OSD beyond 3 that might run simultaneously on any node.
: "${NUM_DAEMONS_PER_NODE:=20}"

## The namespace to install multus test daemons into.
: "${VALIDATION_NAMESPACE:="rook-ceph"}"

## If you use a command other than 'kubectl' for getting/creating cluster resources, change this.
: "${KUBECTL:="kubectl"}"

## If you use a command other than 'jq' for json querying, change this.
: "${JQ:="jq"}"

ALL_DAEMONSETS=() # global for cleanup function
function main() {
  trap cleanup EXIT

  ALL_DAEMONSETS=()
  for i in $(seq "$NUM_DAEMONS_PER_NODE"); do
    daemonset_name="multus-validator-${i}"
    ALL_DAEMONSETS+=("${daemonset_name}")

    render_daemonset "${daemonset_name}" | $KUBECTL create -f -
  done

  echo "Waiting for ${#ALL_DAEMONSETS[@]} daemonsets to have pods scheduled"
  num_expected_pods=0
  for ds in "${ALL_DAEMONSETS[@]}"; do
    echo " └─ waiting for daemonset '${ds}' to have pods scheduled"
    while true; do
      num_desired="$($KUBECTL --namespace $VALIDATION_NAMESPACE get daemonset "${ds}" \
        --output jsonpath='{.status.desiredNumberScheduled}' || true)"
      if [[ -n "$num_desired" ]] && [[ "$num_desired" -gt 0 ]]; then
        num_expected_pods=$((num_expected_pods + num_desired))
        break
      fi
      sleep 1
    done
  done

  echo "Waiting for ${num_expected_pods} pods from ${#ALL_DAEMONSETS[@]} daemonsets"
  num_ready_with_multus=0
  until [[ $num_ready_with_multus -eq $num_expected_pods ]]; do
    sleep 2
    pods="$(get_validator_pods_status_and_multus)"
    num_ready_with_multus="$(echo "${pods}" | grep 'Running' | grep 'multus=true' --count)"
    echo " └─ ${num_ready_with_multus} of ${num_expected_pods} pods have multus networks"
  done

  echo "Test successful!"
}

function cleanup() {
  echo "Cleaning up daemonsets"
  set +eE # don't fail out out on errors on cleanup
  for ds in "${ALL_DAEMONSETS[@]}"; do
    $KUBECTL --namespace "$VALIDATION_NAMESPACE" delete daemonset "${ds}" &
  done
  until [[ "$(number_of_validation_pods)" -eq 0 ]]; do
    sleep 2
    echo -n '.'
  done
}

function number_of_validation_pods() {
  $KUBECTL --namespace "$VALIDATION_NAMESPACE" get pod \
    --selector app=multus-validation --no-headers | wc -l
}

# Output:
#    <name> <phase> multus=true  - if multus k8s.v1.cni.cncf.io/network-status annotation is present
#    <name> <phase> multus=false - if multus k8s.v1.cni.cncf.io/network-status annotation is NOT present
#
# Annotation presence is important because if no multus NADs are specified, pod can still be Ready.
function get_validator_pods_status_and_multus() {
  $KUBECTL --namespace "$VALIDATION_NAMESPACE" get pods --selector app=multus-validation --output json |
    jq -r '.items[]
      | [
          .metadata.name,
          .status.phase,
          (if .metadata.annotations["k8s.v1.cni.cncf.io/network-status"] then "multus=true" else "multus=false" end)
        ]
      | @tsv'
  # @tsv makes the output tab-delimited
}

function render_daemonset() {
  local daemonset_name="$1"

  cat <<EOF
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: "${daemonset_name}"
  namespace: "$VALIDATION_NAMESPACE"
  labels:
    app: multus-validation
spec:
  selector:
    matchLabels:
      name: "${daemonset_name}"
      app: multus-validation
  template:
    metadata:
      labels:
        name: "${daemonset_name}"
        app: multus-validation
      annotations:
        k8s.v1.cni.cncf.io/networks: $MULTUS_NETWORKS
    spec:
      # TODO: node selectors, tolerations, etc.
      containers:
        - name: multus-validator
          image: quay.io/fedora/fedora:latest
          command:
            - sleep
            - infinity
          resources: {}
EOF
}

main # run main
