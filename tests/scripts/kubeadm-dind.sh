#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source "${scriptdir}/../../build/common.sh"

tarname=image.tar
tarfile=${WORK_DIR}/tests/${tarname}

copy_image_to_cluster() {
    local build_image=$1
    local final_image=$2

    mkdir -p ${WORK_DIR}/tests
    docker save -o ${tarfile} ${build_image}
    for c in kube-master kube-node-1 kube-node-2; do
        docker cp ${tarfile} ${c}:/
        docker exec ${c} /bin/bash -c "docker load -i /${tarname}"
        docker exec ${c} /bin/bash -c "docker tag ${build_image} ${final_image}"
    done
}

copy_rbd() {
    for c in kube-master kube-node-1 kube-node-2; do
        docker cp ${scriptdir}/dind-cluster-rbd ${c}:/bin/rbd
        docker exec ${c} /bin/bash -c "chmod +x /bin/rbd"
        docker exec ${c} /bin/bash -c "docker pull ceph/base"
    done
}

# configure dind-cluster
export EMBEDDED_CONFIG=1
export KUBECTL_DIR=${KUBEADM_DIND_DIR}
export DIND_SUBNET=10.192.0.0
export APISERVER_PORT=${APISERVER_PORT:-8080}
export NUM_NODES=${NUM_NODES:-2}
export KUBE_VERSION=${KUBE_VERSION:-"v1.6"}
export DIND_IMAGE="${DIND_IMAGE:-mirantis/kubeadm-dind-cluster:${KUBE_VERSION}}"
export CNI_PLUGIN="${CNI_PLUGIN:-bridge}"

case "${1:-}" in
  up)
    ${scriptdir}/dind-cluster.sh reup
    copy_image_to_cluster ${BUILD_REGISTRY}/rook-amd64:latest rook/rook:master
    copy_image_to_cluster ${BUILD_REGISTRY}/toolbox-amd64:latest rook/toolbox:master
    copy_rbd
    ;;
  down)
    ${scriptdir}/dind-cluster.sh down
    ;;
  clean)
    ${scriptdir}/dind-cluster.sh clean
    ;;
  *)
    echo "usage:" >&2
    echo "  $0 up" >&2
    echo "  $0 down" >&2
    echo "  $0 clean" >&2
esac
