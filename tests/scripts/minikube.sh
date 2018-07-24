#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
# shellcheck disable=SC1090
source "${scriptdir}/../../build/common.sh"

function init_flexvolume() {
    local flexname=$1

    cat <<EOF | ssh -i "$(minikube ssh-key)" "docker@$(minikube ip)" -o IdentitiesOnly=yes -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no -o LogLevel=quiet 'cat - > ~/rook'
#!/bin/bash
echo -ne '{"status": "Success", "capabilities": {"attach": false}}' >&1
exit 0
EOF
    minikube ssh "chmod +x ~/rook"
    minikube ssh "sudo chown root:root ~/rook"
    minikube ssh "sudo mkdir -p /usr/libexec/kubernetes/kubelet-plugins/volume/exec/${flexname}~rook"
    minikube ssh "sudo mv ~/rook /usr/libexec/kubernetes/kubelet-plugins/volume/exec/${flexname}~rook"
}

# workaround for kube-dns CrashLoopBackOff issue with RBAC enabled
#issue https://github.com/kubernetes/minikube/issues/1734 and https://github.com/kubernetes/minikube/issues/1722
function enable_roles_for_RBAC() {
    cat <<EOF | kubectl create -f -
# Wide open access to the cluster (mostly for kubelet)
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: cluster-writer
rules:
  - apiGroups: ["*"]
    resources: ["*"]
    verbs: ["*"]
  - nonResourceURLs: ["*"]
    verbs: ["*"]
---
# Give admin, kubelet, kube-system, kube-proxy god access
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: cluster-write
subjects:
  - kind: User
    name: admin
  - kind: User
    name: kubelet
  - kind: ServiceAccount
    name: default
    namespace: kube-system
  - kind: User
    name: kube-proxy
roleRef:
  kind: ClusterRole
  name: cluster-writer
  apiGroup: rbac.authorization.k8s.io
EOF
}

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

# configure minikube
KUBE_VERSION=${KUBE_VERSION:-"v1.10.0"}
MEMORY=${MEMORY:-"3000"}

case "${1:-}" in
  up)
    # Use kubeadm bootstrapper for 1.8+ since localkube was deprecated in 1.8
    if [[ $KUBE_VERSION == v1.7* ]] ; then
      echo "starting minikube with localkube bootstrapper"
      minikube start --memory="${MEMORY}" -b localkube --kubernetes-version "${KUBE_VERSION}" --extra-config=apiserver.Authorization.Mode=RBAC
      wait_for_ssh
      enable_roles_for_RBAC
      echo "initializing flexvolume for ceph.rook.io"
      init_flexvolume ceph.rook.io
      echo "initializing flexvolume for rook.io"
      init_flexvolume rook.io
      echo "restarting minikube"
      minikube start --memory="${MEMORY}" -b localkube --kubernetes-version "${KUBE_VERSION}" --extra-config=apiserver.Authorization.Mode=RBAC
    else
      echo "starting minikube with kubeadm bootstrapper"
      minikube start --memory="${MEMORY}" -b kubeadm --kubernetes-version "${KUBE_VERSION}"
      wait_for_ssh
    fi
    # create a link so the default dataDirHostPath will work for this environment
    minikube ssh "sudo mkdir -p /mnt/sda1/${PWD}; sudo mkdir -p $(dirname $PWD); sudo ln -s /mnt/sda1/${PWD} $(dirname $PWD)/"    
    minikube ssh "sudo mkdir /mnt/sda1/var/lib/rook;sudo ln -s /mnt/sda1/var/lib/rook /var/lib/rook"
    copy_image_to_cluster "${BUILD_REGISTRY}/ceph-amd64" rook/ceph:master
    copy_image_to_cluster "${BUILD_REGISTRY}/ceph-toolbox-amd64" rook/ceph-toolbox:master
    copy_image_to_cluster "${BUILD_REGISTRY}/cockroachdb-amd64" rook/cockroachdb:master
    copy_image_to_cluster "${BUILD_REGISTRY}/minio-amd64" rook/minio:master
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
    copy_image_to_cluster "${BUILD_REGISTRY}/ceph-amd64" rook/ceph:master
    copy_image_to_cluster "${BUILD_REGISTRY}/ceph-toolbox-amd64" rook/ceph-toolbox:master
    copy_image_to_cluster "${BUILD_REGISTRY}/cockroachdb-amd64" rook/cockroachdb:master
    copy_image_to_cluster "${BUILD_REGISTRY}/minio-amd64" rook/minio:master
    ;;
  restart)
    if check_context; then
        [ "$2" ] && regex=$2 || regex="^rook-"
        echo "Restarting Rook pods matching the regex \"$regex\" in \"rook\" namespace."
        delete_rook_pods "rook" $regex
        echo "Restarting Rook pods matching the regex \"$regex\" in \"rook-system\" namespace.."
        delete_rook_pods "rook-system" $regex
    else
      echo "To prevent accidental data loss acting only on 'minikube' context. No action is taken."
    fi
    ;;
  wordpress)
    echo "copying the wordpress images"
    copy_image_to_cluster mysql:5.6 mysql:5.6
    copy_image_to_cluster wordpress:4.6.1-apache wordpress:4.6.1-apache
    ;;
  helm)
    echo " copying rook image for helm"
    helm_tag="$(cat _output/version)"
    copy_image_to_cluster "${BUILD_REGISTRY}/ceph-amd64" "rook/ceph:${helm_tag}"
    copy_image_to_cluster "${BUILD_REGISTRY}/minio-amd64" "rook/minio:${helm_tag}"
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
    echo "  $0 restart <pod-name-regex> (the pod name is a regex to match e.g. restart ^rook-ceph-osd)" >&2
    echo "  $0 wordpress" >&2
    echo "  $0 helm" >&2
esac
