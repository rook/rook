#!/bin/bash +e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
temp="/tmp/rook-tests-scripts-helm"

HELM="helm"
helm_version="${HELM_VERSION:-"v3.2.1"}"
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

install() {
    if ! helm_loc="$(type -p "helm")" || [[ -z ${helm_loc} ]]; then
        # Download and unpack helm
        local dist
        dist="$(uname -s)"
        dist=$(echo "${dist}" | tr "[:upper:]" "[:lower:]")
        mkdir -p "${temp}"
        wget "https://get.helm.sh/helm-${helm_version}-${dist}-${arch}.tar.gz" -O "${temp}/helm.tar.gz"
        tar -C "${temp}" -zxvf "${temp}/helm.tar.gz"
        HELM="${temp}/${dist}-${arch}/helm"
    fi

    # set up local repo for helm and add local/rook-ceph
    "${HELM}" repo remove local || true
    "${HELM}" repo remove stable

    "${HELM}" repo index _output/charts/ --url http://127.0.0.1:8879
    nohup "${HELM}" serve --repo-path _output/charts/ > /dev/null 2>&1 &
    sleep 10 # wait for helm serve to start

    "${HELM}" repo add local http://127.0.0.1:8879
    "${HELM}" search rook-ceph

}

helm_reset() {
    if ! helm_loc="$(type -p "helm")" || [[ -z ${helm_loc} ]]; then
        local dist
        dist="$(uname -s)"
        dist=$(echo "${dist}" | tr "[:upper:]" "[:lower:]")
        HELM="${temp}/${dist}-${arch}/helm"
    fi
    "${HELM}" reset
    # shellcheck disable=SC2021
    pgrep "${HELM}" | grep -v grep | awk '{print $2}'| xargs kill -9
    rm -rf "${temp}"
}


if [ -z "${arch}" ]; then
    detectArch
fi

case "${1:-}" in
    up)
        install
        # shellcheck disable=2002
        cat _output/version | xargs "${scriptdir}/makeTestImages.sh" tag "${arch}" || true
        "${HELM}" repo add stable https://kubernetes-charts.storage.googleapis.com/
        ;;
    clean)
        helm_reset
        ;;
    *)
        echo "usage:" >&2
        echo "  $0 up" >&2
        echo "  $0 clean" >&2
esac
