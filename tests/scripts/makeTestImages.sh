#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source "${scriptdir}/../../build/common.sh"


case "${1:-}" in
  save)
    case "${2:-}" in
        arm|arm64|amd64)
            docker tag ${BUILD_REGISTRY}/rook-$2:latest rook/rook:master
            docker tag ${BUILD_REGISTRY}/toolbox-$2:latest rook/toolbox:master
            if [ ! -z $3 ]
                then
                    docker tag ${BUILD_REGISTRY}/rook-$2:latest rook/rook:$3
                    docker save -o rook-$2.tar rook/rook:master rook/rook:$3 rook/toolbox:master
                else
                   docker save -o rook-$2.tar rook/rook:master rook/toolbox:master
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
            docker load -i rook-$2.tar
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
            docker tag ${BUILD_REGISTRY}/rook-$2:latest rook/rook:${tag_version}
            docker tag ${BUILD_REGISTRY}/toolbox-$2:latest rook/toolbox:${tag_version}
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
