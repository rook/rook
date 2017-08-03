#!/bin/bash +e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

tarname=image.tar
tarfile=${WORK_DIR}/tests/${tarname}

export KUBE_VERSION=${KUBE_VERSION:-"v1.7.2"}


install() {

    sudo kubeadm init --skip-preflight-checks

    sudo cp /etc/kubernetes/admin.conf $HOME/
    sudo chown $(id -u):$(id -g) $HOME/admin.conf
    export KUBECONFIG=$HOME/admin.conf

    kubectl taint nodes --all node-role.kubernetes.io/master-
    kubectl apply -f https://git.io/weave-kube-1.6

    echo "wait for K8s node to be Ready"
    kube_ready=$(kubectl get node -o jsonpath='{.items[0].status.conditions[3].status}')
    INC=0
    until [[ "${kube_ready}" == "True" || $INC -gt 10 ]]; do
        echo "."
        sleep 5
        ((++INC))
        kube_ready=$(kubectl get node -o jsonpath='{.items[0].status.conditions[3].status}')
    done

    if [ "${kube_ready}" == "False" ]; then
        echo "k8s node never went to Ready status"
        exit 1
    fi

    echo "k8s node in Ready status"

}

kubeadm_reset() {
    kubeadm reset
    sudo rm /usr/local/bin/kube*
    sudo rm kubectl
    rm $HOME/admin.conf
    rm -rf $HOME/.kube
    sudo apt-get -y remove kubelet
    sudo apt-get -y remove kubeadm
}


case "${1:-}" in
  up)
    sudo sh -c "${scriptdir}/kubeadm-install.sh ${KUBE_VERSION}" root
    install
    ${scriptdir}/makeTestImages.sh save amd64 || true
    sudo cp ${scriptdir}/kubeadm-rbd /bin/rbd
    sudo chmod +x /bin/rbd
    docker pull ceph/base
    ;;
  clean)
    kubeadm_reset
    ;;
  *)
    echo "usage:" >&2
    echo "  $0 up" >&2
    echo "  $0 clean" >&2
esac
