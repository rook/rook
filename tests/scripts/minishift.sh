#!/bin/bash -e

scriptdir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source "${scriptdir}/../../build/common.sh"

function wait_for_ssh() {
    local tries=100
    while (( ${tries} > 0 )) ; do
        if `minishift ssh echo connected &> /dev/null` ; then
            return 0
        fi
        tries=$(( ${tries} - 1 ))
        sleep 0.1
    done
    echo ERROR: ssh did not come up >&2
    exit 1
}

function copy_image_to_cluster() {
    local build_image=$1
    local final_image=$2
    docker save ${build_image} | (eval $(minishift docker-env --shell bash) && docker load && docker tag ${build_image} ${final_image})
}

# current kubectl context == minishift, returns boolean
function check_context() {
  if [ "$(kubectl config view 2>/dev/null | awk '/current-context/ {print $NF}')" = "minishift" ]; then
    return 0
  fi

  return 1
}

# configure minishift
MEMORY=${MEMORY:-"3000"}

case "${1:-}" in
  up)
    echo "starting minishift"
    minishift start --memory=${MEMORY} --vm-driver=virtualbox --iso-url centos
    wait_for_ssh

    # create a link so the default dataDirHostPath will work for this environment
    #minishift ssh "sudo mkdir /mnt/sda1/var/lib/rook;sudo ln -s /mnt/sda1/var/lib/rook /var/lib/rook"
    copy_image_to_cluster ${BUILD_REGISTRY}/ceph-amd64 rook/ceph:master
    ;;
  down)
    minishift delete -f
    ;;
  ssh)
    echo "connecting to minishift"
    minishift ssh
    ;;
  update)
    echo "updating the rook images"
    copy_image_to_cluster ${BUILD_REGISTRY}/ceph-amd64 rook/ceph:master
    ;;
  restart)
    if check_context; then
        [ "$2" ] && regex=$2 || regex="^rook-"
        echo "Restarting Rook pods matching the regex \"$regex\" in \"rook\" namespace."
        delete_rook_pods "rook" $regex
        echo "Restarting Rook pods matching the regex \"$regex\" in \"rook-system\" namespace.."
        delete_rook_pods "rook-system" $regex
    else
      echo "To prevent accidental data loss acting only on 'minishift' context. No action is taken."
    fi
    ;;
  wordpress)
    echo "copying the wordpress images"
    copy_image_to_cluster mysql:5.6 mysql:5.6
    copy_image_to_cluster wordpress:4.6.1-apache wordpress:4.6.1-apache
    ;;
  helm)
    echo " copying rook image for helm"
    helm_tag="`cat _output/version`"
    copy_image_to_cluster ${BUILD_REGISTRY}/rook-amd64 rook/rook:${helm_tag}
    ;;
  clean)
    minishift delete -f
    ;;
  *)
    echo "usage:" >&2
    echo "  $0 up" >&2
    echo "  $0 down" >&2
    echo "  $0 clean" >&2
    echo "  $0 ssh" >&2
    echo "  $0 update" >&2
    echo "  $0 restart <pod-name-regex> (the pod name is a regex to match e.g. restart ^rook-ceph-osd)" >&2
    echo "  $0 wordpress" >&2
    echo "  $0 helm" >&2
esac
