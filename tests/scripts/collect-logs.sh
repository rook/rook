#!/usr/bin/env bash

set -x

# User parameters
: "${CLUSTER_NAMESPACE:="rook-ceph"}"
: "${OPERATOR_NAMESPACE:="$CLUSTER_NAMESPACE"}"
: "${ADDITIONAL_NAMESPACE:=""}"
: "${LOG_DIR:="test"}"

LOG_DIR="${LOG_DIR%/}" # remove trailing slash if necessary
mkdir -p "${LOG_DIR}"

CEPH_CMD="kubectl -n ${CLUSTER_NAMESPACE} exec deploy/rook-ceph-tools -- ceph --connect-timeout 10"

$CEPH_CMD -s >"${LOG_DIR}"/ceph-status.txt
$CEPH_CMD osd dump >"${LOG_DIR}"/ceph-osd-dump.txt
$CEPH_CMD report >"${LOG_DIR}"/ceph-report.txt
$CEPH_CMD auth ls >"${LOG_DIR}"/ceph-auth-ls.txt

NAMESPACES=("$CLUSTER_NAMESPACE")
if [[ "$OPERATOR_NAMESPACE" != "$CLUSTER_NAMESPACE" ]]; then
  NAMESPACES+=("$OPERATOR_NAMESPACE")
fi

if [[ -n "${ADDITIONAL_NAMESPACE}" ]]; then
  NAMESPACES+=("${ADDITIONAL_NAMESPACE}")
fi

for NAMESPACE in "${NAMESPACES[@]}"; do
  # each namespace is a sub-directory for easier debugging
  NS_DIR="${LOG_DIR}"/namespace-"${NAMESPACE}"
  mkdir "${NS_DIR}"

  # describe every one of the k8s resources in the namespace which rook commonly uses
  for KIND in 'pod' 'deployment' 'job' 'daemonset' 'cm' 'pvc' 'sc'; do
    kubectl -n "$NAMESPACE" get "$KIND" -o wide >"${NS_DIR}"/"$KIND"-list.txt
    for resource in $(kubectl -n "$NAMESPACE" get "$KIND" -o jsonpath='{.items[*].metadata.name}'); do
      kubectl -n "$NAMESPACE" describe "$KIND" "$resource" >"${NS_DIR}"/"$KIND"-describe--"$resource".txt

      # collect logs for pods along the way
      if [[ "$KIND" == 'pod' ]]; then
        kubectl -n "$NAMESPACE" logs --all-containers "$resource" >"${NS_DIR}"/logs--"$resource".txt
      fi
    done
  done

  # secret need `-oyaml` to read the content instead of `describe` since secrets `describe` will be encrypted.
  # so keeping it in a different block.
  for secret in $(kubectl -n "$NAMESPACE" get secrets -o jsonpath='{.items[*].metadata.name}'); do
    kubectl -n "$NAMESPACE" get -o yaml secret "$secret" >"${NS_DIR}"/secret-get--"$secret".txt
  done

  # describe every one of the custom resources in the namespace since all should be rook-related and
  # they aren't captured by 'kubectl get all'
  for CRD in $(kubectl get crds -o jsonpath='{.items[*].metadata.name}'); do
    for resource in $(kubectl -n "$NAMESPACE" get "$CRD" -o jsonpath='{.items[*].metadata.name}'); do
      crd_main_type="${CRD%%.*}" # e.g., for cephclusters.ceph.rook.io, only use 'cephclusters'
      kubectl -n "$NAMESPACE" describe "$CRD" "$resource" >>"${NS_DIR}"/"$crd_main_type"-describe--"$resource".txt
    done
  done

  # do simple 'get all' calls for resources we don't often want to look at
  kubectl get all -n "$NAMESPACE" -o wide >"${NS_DIR}"/all-wide.txt
  kubectl get all -n "$NAMESPACE" -o yaml >"${NS_DIR}"/all-yaml.txt
done

sudo lsblk | sudo tee -a "${LOG_DIR}"/lsblk.txt
journalctl -o short-precise --dmesg >"${LOG_DIR}"/dmesg.txt
journalctl >"${LOG_DIR}"/journalctl.txt
