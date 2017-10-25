#!/bin/bash +e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

tarname=image.tar
tarfile=${WORK_DIR}/tests/${tarname}

export KUBE_VERSION=${KUBE_VERSION:-"v1.8.2"}

usage(){
    echo "usage:" >&2
    echo "  $0 up " >&2
    echo "  $0 install master" >&2
    echo "  $0 install node --token <token> <master-ip>:<master-port>" >&2
    echo "  $0 wait <number of nodes>" >&2
    echo "  $0 clean" >&2
}

#install k8s master node
install_master(){

    # This is needed on K8S 1.6 to fix a regression. https://github.com/kubernetes/kubernetes/issues/47109
    if [[ $KUBE_VERSION == v1.6* ]] ;
    then
        cat << EOF | sudo tee -a /etc/systemd/system/kubelet.service.d/11-disable_attachdetach_controller.conf
[Service]
Environment="KUBELET_SYSTEM_PODS_ARGS=--pod-manifest-path=/etc/kubernetes/manifests --allow-privileged=true --enable-controller-attach-detach=false"
EOF
        sudo systemctl daemon-reload
    fi

    sudo kubeadm init --skip-preflight-checks

    sudo cp /etc/kubernetes/admin.conf $HOME/
    sudo chown $(id -u):$(id -g) $HOME/admin.conf
    export KUBECONFIG=$HOME/admin.conf

    kubectl taint nodes --all node-role.kubernetes.io/master-
    kubectl apply -f "https://cloud.weave.works/k8s/net?k8s-version=$(kubectl version | base64 | tr -d '\n')"

     echo "wait for K8s master node to be Ready"
    kube_ready=$(kubectl get node -o jsonpath='{.items[0].status.conditions[3].status}')
    INC=0
    until [[ "${kube_ready}" == "True" || $INC -gt 20 ]]; do
        echo "."
        sleep 10
        ((++INC))
        kube_ready=$(kubectl get node -o jsonpath='{.items[0].status.conditions[3].status}')
    done

    if [ "${kube_ready}" == "False" ]; then
        echo "k8s master node never went to Ready status"
        exit 1
    fi

    echo "k8s master node in Ready status"
}

#install k8s node
install_node(){
    echo "inside install node function"
    echo "kubeadm join ${1} ${2} ${3} --skip-preflight-checks"
    sudo kubeadm join ${1} ${2} ${3} --skip-preflight-checks || true
}

#wait for all nodes in the cluster to be ready status
wait_for_ready(){
    #expect 3 node cluster by default
    local numberOfNode=${1:-3}
    local count=0
    sudo cp /etc/kubernetes/admin.conf $HOME/
    sudo chown $(id -u):$(id -g) $HOME/admin.conf
    export KUBECONFIG=$HOME/admin.conf

    until [[ $count -eq $numberOfNode ]]; do
        echo "wait for K8s node $count to be Ready"
        kube_ready=$(kubectl get node -o jsonpath='{.items['$count'].status.conditions[3].status}')
        INC=0
        until [[ "${kube_ready}" == "True" || $INC -gt 90 ]]; do
            echo  -n "."
            sleep 10
            ((++INC))
            kube_ready=$(kubectl get node -o jsonpath='{.items['$count'].status.conditions[3].status}')
        done
        echo
        if [ "${kube_ready}" == "False" ]; then
            echo "k8s node ${count} never went to Ready status"
            exit 1
        fi

        echo "k8s node ${count} in Ready status"
        ((++count))

    done

    echo "All k8s node in Ready status"

}

kubeadm_reset() {
    kubectl delete -f "https://cloud.weave.works/k8s/net?k8s-version=$(kubectl version | base64 | tr -d '\n')"
    sudo kubeadm reset --skip-preflight-checks
    sudo rm /usr/local/bin/kube*
    sudo rm kubectl
    rm $HOME/admin.conf
    rm -rf $HOME/.kube
    sudo apt-get -y remove kubelet
    sudo apt-get -y remove kubeadm
    sudo swapon -a
}

case "${1:-}" in
  up)
    sudo sh -c "${scriptdir}/kubeadm-install.sh ${KUBE_VERSION}" root
    install_master
    ${scriptdir}/makeTestImages.sh tag amd64 || true
    ;;
  clean)
    kubeadm_reset
    ;;
  install)
    if [ "$#" -lt 2 ]; then
        echo "invalid arguments for install"
        usage
        exit 1
    fi
    sudo sh -c "${scriptdir}/kubeadm-install.sh ${KUBE_VERSION}" root
    case "${2:-}" in
        master)
            install_master
        ;;
        node)
            if [ "$#" -eq 5 ]; then
                install_node $3 $4 $5
            else
                echo "invalid arguments for install node"
                usage
                exit 1
            fi
        ;;
        *)
            echo "invalid arguments for install" >&2
            usage
            exit 1
        ;;
    esac
    ;;
  wait)
    if [ "$#" -eq 2 ]; then
        wait_for_ready $2
    else
        echo "invalid number of arguments for wait"
        usage
        exit 1
    fi
    ;;
  *)
    usage
    exit 1
esac
