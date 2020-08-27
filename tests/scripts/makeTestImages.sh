#!/bin/bash
set -ex

# readable bash output
export PS4='+${BASH_SOURCE}:${LINENO}: ${FUNCNAME[0]:+${FUNCNAME[0]}(): }'

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
# shellcheck disable=SC1090
source "${scriptdir}/../../build/common.sh"

# list of images to build
imagesToBuild=(ceph cockroachdb cassandra nfs yugabytedb)

case "${1:-}" in
    save)
    # Storage backend to tag/save/build
    storageBackend="$3"
    if [ -z "$storageBackend" ]; then
        storageBackend="all"
    fi
        case "${2:-}" in
            arm|arm64|amd64)
                if [[ $storageBackend == "all" ]]; then
                    for image in "${imagesToBuild[@]}"; do
                        docker tag "${BUILD_REGISTRY}/$image-$2:latest" rook/"$image":master
                    done
                else
                    docker tag "${BUILD_REGISTRY}/$storageBackend-$2:latest" rook/"$storageBackend":master
                fi
                if [ -n "$4" ]; then
                    if [[ $storageBackend == "all" ]]; then
                        for image in "${imagesToBuild[@]}"; do
                            docker tag "${BUILD_REGISTRY}/$image-$2:latest" "rook/$image:$3"
                            docker save -o "$image-$2.tar" rook/"$image":master "rook/$image:$3"
                        done
                    else
                        docker tag "${BUILD_REGISTRY}/$storageBackend-$2:latest" "rook/ceph:$3"
                        docker save -o "$storageBackend-$2.tar" rook/"$storageBackend":master "rook/$storageBackend:$3"
                    fi
                else
                    if [[ $storageBackend == "all" ]]; then
                        for image in "${imagesToBuild[@]}"; do
                            docker save -o "$image-$2.tar" rook/$image:master
                        done
                    else
                        docker save -o "$storageBackend-$2.tar" rook/$storageBackend:master
                    fi
                fi

                 echo "Saved docker images in archives: $(ls | grep tar)"
                ;;
            *)
                echo "usage :" >&2
                echo "$0 $1 [arm|arm64|amd64] [new_tag]" >&2
        esac
        ;;
    load)
        case "${2:-}" in
            arm|arm64|amd64)
                # Storage backend to tag/save/build
                storageBackend="$3"
                if [ -z "$storageBackend" ]; then
                    storageBackend="all"
                fi
                echo "Loading archived images to docker: $(ls | grep tar)"

                if [[ $storageBackend == "all" ]]; then
                    for image in "${imagesToBuild[@]}"; do
                        docker load -i "$image-$2.tar"
                    done
                else
                    docker load -i "$storageBackend-$2.tar"
                fi
                ;;
            *)
                echo "usage :" >&2
                echo "$0 $1 [arm|arm64|amd64]" >&2

        esac
        ;;
    tag)
        case "${2:- }" in
            arm|arm64|amd64)
                tag_version="${3:-"master"}"
                for image in "${imagesToBuild[@]}"; do
                    docker tag "${BUILD_REGISTRY}/$image-$2:latest" "rook/$image:${tag_version}"
                done
                ;;
            *)
                echo "usage :" >&2
                echo "$0 $1 [arm|arm64|amd64] [new_tag]" >&2
        esac
        ;;
    *)
        echo "usage:" >&2
        echo "  $0 save [arm|arm64|amd64] [new_tag]" >&2
        echo "  $0 load [arm|arm64|amd64]" >&2
        echo "  $0 tag [arm|arm64|amd64] [new_tag]" >&2
esac
