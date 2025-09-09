#!/usr/bin/env bash

DEFAULT_NS="rook-ceph"
CLUSTER_FILES="common.yaml operator.yaml cluster-test.yaml cluster-on-pvc-minikube.yaml dashboard-external-http.yaml toolbox.yaml csi-operator.yaml"
MONITORING_FILES="monitoring/prometheus.yaml monitoring/service-monitor.yaml monitoring/exporter-service-monitor.yaml monitoring/prometheus-service.yaml monitoring/rbac.yaml"
SCRIPT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)

# Script arguments: new arguments must be added here (following the same format)
export MINIKUBE_NODES="${MINIKUBE_NODES:=1}" ## Specify the minikube number of nodes to create
export MINIKUBE_DISK_SIZE="${MINIKUBE_DISK_SIZE:=40g}" ## Specify the minikube disk size
export MINIKUBE_EXTRA_DISKS="${MINIKUBE_EXTRA_DISKS:=3}" ## Specify the minikube number of extra disks
export ROOK_PROFILE_NAME="${ROOK_PROFILE_NAME:=rook}" ## Specify the minikube profile name
export ROOK_CLUSTER_NS="${ROOK_CLUSTER_NS:=$DEFAULT_NS}" ## CephCluster namespace
export ROOK_OPERATOR_NS="${ROOK_OPERATOR_NS:=$DEFAULT_NS}" ## Rook operator namespace (if different from CephCluster namespace)
export ROOK_EXAMPLES_DIR="${ROOK_EXAMPLES_DIR:="$SCRIPT_ROOT"/../../deploy/examples}" ## Path to Rook examples directory (i.e github.com/rook/rook/deploy/examples)
export ROOK_CLUSTER_SPEC_FILE="${ROOK_CLUSTER_SPEC_FILE:=cluster-test.yaml}" ## CephCluster manifest file

init_vars(){
    MINIKUBE="minikube --profile $ROOK_PROFILE_NAME"
    KUBECTL="$MINIKUBE kubectl --"

    echo "Using '$(realpath "$ROOK_EXAMPLES_DIR")' as examples directory.."
    echo "Using '$ROOK_CLUSTER_SPEC_FILE' as cluster spec file.."
    echo "Using '$ROOK_PROFILE_NAME' as minikube profile.."
    echo "Using '$ROOK_CLUSTER_NS' as cluster namespace.."
    echo "Using '$ROOK_OPERATOR_NS' as operator namespace.."
}

update_namespaces() {
    if [ "$ROOK_CLUSTER_NS" != "$DEFAULT_NS" ] || [ "$ROOK_OPERATOR_NS" != "$DEFAULT_NS" ]; then
	for file in $CLUSTER_FILES $MONITORING_FILES; do
	    echo "Updating namespace on $file"
	    sed -i.bak \
		-e "s/\(.*\):.*# namespace:operator/\1: $ROOK_OPERATOR_NS # namespace:operator/g" \
		-e "s/\(.*\):.*# namespace:cluster/\1: $ROOK_CLUSTER_NS # namespace:cluster/g" \
		-e "s/\(.*serviceaccount\):.*:\(.*\) # serviceaccount:namespace:operator/\1:$ROOK_OPERATOR_NS:\2 # serviceaccount:namespace:operator/g" \
		-e "s/\(.*serviceaccount\):.*:\(.*\) # serviceaccount:namespace:cluster/\1:$ROOK_CLUSTER_NS:\2 # serviceaccount:namespace:cluster/g" \
		-e "s/\(.*\): [-_A-Za-z0-9]*\.\(.*\) # csi-provisioner-name/\1: $ROOK_OPERATOR_NS.\2 # csi-provisioner-name/g" \
		-e "s/\(.*\): [-_A-Za-z0-9]*\.\(.*\) # driver:namespace:cluster/\1: $ROOK_CLUSTER_NS.\2 # driver:namespace:cluster/g" \
		"$file"
	done
    fi
}

wait_for_ceph_cluster() {
    echo "Waiting for ceph cluster to enter HEALTH_OK"
    WAIT_CEPH_CLUSTER_RUNNING=20
    while ! $KUBECTL get cephclusters.ceph.rook.io -n "$ROOK_CLUSTER_NS" -o jsonpath='{.items[?(@.kind == "CephCluster")].status.ceph.health}' | grep -q "HEALTH_OK"; do
	echo "Waiting for Ceph cluster to enter HEALTH_OK"
	sleep ${WAIT_CEPH_CLUSTER_RUNNING}
    done
    echo "Ceph cluster installed and running"
}

get_minikube_driver() {
    os=$(uname)
    architecture=$(uname -m)
    if [[ "$os" == "Darwin" ]]; then
        if [[ "$architecture" == "x86_64" ]]; then
            echo "hyperkit"
        elif [[ "$architecture" == "arm64" ]]; then
            echo "qemu"
        else
            echo "Unknown Architecture on Apple OS"
	    exit 1
        fi
    elif [[ "$os" == "Linux" ]]; then
        echo "qemu2"
    else
        echo "Unknown/Unsupported OS"
	exit 1
    fi
}

show_info() {
    local monitoring_enabled=$1
    DASHBOARD_PASSWORD=$($KUBECTL -n "$ROOK_CLUSTER_NS" get secret rook-ceph-dashboard-password -o jsonpath="{['data']['password']}" | base64 --decode && echo)
    DASHBOARD_END_POINT=$($MINIKUBE service rook-ceph-mgr-dashboard-external-http -n "$ROOK_CLUSTER_NS" --url)
    BASE_URL="$DASHBOARD_END_POINT"
    echo "==========================="
    echo "Ceph Dashboard:"
    echo "   IP_ADDR  : $BASE_URL"
    echo "   USER     : admin"
    echo "   PASSWORD : $DASHBOARD_PASSWORD"
    if [ "$monitoring_enabled" = true ]; then
	PROMETHEUS_API_HOST="http://$(kubectl -n "$ROOK_CLUSTER_NS" -o jsonpath='{.status.hostIP}' get pod prometheus-rook-prometheus-0):30900"
    echo "Prometheus Dashboard: "
    echo "   API_HOST: $PROMETHEUS_API_HOST"
    fi
    echo "==========================="
    echo " "
    echo " *** To start using your rook cluster please set the following env: "
    echo " "
    echo "   > eval \$($MINIKUBE docker-env)"
    echo "   > alias kubectl=\"$KUBECTL"\"
    echo " "
    echo " *** To access the new cluster with k9s: "
    echo " "
    echo "   > k9s --context $ROOK_PROFILE_NAME"
    echo " "
}

check_minikube_exists() {
    echo "Checking minikube profile '$ROOK_PROFILE_NAME'..."
    if minikube profile list -l 2> /dev/null | grep -qE "\s$ROOK_PROFILE_NAME\s"; then
        echo "A minikube profile '$ROOK_PROFILE_NAME' already exists, please use -f to force the cluster creation."
	exit 1
    fi
}

setup_minikube_env() {
    minikube_driver="$(get_minikube_driver)"
    echo "Setting up minikube env for profile '$ROOK_PROFILE_NAME' (using $minikube_driver driver)"
    $MINIKUBE delete
    $MINIKUBE start --disk-size="$MINIKUBE_DISK_SIZE" --extra-disks="$MINIKUBE_EXTRA_DISKS" --driver "$minikube_driver" -n "$MINIKUBE_NODES" $ROOK_MINIKUBE_EXTRA_ARGS
    eval "$($MINIKUBE docker-env)"
}

create_rook_cluster() {
    echo "Creating cluster"
    # create operator namespace if it doesn't exist
    if ! kubectl get namespace "$ROOK_OPERATOR_NS" &> /dev/null; then
	kubectl create namespace "$ROOK_OPERATOR_NS"
    fi
    $KUBECTL apply -f crds.yaml -f common.yaml -f operator.yaml -f csi-operator.yaml
    $KUBECTL apply -f "$ROOK_CLUSTER_SPEC_FILE" -f toolbox.yaml
    $KUBECTL apply -f dashboard-external-http.yaml
}

change_to_examples_dir() {
    if [ ! -e "$ROOK_EXAMPLES_DIR" ]; then
	echo "Examples directory '$ROOK_EXAMPLES_DIR' does not exist. Please, provide a valid rook examples directory."
	exit 1
    fi

    CRDS_FILE_PATH=$(realpath "$ROOK_EXAMPLES_DIR/crds.yaml")
    if [ ! -e "$CRDS_FILE_PATH" ]; then
	echo "File '$CRDS_FILE_PATH' does not exist. Please, provide a valid rook examples directory."
	exit 1
    fi

    ROOK_CLUSTER_SPEC_PATH=$(realpath "$ROOK_EXAMPLES_DIR/$ROOK_CLUSTER_SPEC_FILE")
    if [ ! -e "$ROOK_CLUSTER_SPEC_PATH" ]; then
	echo "File '$ROOK_CLUSTER_SPEC_PATH' does not exist. Please, provide a valid cluster spec file."
	exit 1
    fi

    cd "$ROOK_EXAMPLES_DIR" || exit
}

wait_for_rook_operator() {
    echo "Waiting for rook operator..."
    $KUBECTL rollout status deployment rook-ceph-operator -n "$ROOK_OPERATOR_NS" --timeout=180s
    while ! $KUBECTL get cephclusters.ceph.rook.io -n "$ROOK_CLUSTER_NS" -o jsonpath='{.items[?(@.kind == "CephCluster")].status.phase}' | grep -q "Ready"; do
	echo "Waiting for ceph cluster to become ready..."
	sleep 20
    done
}

enable_rook_orchestrator() {
    echo "Enabling rook orchestrator"
    $KUBECTL rollout status deployment rook-ceph-tools -n "$ROOK_CLUSTER_NS" --timeout=90s
    $KUBECTL -n "$ROOK_CLUSTER_NS" exec -it deploy/rook-ceph-tools -- ceph mgr module enable rook
    $KUBECTL -n "$ROOK_CLUSTER_NS" exec -it deploy/rook-ceph-tools -- ceph orch set backend rook
    $KUBECTL -n "$ROOK_CLUSTER_NS" exec -it deploy/rook-ceph-tools -- ceph orch status
}

enable_monitoring() {
    echo "Enabling monitoring"
    $KUBECTL create -f https://raw.githubusercontent.com/coreos/prometheus-operator/v0.82.0/bundle.yaml
    $KUBECTL wait --for=condition=ready pod -l app.kubernetes.io/name=prometheus-operator --timeout=30s
    $KUBECTL apply -f monitoring/rbac.yaml
    $KUBECTL apply -f monitoring/service-monitor.yaml
    $KUBECTL apply -f monitoring/exporter-service-monitor.yaml
    $KUBECTL apply -f monitoring/prometheus.yaml
    $KUBECTL apply -f monitoring/prometheus-service.yaml
    PROMETHEUS_API_HOST="http://$(kubectl -n "$ROOK_CLUSTER_NS" -o jsonpath='{.status.hostIP}' get pod prometheus-rook-prometheus-0):30900"
    $KUBECTL -n "$ROOK_CLUSTER_NS" exec -it deploy/rook-ceph-tools -- ceph dashboard set-prometheus-api-host "$PROMETHEUS_API_HOST"
}

show_usage() {
    echo ""
    echo "Usage: [ARG=VALUE]... $(basename "$0") [-f] [-r] [-m]"
    echo "  -f     Force cluster creation by deleting minikube profile"
    echo "  -r     Enable rook orchestrator"
    echo "  -m     Enable monitoring"
    echo "  Args:"
    sed -n -E "s/^export (.*)=\".*:=.*\" ## (.*)/    \1 (\\$\1):  \2/p;" "$SCRIPT_ROOT"/"$(basename "$0")" | envsubst
    echo ""
}

invocation_error() {
    printf "%s\n" "$*" > /dev/stderr
    show_usage
    exit 1
}

####################################################################
################# MAIN #############################################

while getopts ":hrmf" opt; do
    case $opt in
	h)
	    show_usage
	    exit 0
	    ;;
	r)
	    enable_rook=true
	    ;;
	m)
	    enable_monitoring=true
	    ;;
	f)
	    force_minikube=true
	    ;;
	\?)
	    invocation_error "Invalid option: -$OPTARG"
	    ;;
	:)
	    invocation_error "Option -$OPTARG requires an argument."
	    ;;
    esac
done

# initialization zone
init_vars
change_to_examples_dir
[ -z "$force_minikube" ] && check_minikube_exists
update_namespaces

# cluster creation zone
setup_minikube_env
create_rook_cluster
wait_for_rook_operator
wait_for_ceph_cluster

# final tweaks and ceph cluster tuning
[ "$enable_rook" = true ] && enable_rook_orchestrator
[ "$enable_monitoring" = true ] && enable_monitoring
show_info "$enable_monitoring"

####################################################################
####################################################################
