#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source "${scriptdir}/../../build/common.sh"

init_flexvolume() {
    cat <<EOF | ssh -i `minikube ssh-key` docker@`minikube ip` -o IdentitiesOnly=yes -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no -o LogLevel=quiet 'cat - > ~/rook'
#!/bin/bash
echo -ne '{"status": "Success", "capabilities": {"attach": false}}' >&1
exit 0
EOF
    minikube ssh "chmod +x ~/rook"
    minikube ssh "sudo chown root:root ~/rook"
    minikube ssh "sudo mkdir -p /usr/libexec/kubernetes/kubelet-plugins/volume/exec/rook.io~rook"
    minikube ssh "sudo mv ~/rook /usr/libexec/kubernetes/kubelet-plugins/volume/exec/rook.io~rook"
    minikube start --memory=3000 --kubernetes-version ${KUBE_VERSION} --extra-config=apiserver.Authorization.Mode=RBAC
}

wait_for_ssh() {
    local tries=100
    while (( ${tries} > 0 )) ; do
        if `minikube ssh echo connected &> /dev/null` ; then
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
    docker save ${build_image} | (eval $(minikube docker-env) && docker load && docker tag ${build_image} ${final_image})
}

# configure dind-cluster
KUBE_VERSION=${KUBE_VERSION:-"v1.7.5"}

case "${1:-}" in
  up)
    minikube start --memory=3000 --kubernetes-version ${KUBE_VERSION} --extra-config=apiserver.Authorization.Mode=RBAC
    wait_for_ssh

    copy_image_to_cluster ${BUILD_REGISTRY}/rook-amd64 rook/rook:master
    copy_image_to_cluster ${BUILD_REGISTRY}/toolbox-amd64 rook/toolbox:master

    if [[ $KUBE_VERSION == v1.5* ]] || [[ $KUBE_VERSION == v1.6* ]] || [[ $KUBE_VERSION == v1.7* ]] ;
    then
        echo "initializing flexvolume for rook"
        init_flexvolume
    fi
    ;;
  down)
    minikube stop
    ;;
  ssh)
    echo "connecting to minikube"
    minikube ssh
    ;;
  update)
    echo "updating the rook images"
    copy_image_to_cluster ${BUILD_REGISTRY}/rook-amd64 rook/rook:master
    copy_image_to_cluster ${BUILD_REGISTRY}/toolbox-amd64 rook/toolbox:master
    ;;
  wordpress)
    echo "copying the wordpress images"
    copy_image_to_cluster mysql:5.6 mysql:5.6
    copy_image_to_cluster wordpress:4.6.1-apache wordpress:4.6.1-apache
    ;;
  helm)
    echo " copying rook image for helm"
    helm_tag="`cat _output/version`"
    copy_image_to_cluster ${BUILD_REGISTRY}/rook-amd64 rook/rook:${helm_tag}
    ;;
  clean)
    minikube delete
    ;;
  *)
    echo "usage:" >&2
    echo "  $0 up" >&2
    echo "  $0 down" >&2
    echo "  $0 clean" >&2
    echo "  $0 ssh" >&2
    echo "  $0 update" >&2
    echo "  $0 wordpress" >&2
    echo "  $0 helm" >&2
esac
