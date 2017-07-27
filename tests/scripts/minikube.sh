#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source "${scriptdir}/../../build/common.sh"

tarname=image.tar
tarfile=${WORK_DIR}/tests/${tarname}

ssh_opts=(
  -o StrictHostKeyChecking=no
  -o "UserKnownHostsFile /dev/null"
  -o LogLevel=quiet
)

minikube_scp() {
    local ip=$(minikube ip)
    local ssh_key=$(minikube ssh-key)
    scp "${ssh_opts[@]}" -i ${ssh_key} $1 docker@${ip}:$2
}

minikube_ssh() {
    local ip=$(minikube ip)
    local ssh_key=$(minikube ssh-key)
    ssh "${ssh_opts[@]}" -i ${ssh_key} docker@${ip} $1
}

wait_for_ssh() {
    local tries=100
    while (( ${tries} > 0 )) ; do
        if minikube_ssh "echo connected" &> /dev/null ; then
            return 0
        fi
        tries=$(( ${tries} - 1 ))
        sleep 0.1
    done
    echo ERROR: ssh did not come up >&2
    exit 1
}

copy_image_to_cluster() {
    local build_image=$1
    local final_image=$2

    echo copying ${build_image} to minikube
    mkdir -p ${WORK_DIR}/tests
    docker save -o ${tarfile} ${build_image}
    minikube_scp ${tarfile} /home/docker
    minikube_ssh "docker load -i /home/docker/${tarname}"
    minikube_ssh "docker tag ${build_image} ${final_image}"

}

# configure dind-cluster
KUBE_VERSION=${KUBE_VERSION:-"v1.6.0"}

case "${1:-}" in
  up)
    minikube start --memory=2600 --iso-url=https://s3-us-west-2.amazonaws.com/minikube-cephfs/minikube.iso --kubernetes-version ${KUBE_VERSION}
    wait_for_ssh
    copy_image_to_cluster ${BUILD_REGISTRY}/rook-amd64 rook/rook:master
    copy_image_to_cluster ${BUILD_REGISTRY}/toolbox-amd64 rook/toolbox:master

    echo setting up rbd
    minikube_scp ${scriptdir}/minikube-rbd /home/docker/minikube-rbd
    minikube_ssh "sudo cp /home/docker/minikube-rbd /bin/rbd && sudo chmod +x /bin/rbd"
    ;;
  down)
    minikube stop
    ;;
  ssh)
    echo "connecting to minikube"
    minikube_ssh
    ;;
  update)
    echo "updating the rook images"
    copy_image_to_cluster ${BUILD_REGISTRY}/rook-amd64 rook/rook:master
    copy_image_to_cluster ${BUILD_REGISTRY}/toolbox-amd64 rook/toolbox:master
    ;;
  clean)
    minikube delete
    ;;
  *)
    echo "usage:" >&2
    echo "  $0 up" >&2
    echo "  $0 down" >&2
    echo "  $0 clean" >&2
esac
