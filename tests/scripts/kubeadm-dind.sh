#!/bin/bash +e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source "${scriptdir}/../../build/common.sh"

tarname=image.tar
tarfile=${WORK_DIR}/tests/${tarname}

copy_image_to_cluster() {
    local final_image=$1

    mkdir -p ${WORK_DIR}/tests
    docker save -o ${tarfile} ${final_image}
    for c in kube-master kube-node-1 kube-node-2; do
        docker cp ${tarfile} ${c}:/
        docker exec ${c} /bin/bash -c "docker load -i /${tarname}"
    done
}

copy_rbd() {
    for c in kube-master kube-node-1 kube-node-2; do
        docker cp ${scriptdir}/dind-cluster-rbd ${c}:/bin/rbd
        docker exec ${c} /bin/bash -c "chmod +x /bin/rbd"
        # hack for Azure, after vm is started first docker pull command fails intermittently
        local maxRetry=3
        local cur=1
        while [ $cur -le $maxRetry ]; do
            docker exec ${c} /bin/bash -c "docker pull ceph/base"
            if [ $? -eq 0 ]; then
                break
            fi
            sleep 1
            ((++cur))
        done
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
        ${scriptdir}/makeTestImages.sh save amd64 || true
        copy_image_to_cluster rook/ceph:master
        set +e
        copy_image_to_cluster ceph/base ceph/base:latest
        set -e
        copy_rbd
        ;;
    down)
        ${scriptdir}/dind-cluster.sh down
        ;;
    clean)
        ${scriptdir}/dind-cluster.sh clean
        ;;
    update)
        copy_image_to_cluster rook/ceph:master
        ;;
    wordpress)
        copy_image_to_cluster mysql:5.6
        copy_image_to_cluster wordpress:4.6.1-apache
        ;;
    *)
        echo "usage:" >&2
        echo "  $0 up" >&2
        echo "  $0 down" >&2
        echo "  $0 clean" >&2
        echo "  $0 update" >&2
        echo "  $0 wordpress" >&2
esac
