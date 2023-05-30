#!/bin/bash
set -xeo pipefail

# initially copied from https://github.com/k8snetworkplumbingwg/whereabouts

MULTUS_DAEMONSET_URL="https://raw.githubusercontent.com/k8snetworkplumbingwg/multus-cni/master/deployments/multus-daemonset.yml"
RETRY_MAX=10
INTERVAL=10
TIMEOUT=60
TIMEOUT_K8="120s"

retry() {
  local status=0
  local retries=${RETRY_MAX:=5}
  local delay=${INTERVAL:=5}
  local to=${TIMEOUT:=20}
  cmd="$*"

  while [ $retries -gt 0 ]; do
    status=0
    timeout $to bash -c "echo $cmd && $cmd" || status=$?
    if [ $status -eq 0 ]; then
      break
    fi
    echo "Exit code: '$status'. Sleeping '$delay' seconds before retrying"
    sleep $delay
    let retries--
  done
  return $status
}

echo "#### set up multus ####"

echo " ## wait for coreDNS"
kubectl -n kube-system wait --for=condition=available deploy/coredns --timeout=$TIMEOUT_K8

echo "## install multus"
retry kubectl create -f "${MULTUS_DAEMONSET_URL}"
kubectl -n kube-system wait --for=condition=ready -l name="multus" pod --timeout=$TIMEOUT_K8

echo "## install CNIs"
retry kubectl create -f "https://raw.githubusercontent.com/k8snetworkplumbingwg/whereabouts/master/hack/cni-install.yml"
kubectl -n kube-system wait --for=condition=ready -l name="cni-plugins" pod --timeout=$TIMEOUT_K8

echo "## install whereabouts"
kubectl create \
  -f https://raw.githubusercontent.com/k8snetworkplumbingwg/whereabouts/master/doc/crds/daemonset-install.yaml \
  -f https://raw.githubusercontent.com/k8snetworkplumbingwg/whereabouts/master/doc/crds/whereabouts.cni.cncf.io_ippools.yaml \
  -f https://raw.githubusercontent.com/k8snetworkplumbingwg/whereabouts/master/doc/crds/whereabouts.cni.cncf.io_overlappingrangeipreservations.yaml
kubectl -n kube-system wait --for=condition=ready -l app=whereabouts pod --timeout=$TIMEOUT_K8

echo "#### set up multus done ####"
