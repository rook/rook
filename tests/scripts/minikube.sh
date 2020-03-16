#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
# shellcheck disable=SC1090
source "${scriptdir}/../../build/common.sh"

function wait_for_ssh() {
    local tries=100
    while (( tries > 0 )) ; do
        if minikube ssh echo connected &> /dev/null ; then
            return 0
        fi
        tries=$(( tries - 1 ))
        sleep 0.1
    done
    echo ERROR: ssh did not come up >&2
    exit 1
}

function copy_image_to_cluster() {
    local build_image=$1
    local final_image=$2
    docker save "${build_image}" | (eval "$(minikube docker-env --shell bash)" && docker load && docker tag "${build_image}" "${final_image}")
}

function copy_images() {
    if [[ "$1" == "" || "$1" == "ceph" ]]; then
      echo "copying ceph images"
      copy_image_to_cluster "${BUILD_REGISTRY}/ceph-amd64" rook/ceph:master
      # uncomment to push the nautilus image when needed
      #copy_image_to_cluster ceph/ceph:v14 ceph/ceph:v14
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

    if [[ "$1" == "" || "$1" == "yugabytedb" ]]; then
      echo "copying yugabytedb image"
      copy_image_to_cluster "${BUILD_REGISTRY}/yugabytedb-amd64" rook/yugabytedb:master
    fi
}

# configure minikube
MEMORY=${MEMORY:-"3000"}

# use vda1 instead of sda1 when running with the libvirt driver
VM_DRIVER=$(minikube config get vm-driver 2>/dev/null || echo "virtualbox")
if [[ "$VM_DRIVER" == "kvm2" ]]; then
  DISK="vda1"
else
  DISK="sda1"
fi

case "${1:-}" in
  up)
    echo "starting minikube with kubeadm bootstrapper"
    minikube start --memory="${MEMORY}" -b kubeadm --vm-driver="${VM_DRIVER}"
    wait_for_ssh
    # create a link so the default dataDirHostPath will work for this environment
    minikube ssh "sudo mkdir -p /mnt/${DISK}/rook/ && sudo ln -sf /mnt/${DISK}/rook/ /var/lib/"
    copy_images "$2"
    ;;
  down)
    minikube stop
    ;;
  ssh)
    echo "connecting to minikube"
    minikube ssh
    ;;
  update)
    copy_images "$2"
    ;;
  wordpress)
    echo "copying the wordpress images"
    copy_image_to_cluster mysql:5.6 mysql:5.6
    copy_image_to_cluster wordpress:4.6.1-apache wordpress:4.6.1-apache
    ;;
  cockroachdb-loadgen)
    echo "copying the cockroachdb loadgen images"
    copy_image_to_cluster cockroachdb/loadgen-kv:0.1 cockroachdb/loadgen-kv:0.1
    ;;
  helm)
    echo " copying rook image for helm"
    helm_tag="$(cat _output/version)"
    copy_image_to_cluster "${BUILD_REGISTRY}/ceph-amd64" "rook/ceph:${helm_tag}"
    ;;
  clean)
    minikube delete
    ;;
  *)
    echo "usage:" >&2
    echo "  $0 up [ceph | cockroachdb | cassandra | nfs | yugabytedb]" >&2
    echo "  $0 down" >&2
    echo "  $0 clean" >&2
    echo "  $0 ssh" >&2
    echo "  $0 update [ceph | cockroachdb | cassandra | nfs | yugabytedb]" >&2
    echo "  $0 wordpress" >&2
    echo "  $0 cockroachdb-loadgen" >&2
    echo "  $0 helm" >&2
esac
