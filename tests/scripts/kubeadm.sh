#!/bin/bash +e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

tarname=image.tar
tarfile="${WORK_DIR}/tests/${tarname}"

export KUBE_VERSION=${KUBE_VERSION:-"v1.8.5"}

usage(){
    echo "usage:" >&2
    echo "  $0 up " >&2
    echo "  $0 install master" >&2
    echo "  $0 install node --token <token> <master-ip>:<master-port> (for k8s 1.7 and older)" >&2
    echo "  $0 install node --token <token> <master-ip>:<master-port> --discovery-token-ca-cert-hash sha256:<hash>" >&2
    echo "  $0 wait <number of nodes>" >&2
    echo "  $0 clean" >&2
}

#install k8s master node
install_master(){

    if [[ $KUBE_VERSION == v1.8* ]] ; then
        # for k8s 1.8, use a non default value for volume plugins
        cat << EOF | sudo tee -a /etc/systemd/system/kubelet.service.d/11-volume_plugin_dir.conf
[Service]
Environment="KUBELET_SYSTEM_PODS_ARGS=--pod-manifest-path=/etc/kubernetes/manifests --allow-privileged=true --volume-plugin-dir=/var/lib/kubelet/volumeplugins"
EOF
        sudo systemctl daemon-reload
    fi

    sudo kubeadm init --skip-preflight-checks --kubernetes-version ${KUBE_VERSION}

    sudo cp /etc/kubernetes/admin.conf $HOME/
    sudo chown $(id -u):$(id -g) $HOME/admin.conf
    export KUBECONFIG=$HOME/admin.conf

    kubectl taint nodes --all node-role.kubernetes.io/master-
    kubectl apply -f "https://cloud.weave.works/k8s/net?k8s-version=$(kubectl version | base64 | tr -d '\n')"

    echo "wait for K8s master node to be Ready"
    INC=0
    while [[ $INC -lt 20 ]]; do
        kube_ready=$(kubectl get node -o jsonpath='{.items['$count'].status.conditions[?(@.reason == "KubeletReady")].status}')
        if [ "${kube_ready}" == "True" ]; then
            break
        fi
        echo "."
        sleep 10
        ((++INC))
    done

    if [ "${kube_ready}" != "True" ]; then
        echo "k8s master node never went to Ready status"
        exit 1
    fi

    echo "k8s master node in Ready status"
}

#install k8s node
install_node(){
    echo "inside install node function"

    # for k8s 1.8, use a non default value for volume plugins
    if [[ $KUBE_VERSION == v1.8* ]] ; then
        cat << EOF | sudo tee -a /etc/systemd/system/kubelet.service.d/11-volume_plugin_dir.conf
[Service]
Environment="KUBELET_SYSTEM_PODS_ARGS=--pod-manifest-path=/etc/kubernetes/manifests --allow-privileged=true --volume-plugin-dir=/var/lib/kubelet/volumeplugins"
EOF
        sudo systemctl daemon-reload
    fi

    echo "kubeadm join ${1} ${2} ${3} ${4} ${5} --skip-preflight-checks"
    sudo kubeadm join ${1} ${2} ${3} ${4} ${5} --skip-preflight-checks || true
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
        echo "wait for K8s node $count to be Ready."
        INC=0
        while [[ $INC -lt 90 ]]; do
            kube_ready=$(kubectl get node -o jsonpath='{.items['$count'].status.conditions[?(@.reason == "KubeletReady")].status}')
            if [ "${kube_ready}" != "True" ]; then
                break
            fi
            echo  -n "."
            sleep 10
            ((++INC))
        done
        echo
        if [ "${kube_ready}" != "True" ]; then
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
                if [ "$#" -eq 5 ] || [ "$#" -eq 7 ]; then
                    install_node $3 $4 $5 $6 $7
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
