#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
# source=build/common.sh
source "${scriptdir}/../../build/common.sh"


case "${1:-}" in
    save)
        case "${2:-}" in
            arm|arm64|amd64)
                docker tag "${BUILD_REGISTRY}/ceph-$2:latest" rook/ceph:master
                docker tag "${BUILD_REGISTRY}/cassandra-$2:latest" rook/cassandra:master
                docker tag "${BUILD_REGISTRY}/nfs-$2:latest" rook/nfs:master
                if [ -n "$3" ]
                then
                    docker tag "${BUILD_REGISTRY}/ceph-$2:latest" "rook/ceph:$3"
                    docker save -o "ceph-$2.tar" rook/ceph:master "rook/ceph:$3"
                    docker tag "${BUILD_REGISTRY}/cassandra-$2:latest" "rook/cassandra:$3"
                    docker save -o "cassandra-$2.tar" rook/cassandra:master "rook/cassandra:$3"
                    docker tag "${BUILD_REGISTRY}/nfs-$2:latest" "rook/nfs:$3"
                    docker save -o "nfs-$2.tar" rook/nfs:master "rook/nfs:$3"
                else
                    docker save -o "ceph-$2.tar" rook/ceph:master
                    docker save -o "cassandra-$2.tar" rook/cassandra:master
                    docker save -o "nfs-$2.tar" rook/nfs:master
                fi

                 echo "Saved docker images in archives: $(ls ./*tar*)"
                ;;
            *)
                echo "usage :" >&2
                echo "$0 $1 [arm|arm64|amd64] [new_tag]" >&2
        esac
        ;;
    load)
        case "${2:-}" in
            arm|arm64|amd64)
                echo "Loading archived images to docker: $(ls ./*tar*)"

                docker load -i "ceph-$2.tar"
                docker load -i "cassandra-$2.tar"
                docker load -i "nfs-$2.tar"
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
                docker tag "${BUILD_REGISTRY}/ceph-$2:latest" "rook/ceph:${tag_version}"
                docker tag "${BUILD_REGISTRY}/cassandra-$2:latest" "rook/cassandra:${tag_version}"
                docker tag "${BUILD_REGISTRY}/nfs-$2:latest" "rook/nfs:${tag_version}"
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
