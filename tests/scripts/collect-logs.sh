#!/usr/bin/env bash

set -x

# User parameters
: "${CLUSTER_NAMESPACE:="rook-ceph"}"
: "${OPERATOR_NAMESPACE:="$CLUSTER_NAMESPACE"}"
: "${LOG_DIR:="test"}"

LOG_DIR="${LOG_DIR%/}" # remove trailing slash if necessary
mkdir -p "${LOG_DIR}"

CEPH_CMD="kubectl -n ${CLUSTER_NAMESPACE} exec deploy/rook-ceph-tools -- ceph --connect-timeout 3"

$CEPH_CMD -s > "${LOG_DIR}"/ceph-status.txt
$CEPH_CMD osd dump > "${LOG_DIR}"/ceph-osd-dump.txt
$CEPH_CMD report > "${LOG_DIR}"/ceph-report.txt

for pod in $(kubectl -n "${CLUSTER_NAMESPACE}" get pod -o jsonpath='{.items[*].metadata.name}'); do
  kubectl -n "${CLUSTER_NAMESPACE}" describe pod "$pod" > "${LOG_DIR}"/pod-describe-"$pod".txt
done
for dep in $(kubectl -n "${CLUSTER_NAMESPACE}" get deploy -o jsonpath='{.items[*].metadata.name}'); do
  kubectl -n "${CLUSTER_NAMESPACE}" describe deploy "$dep" > "${LOG_DIR}"/deploy-describe-"$dep".txt
  kubectl -n "${CLUSTER_NAMESPACE}" log deploy "$dep" --all-containers > "${LOG_DIR}"/deploy-describe-"$dep"-log.txt
done
kubectl -n "${OPERATOR_NAMESPACE}" logs deploy/rook-ceph-operator > "${LOG_DIR}"/operator-logs.txt
kubectl -n "${OPERATOR_NAMESPACE}" get pods -o wide > "${LOG_DIR}"/operator-pods-list.txt
kubectl -n "${CLUSTER_NAMESPACE}" get pods -o wide > "${LOG_DIR}"/cluster-pods-list.txt
kubectl -n "${CLUSTER_NAMESPACE}" get jobs -o wide > "${LOG_DIR}"/cluster-jobs-list.txt
prepare_job="$(kubectl -n "${CLUSTER_NAMESPACE}" get job -l app=rook-ceph-osd-prepare --output name | awk 'FNR <= 1')" # outputs job/<name>
removal_job="$(kubectl -n "${CLUSTER_NAMESPACE}" get job -l app=rook-ceph-purge-osd --output name | awk 'FNR <= 1')" # outputs job/<name>
kubectl -n "${CLUSTER_NAMESPACE}" describe "${prepare_job}" > "${LOG_DIR}"/osd-prepare-describe.txt
kubectl -n "${CLUSTER_NAMESPACE}" logs "${prepare_job}" > "${LOG_DIR}"/osd-prepare-logs.txt
kubectl -n "${CLUSTER_NAMESPACE}" describe "${removal_job}" > "${LOG_DIR}"/osd-removal-describe.txt
kubectl -n "${CLUSTER_NAMESPACE}" logs "${removal_job}" > "${LOG_DIR}"/osd-removal-logs.txt
kubectl -n "${CLUSTER_NAMESPACE}" logs deploy/rook-ceph-osd-0 --all-containers > "${LOG_DIR}"/rook-ceph-osd-0-logs.txt
kubectl -n "${CLUSTER_NAMESPACE}" logs deploy/rook-ceph-osd-1 --all-containers > "${LOG_DIR}"/rook-ceph-osd-1-logs.txt
kubectl get all -n "${OPERATOR_NAMESPACE}" -o wide > "${LOG_DIR}"/operator-wide.txt
kubectl get all -n "${OPERATOR_NAMESPACE}" -o wide > "${LOG_DIR}"/operator-yaml.txt
kubectl get all -n "${CLUSTER_NAMESPACE}" -o wide > "${LOG_DIR}"/cluster-wide.txt
kubectl get all -n "${CLUSTER_NAMESPACE}" -o yaml > "${LOG_DIR}"/cluster-yaml.txt
kubectl -n "${CLUSTER_NAMESPACE}" get cephcluster -o yaml > "${LOG_DIR}"/cephcluster.txt
sudo lsblk | sudo tee -a "${LOG_DIR}"/lsblk.txt
journalctl -o short-precise --dmesg > "${LOG_DIR}"/dmesg.txt
journalctl > "${LOG_DIR}"/journalctl.txt
