#!/bin/bash +e
scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

install() {
    #Download and unpack helm
    local dist
    dist="$(uname -s)"
    # shellcheck disable=SC2021
    dist=$(echo "${dist}" | tr "[A-Z]" "[a-z]")
    wget "https://storage.googleapis.com/kubernetes-helm/helm-v2.6.0-${dist}-amd64.tar.gz"
    tar -zxvf "helm-v2.6.0-${dist}-amd64.tar.gz"
    sudo mv "${dist}-amd64/helm" /usr/local/bin/helm

    #Init helm
    helm init

    sleep 5

    helm_ready=$(kubectl get pods -l app=helm -n kube-system -o jsonpath='{.items[0].status.phase}')
    INC=0
    until [[ "${helm_ready}" == "Running" || $INC -gt 20 ]]; do
        echo "."
        sleep 10
        ((++INC))
        helm_ready=$(kubectl get pods -l app=helm -n kube-system -o jsonpath='{.items[0].status.phase}')
    done

    if [ "${helm_ready}" != "Running" ]; then
        echo "Helm init not successfully"
        exit 1
    fi

    echo "Helm init successful"


    # set up RBAC for helm
    kubectl -n kube-system create sa tiller
    kubectl create clusterrolebinding tiller --clusterrole cluster-admin --serviceaccount=kube-system:tiller
    kubectl -n kube-system patch deploy/tiller-deploy -p '{"spec": {"template": {"spec": {"serviceAccountName": "tiller"}}}}'

    #set up local repo for helm and add local/rook-ceph
    helm repo remove local
    helm repo remove stable

    helm repo index _output/charts/ --url http://127.0.0.1:8879
    nohup helm serve --repo-path _output/charts/ > /dev/null 2>&1 &
    sleep 10 # wait for helm serve to start

    helm repo add local http://127.0.0.1:8879
    helm search rook-ceph

}

helm_reset() {
    helm reset
    local dist
    dist="$(uname -s)"
    # shellcheck disable=SC2021
    dist=$(echo "${dist}" | tr "[A-Z]" "[a-z]")
    sudo rm /usr/local/bin/helm
    rm -rf "${dist}-amd64/"
    rm "helm-v2.6.0-${dist}-amd64.tar.gz"*
    pgrep helm | grep -v grep | awk '{print $2}'| xargs kill -9

}


case "${1:-}" in
    up)
        install
        # shellcheck disable=2002
        cat _output/version | xargs "${scriptdir}/makeTestImages.sh" tag amd64 || true
        helm repo add stable https://kubernetes-charts.storage.googleapis.com/
        ;;
    clean)
        helm_reset
        ;;
    *)
        echo "usage:" >&2
        echo "  $0 up" >&2
        echo "  $0 clean" >&2
esac
