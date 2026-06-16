#!/usr/bin/env bash

set -x
if ! ensure_kubectl_can_reach_k8s; then
	echo "WARNING: kubectl cannot reach a cluster; skipping Kubernetes log collection"
	# returning success here so log collection is considered optiopnal for green CI.
	# it is only invoked upon test failure anyway.
	exit 0
fi

# User parameters
: "${CLUSTER_NAMESPACE:="rook-ceph"}"
: "${OPERATOR_NAMESPACE:="$CLUSTER_NAMESPACE"}"
: "${ADDITIONAL_NAMESPACE:=""}"
: "${LOG_DIR:="test"}"

LOG_DIR="${LOG_DIR%/}" # remove trailing slash if necessary
mkdir -p "${LOG_DIR}"


KUBECTL_CMD="kubectl -n ${CLUSTER_NAMESPACE}"

TOOLS_POD="$(${KUBECTL_CMD} get pod -l app=rook-ceph-tools -o jsonpath='{.items[0].metadata.name}')"
if [[ -z "${TOOLS_POD}" ]]; then
	echo "WARNING: rook-ceph-tools pod not found; skipping ceph log collection"
	exit 0
fi


# function to test if kubectl can reach the k8s cluster.
ensure_kubectl_can_reach_k8s() {
	echo "== kubectl diagnostics =="
	echo "KUBECONFIG=${KUBECONFIG:-<unset>}"
	kubectl config current-context 2>&1 || true
	kubectl config get-contexts 2>&1 || true
	kubectl config view --minify 2>&1 || true
	if ! kubectl version --request-timeout=10s >/dev/null 2>&1; then
		return 1
	fi
	return 0
}

# wrapper for running a ceph command in the toolbox pod with kubectl exec
ceph_exec() {
	kubectl -n "${CLUSTER_NAMESPACE}" exec "${TOOLS_POD}" -- ceph --connect-timeout 10 "$@"
}

# a retry wrapper intended for running ceph commands
# in the toolbox via kubectl exec
retry_exec() {
	local retries="${1:-5}"
	shift
	local i
	# shellcheck disable=SC2034 # i is not referenced: only used to loop.
	for i in $(seq 1 "$retries"); do
	if "$@"; then
		return 0
	fi
	sleep 5
	done
	return 1
}

retry_exec 5 ceph_exec -s >"${LOG_DIR}"/ceph-status.txt
retry_exec 5 ceph_exec osd dump >"${LOG_DIR}"/ceph-osd-dump.txt
retry_exec 5 ceph_exec report >"${LOG_DIR}"/ceph-report.txt
retry_exec 5 ceph_exec auth ls >"${LOG_DIR}"/ceph-auth-ls.txt

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
