#!/usr/bin/env bash


SCRIPT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
ROOK_EXAMPLES_DIR="${SCRIPT_ROOT}/../../deploy/examples/"

init_vars(){
    local rook_profile_name=$1
    ROOK_PROFILE_NAME=$rook_profile_name
    MINIKUBE="minikube --profile $rook_profile_name"
    KUBECTL="$MINIKUBE kubectl --"
}

wait_for_ceph_cluster() {
    echo "Waiting for ceph cluster"
    WAIT_CEPH_CLUSTER_RUNNING=20
    while ! $KUBECTL get cephclusters.ceph.rook.io -n rook-ceph -o jsonpath='{.items[?(@.kind == "CephCluster")].status.ceph.health}' | grep -q "HEALTH_OK"; do
	echo "Waiting for Ceph cluster installed"
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
        echo "kvm2"
    else
        echo "Unknown/Unsupported OS"
	exit 1
    fi
}

show_info() {
    local monitoring_enabled=$1
    DASHBOARD_PASSWORD=$($KUBECTL -n rook-ceph get secret rook-ceph-dashboard-password -o jsonpath="{['data']['password']}" | base64 --decode && echo)
    IP_ADDR=$($KUBECTL get po --selector="app=rook-ceph-mgr" -n rook-ceph --output jsonpath='{.items[*].status.hostIP}')
    PORT="$($KUBECTL -n rook-ceph -o=jsonpath='{.spec.ports[?(@.name == "dashboard")].nodePort}' get services rook-ceph-mgr-dashboard-external-http)"
    BASE_URL="http://$IP_ADDR:$PORT"
    echo "==========================="
    echo "Ceph Dashboard: "
    echo "   IP_ADDRESS: $BASE_URL"
    echo "   PASSWORD: $DASHBOARD_PASSWORD"
    if [ "$monitoring_enabled" = true ]; then
	PROMETHEUS_API_HOST="http://$(kubectl -n rook-ceph -o jsonpath='{.status.hostIP}' get pod prometheus-rook-prometheus-0):30900"
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
    $MINIKUBE start --disk-size=40g --extra-disks=3 --driver "$minikube_driver"
    eval "$($MINIKUBE docker-env)"
}

create_rook_cluster() {
    echo "Creating cluster"
    $KUBECTL apply -f crds.yaml -f common.yaml -f operator.yaml
    $KUBECTL apply -f cluster-test.yaml -f toolbox.yaml
    $KUBECTL apply -f dashboard-external-http.yaml
}

check_examples_dir() {
    CRDS_FILE="crds.yaml"
    if [ ! -e ${CRDS_FILE} ]; then
	echo "File ${ROOK_EXAMPLES_DIR}/${CRDS_FILE} does not exist. Please, provide a valid rook examples directory."
	exit 1
    fi
}

wait_for_rook_operator() {
    echo "Waiting for rook operator"
    $KUBECTL rollout status deployment rook-ceph-operator -n rook-ceph --timeout=180s
    while ! $KUBECTL get cephclusters.ceph.rook.io -n rook-ceph -o jsonpath='{.items[?(@.kind == "CephCluster")].status.phase}' | grep -q "Ready"; do
	echo "Waiting for cluster to be ready..."
	sleep 20
    done
}

enable_rook_orchestrator() {
    echo "Enabling rook orchestrator"
    $KUBECTL rollout status deployment rook-ceph-tools -n rook-ceph --timeout=30s
    $KUBECTL -n rook-ceph exec -it deploy/rook-ceph-tools -- ceph mgr module enable rook
    $KUBECTL -n rook-ceph exec -it deploy/rook-ceph-tools -- ceph orch set backend rook
    $KUBECTL -n rook-ceph exec -it deploy/rook-ceph-tools -- ceph orch status
}

enable_monitoring() {
    echo "Enabling monitoring"
    $KUBECTL apply -f https://raw.githubusercontent.com/coreos/prometheus-operator/v0.40.0/bundle.yaml
    $KUBECTL wait --for=condition=ready pod -l app.kubernetes.io/name=prometheus-operator --timeout=30s
    $KUBECTL apply -f monitoring/service-monitor.yaml
    $KUBECTL apply -f monitoring/exporter-service-monitor.yaml
    $KUBECTL apply -f monitoring/prometheus.yaml
    $KUBECTL apply -f monitoring/prometheus-service.yaml
    PROMETHEUS_API_HOST="http://$(kubectl -n rook-ceph -o jsonpath='{.status.hostIP}' get pod prometheus-rook-prometheus-0):30900"
    $KUBECTL -n rook-ceph exec -it deploy/rook-ceph-tools -- ceph dashboard set-prometheus-api-host "$PROMETHEUS_API_HOST"
}

show_usage() {
    echo ""
    echo " Usage: $(basename "$0") [-r] [-m] [-p <profile-name>] [-d /path/to/rook-examples/dir]"
    echo "  -r                Enable rook orchestrator"
    echo "  -m                Enable monitoring"
    echo "  -p <profile-name> Specify the minikube profile name"
    echo "  -d value          Path to Rook examples directory (i.e github.com/rook/rook/deploy/examples)"
}

invocation_error() {
    printf "%s\n" "$*" > /dev/stderr
    show_usage
    exit 1
}

####################################################################
################# MAIN #############################################



while getopts ":hrmfd:p:" opt; do
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
	d)
	    ROOK_EXAMPLES_DIR="$OPTARG"
	    ;;
	p)
	    minikube_profile_name="$OPTARG"
	    ;;
	\?)
	    invocation_error "Invalid option: -$OPTARG"
	    ;;
	:)
	    invocation_error "Option -$OPTARG requires an argument."
	    ;;
    esac
done

echo "Using '$ROOK_EXAMPLES_DIR' as examples directory.."

cd "$ROOK_EXAMPLES_DIR" || exit
check_examples_dir
init_vars "${minikube_profile_name:-rook}"

if [ -z "$force_minikube" ]; then
    check_minikube_exists
fi

setup_minikube_env
create_rook_cluster
wait_for_rook_operator
wait_for_ceph_cluster

if [ "$enable_rook" = true ]; then
    enable_rook_orchestrator
fi

if [ "$enable_monitoring" = true ]; then
    enable_monitoring
fi

show_info "$enable_monitoring"

####################################################################
####################################################################
