#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source "${scriptdir}/../../build/common.sh"


case "${1:-}" in
  save)
    case "${2:-}" in
        arm|arm64|amd64)
            docker tag ${BUILD_REGISTRY}/ceph-$2:latest rook/ceph:master
            docker tag ${BUILD_REGISTRY}/ceph-toolbox-$2:latest rook/ceph-toolbox:master
            if [ ! -z $3 ]
                then
                    docker tag ${BUILD_REGISTRY}/ceph-$2:latest rook/ceph:$3
                    docker save -o ceph-$2.tar rook/ceph:master rook/ceph:$3 rook/ceph-toolbox:master
                else
                   docker save -o ceph-$2.tar rook/ceph:master rook/ceph-toolbox:master
            fi
            ;;
        *)
            echo "usage :" >&2
            echo "$0 $1 [arm|arm64|amd64] [new_tag]" >&2
    esac
    ;;
  load)
    case "${2:-}" in
         arm|arm64|amd64)
            docker load -i ceph-$2.tar
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
            docker tag ${BUILD_REGISTRY}/ceph-$2:latest rook/ceph:${tag_version}
            docker tag ${BUILD_REGISTRY}/ceph-toolbox-$2:latest rook/ceph-toolbox:${tag_version}
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
