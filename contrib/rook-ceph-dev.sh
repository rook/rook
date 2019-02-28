#!/usr/bin/env bash
set -e

# Use Docker environment of Minikube
eval "$(minikube docker-env)"


#############
# VARIABLES #
#############

scriptdir="$( cd "$( dirname "$(readlink -f "${BASH_SOURCE[0]}")" )" && pwd )"
# shellcheck disable=SC1090
source "${scriptdir}/../build/common.sh"
CEPH_IMAGE="${BUILD_REGISTRY}/ceph-amd64"


#############
# FUNCTIONS #
#############
function delete_rook {
    # allow these to fail so we can run them multiple times
    set +e
    kubectl delete -f .idea/cluster.yaml
    kubectl delete -f .idea/operator.yaml
    minikube ssh "sudo rm -rf /mnt/vda1/rook/ /var/lib/rook && sudo mkdir /mnt/vda1/rook/ && sudo ln -sf /mnt/vda1/rook/ /var/lib/"
    set -e
    # sleep a bit so ressources are truly gone
    sleep 5
}

function build_rook {
    local ceph_base_image=$1
    make clean

    if [[ -n $1 ]]; then
        make -j4 BASEIMAGE="$ceph_base_image" IMAGES='ceph' build
        return
    fi

    make -j4 IMAGES='ceph' build
}

function docker_tag {
    docker tag "$CEPH_IMAGE" rook/ceph:master
}

function create_rook {
    set +e
    kubectl create -f .idea/operator.yaml
    kubectl create -f .idea/cluster.yaml
    set -e
}

function status_rook {
    kubectl get pods -n rook-ceph-system
    kubectl get pods -n rook-ceph
}

function tail_operator_logs {
    kubectl -n rook-ceph-system logs -f "$(kubectl get pods -n rook-ceph-system | awk '/rook-ceph-operator/ {print $1}')"
}

function enter_operator {
    kubectl -n rook-ceph-system exec -ti "$(kubectl get pods -n rook-ceph-system | awk '/rook-ceph-operator/ {print $1}')" bash
}

function docker_images {
    docker images
}


########
# MAIN #
########
case "${1:-}" in
  up)
    create_rook
    ;;
  down)
    delete_rook
    ;;
  build)
    build_rook "$2"
    docker_tag
    ;;
  test)
    make test
    ;;
  status)
    status_rook
    ;;
  oplog)
    tail_operator_logs
    ;;
  opexec)
    enter_operator
    ;;
  imgls)
    docker_images
    ;;
  *)
    echo "usage:" >&2
    echo "for ease of use symlink me to /usr/local/bin"
    echo "  $0 up (Deploying operator and Ceph cluster)" >&2
    echo "  $0 down (Tiering down Rook entirely (operator + Ceph cluster)" >&2
    echo "  $0 build [ceph_base_image]" >&2
    echo "  $0 test (Running Rook unit tests)" >&2
    echo "  $0 status (Operator and cluster status)" >&2
    echo "  $0 oplog (Tail operator logs-" >&2
    echo "  $0 opexec (Exec into the operator)" >&2
    echo "  $0 imgls (List docker images built)" >&2
esac
