#!/bin/bash +e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
temp="/tmp/rook-tests-scripts-helm"
arch="${ARCH:-}"

detectArch() {
    case "$(uname -m)" in
        "x86_64" | "amd64")
            arch="amd64"
            ;;
        "aarch64")
            arch="arm64"
            ;;
        "i386")
            arch="i386"
            ;;
        *)
            echo "Couldn't translate 'uname -m' output to an available arch."
            echo "Try setting ARCH environment variable to your system arch:"
            echo "amd64, x86_64. aarch64, i386"
            exit 1
            ;;
    esac
}

# Echo the name or path of the helm command. Install helm to a temp location
# if it's not present.
install_helm() {
    local helm="helm"
    local dist="$(uname -s)"
    dist="$(echo "${dist}" | tr '[A-Z]' '[a-z]')"
    local tmp_helm="${temp}/${dist}-${arch}/helm"

    if [[ -f "${tmp_helm}" ]]; then # helm installed by this script
        helm="${tmp_helm}"
    elif ! which ${helm} &>/dev/null; then
        echo "Installing helm..." >&2
        mkdir -p "${temp}"
        local helm_url="https://storage.googleapis.com/kubernetes-helm/helm-v2.13.1-${dist}-${arch}.tar.gz"
        local helm_gz="${temp}/helm.tar.gz"
        wget "${helm_url}" -O ${helm_gz} >&2
        tar -C "${temp}" -zxvf ${helm_gz} >&2
        helm="${tmp_helm}"
    fi

    echo "${helm}"
}

install() {
    # Init helm
    "${HELM}" init

    sleep 5

    helm_ready=$(kubectl get pods -l app=helm -n kube-system -o jsonpath='{.items[0].status.phase}')
    INC=0
    until [[ "${helm_ready}" == "Running" || $INC -gt 20 ]]; do
        sleep 10
        (( ++INC ))
        helm_ready=$(kubectl get pods -l app=helm -n kube-system -o jsonpath='{.items[0].status.phase}')
        echo "helm pod status: $(helm_ready)"
    done

    if [ "${helm_ready}" != "Running" ]; then
        echo "Helm init not successful"
        exit 1
    fi

    echo "Helm init successful"


    # set up RBAC for helm
    kubectl -n kube-system create sa tiller
    kubectl create clusterrolebinding tiller --clusterrole cluster-admin --serviceaccount=kube-system:tiller
    kubectl -n kube-system patch deploy/tiller-deploy -p '{"spec": {"template": {"spec": {"serviceAccountName": "tiller"}}}}'

    # set up local repo for helm and add local/rook-ceph
    "${HELM}" repo remove local
    "${HELM}" repo remove stable

    "${HELM}" repo index _output/charts/ --url http://127.0.0.1:8879
    nohup "${HELM}" serve --repo-path _output/charts/ > /dev/null 2>&1 &
    sleep 10 # wait for helm serve to start

    "${HELM}" repo add local http://127.0.0.1:8879
    "${HELM}" search rook-ceph
}

# Reset helm or reset & rm temp helm
helm_reset() {
    local do_rm="$1"

    ${HELM} reset

    kubectl -n kube-system delete sa tiller
    kubectl delete clusterrolebinding tiller

    local pid
    pid="$(pgrep ${HELM})"
    (( $? == 0 )) && kill -9 ${pid}

    [[ -n "${do_rm}" ]] && rm -rf "${temp}"
}


if [ -z "${arch}" ]; then
    detectArch
fi

HELM="$(install_helm)"
echo "debug: HELM=$HELM"

case "${1:-}" in
    up)
        install
        cat _output/version | xargs "${scriptdir}/makeTestImages.sh" tag "${arch}" || true
        "${HELM}" repo add stable https://kubernetes-charts.storage.googleapis.com/
        ;;
    reset)
        helm_reset
        ;;
    clean)
        helm_reset "rm"
        ;;
    *)
        echo "usage:" >&2
        echo "  $0 up # bring up helm" >&2
        echo "  $0 reset # reset helm" >&2
        echo "  $0 clean # reset helm and rm tmp helm if present" >&2
esac
