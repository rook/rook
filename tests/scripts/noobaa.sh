# dev commands

function cmd_dev {
    make -j4 codegen || exit 1
    make -j4 GO_STATIC_PACKAGES=github.com/rook/rook/cmd/rook go.build || exit 2
    ROOK=_output/bin/$(go env GOHOSTOS)_$(go env GOHOSTARCH)/rook
    DEV=true $ROOK noobaa operator
}

# run the operator as a job - not so useful but might be in some cases

function cmd_job {
    kubectl run rook-noobaa-operator \
        -it \
        --rm --restart=Never \
        --image=rook/noobaa:master \
        --serviceaccount=rook-noobaa-operator \
        --namespace=rook-noobaa \
        --env="POD_NAME=rook-noobaa-operator" \
        --env="POD_NAMESPACE=rook-noobaa" \
        --env="ROOK_LOG_LEVEL=DEBUG" \
        -- noobaa operator
}

# build commands

function cmd_build {
    if ! docker info | grep -q 'Name: minikube'
    then
        echo 'NOTE: To build directly on minikube use: eval $(minikube docker-env)'
    fi
    make IMAGES=noobaa || exit 1
    . build/common.sh
    docker tag ${BUILD_REGISTRY}/noobaa-amd64 rook/noobaa:master
}

function cmd_unbuild {
    make clean
    rm -rf vendor/ pkg/client/
    dep ensure
    make codegen || exit 1
}

function cmd_rebuild {
    cmd_unbuild
    cmd_build
}

# operator commands

function cmd_install {
    kubectl create -f cluster/examples/kubernetes/noobaa/noobaa-operator.yaml
}

function cmd_uninstall {
    kubectl delete -f cluster/examples/kubernetes/noobaa/noobaa-operator.yaml
    kubectl delete -f cluster/examples/kubernetes/noobaa/noobaa-system-crd.yaml
    kubectl delete -f cluster/examples/kubernetes/noobaa/noobaa-backing-store-crd.yaml
    kubectl delete -f cluster/examples/kubernetes/noobaa/noobaa-bucket-class-crd.yaml
}

function cmd_reinstall {
    cmd_uninstall
    cmd_install
}

function cmd_restart {
    kubectl delete pod -l app=rook-noobaa-operator -n rook-noobaa
}

function cmd_logs {
    kubectl logs --tail=1000 -l app=rook-noobaa-operator -n rook-noobaa
}

# noobaa commands

function cmd_create {
    kubectl create -f cluster/examples/kubernetes/noobaa/noobaa-system-example.yaml
}

function cmd_delete {
    kubectl delete -f cluster/examples/kubernetes/noobaa/noobaa-system-example.yaml
}

function cmd_recreate {
    cmd_delete
    cmd_create
}

# general commands

function cmd_cleanup {
    cmd_delete
    cmd_uninstall
    cmd_unbuild
}

function cmd_status {
    echo "> kubectl get noobaa,all,secret,cm,pvc,sa,role,rolebinding:"
    kubectl get noobaa -n rook-noobaa -L app 2>&1 | grep .
    kubectl get all,secret,cm,pvc,sa,role,rolebinding -n rook-noobaa -L app 2>&1 | grep .
}


if [ "$(type -t cmd_$1)" = "function" ]
then
    cmd=$1
    shift
    cmd_$cmd $*
else
    echo "Commands:"
    echo "========"
    set | egrep '^cmd_[a-z]+ ()' | cut -d'_' -f2- | cut -d' ' -f1 | sort
    exit 1
fi
