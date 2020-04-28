#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
REPO_DIR="${REPO_DIR:-${scriptdir}/../../.cache/k8s-vagrant-multi-node/}"
# shellcheck disable=SC1090
source "${scriptdir}/../../build/common.sh"

function init() {
    if [ ! -d "${REPO_DIR}" ]; then
        echo "k8s-vagrant-multi-node not found in rook cache dir. Cloning.."
        mkdir -p "${REPO_DIR}"
        git clone https://github.com/galexrt/k8s-vagrant-multi-node.git "${REPO_DIR}"
        # checkout latest tag of the repo initially after clone
        git -C "${REPO_DIR}" checkout "$(git -C "${REPO_DIR}" describe --tags `git rev-list --tags --max-count=1`)"
    else
        git -C "${REPO_DIR}" pull || { echo "git pull failed with exit code $?. continuing as the repo is already there ..."; }
    fi
}

# Deletes pods with 'rook-' prefix. Namespace is expected as the first argument
function delete_rook_pods() {
    for P in $(kubectl get pods -n "$1" | awk "/$2/ {print \$1}"); do
        kubectl delete pod "$P" -n "$1"
    done
}

# current kubectl context == minikube, returns boolean
function check_context() {
    if [ "$(kubectl config view 2>/dev/null | awk '/current-context/ {print $NF}')" = "minikube" ]; then
        return 0
    fi

    return 1
}

function copy_image_to_cluster() {
    local build_image=$1
    local final_image=$2
    make load-image IMG="${build_image}" TAG="${final_image}"
}

function copy_images() {
    if [[ "$1" == "" || "$1" == "ceph" ]]; then
      echo "copying ceph images"
      copy_image_to_cluster "${BUILD_REGISTRY}/ceph-amd64" rook/ceph:master
      copy_image_to_cluster ceph/ceph:v13 ceph/ceph:v13
    fi

    if [[ "$1" == "" || "$1" == "cockroachdb" ]]; then
      echo "copying cockroachdb image"
      copy_image_to_cluster "${BUILD_REGISTRY}/cockroachdb-amd64" rook/cockroachdb:master
    fi

    if [[ "$1" == "" || "$1" == "cassandra" ]]; then
      echo "copying cassandra image"
      copy_image_to_cluster "${BUILD_REGISTRY}/cassandra-amd64" rook/cassandra:master
    fi

    if [[ "$1" == "" || "$1" == "nfs" ]]; then
        echo "copying nfs image"
        copy_image_to_cluster "${BUILD_REGISTRY}/nfs-amd64" rook/nfs:master
    fi
}

init

cd "${REPO_DIR}" || { echo "failed to access k8s-vagrant-multi-node dir ${REPO_DIR}. exiting."; exit 1; }

case "${1:-}" in
    status)
        make status
    ;;
    up)
        make up
        copy_images "${2}"
    ;;
    update)
        copy_images "${2}"
    ;;
    restart)
        if check_context; then
            [ "$2" ] && regex=$2 || regex="^rook-"
            echo "Restarting Rook pods matching the regex \"$regex\" in the following namespaces.."
            for ns in $(kubectl get ns -o name | grep '^rook-'); do
                echo "-> $ns"
                delete_rook_pods "$ns" $regex
            done
        else
            echo "To prevent accidental data loss acting only on 'minikube' context. No action is taken."
        fi
    ;;
    helm)
        echo " copying rook image for helm"
        helm_tag="$(cat _output/version)"
        copy_image_to_cluster "${BUILD_REGISTRY}/ceph-amd64" "rook/ceph:${helm_tag}"
        ;;
    clean)
        make clean
    ;;
    shell)
        "${SHELL}"
    ;;
    *)
        echo "usage:" >&2
        echo "  $0 status" >&2
        echo "  $0 up [ceph | cockroachdb | cassandra | nfs]" >&2
        echo "  $0 update" >&2
        echo "  $0 restart" >&2
        echo "  $0 helm" >&2
        echo "  $0 clean" >&2
        echo "  $0 shell - Open a '${SHELL}' shell in the cloned k8s-vagrant-multi-node project." >&2
        echo "  $0 help" >&2
    ;;
esac
