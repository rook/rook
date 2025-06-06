#!/usr/bin/env -S bash +e

temp="/tmp/rook-tests-scripts-helm"

helm_version="${HELM_VERSION:-"v3.18.2"}"
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
    # Download and unpack helm
    local dist
    dist="$(uname -s)"
    dist=$(echo "${dist}" | tr "[:upper:]" "[:lower:]")
    mkdir -p "${temp}"
    wget "https://get.helm.sh/helm-${helm_version}-${dist}-${arch}.tar.gz" -O "${temp}/helm.tar.gz"
    tar -C "${temp}" -xvf "${temp}/helm.tar.gz" --strip-components 1
}

if [ -z "${arch}" ]; then
    detectArch
fi

case "${1:-}" in
    up)
        install
        ;;
    *)
        echo "usage:" >&2
        echo "  $0 up" >&2
esac
