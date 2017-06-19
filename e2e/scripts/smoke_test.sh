#!/bin/bash -e

test_source_repo=$(pwd)
docker_test_repo=/workspace/go/src/github.com/rook/rook
git_smoke_test_directory=github.com/rook/rook/e2e/tests/smoke
git_test_directory=github.com/rook/rook/e2e/tests
container_image=quay.io/rook/rook-test
tmp_docker_sock_path=/var/run/docker.sock
results_dir=results
results_filename=test.log
temp_archive=forRookImage.tar

#create the rook infrastructure container
rook_infra::create() {
    export id=$(docker run \
        -d \
        -e GOPATH=/workspace/go \
        -e GOROOT=/usr/lib/go \
        --privileged \
        --security-opt=seccomp:unconfined \
        -v /lib/modules:/lib/modules \
        -v /sbin/modprobe:/sbin/modprobe \
        -v /sys/bus:/sys/bus \
        -v /dev:/dev \
        -v /var/run/docker.sock:${tmp_docker_sock_path} \
        -v ${test_source_repo}:${docker_test_repo} \
        -w ${docker_test_repo}/e2e \
        ${container_image} \
        /sbin/init)

    rc=$?; if [[ $rc != 0 ]]; then set -e; fi

    local attempt_num=1
    local max_attempts=10

    until [[ $(docker inspect -f {{.State.Running}} $id) == "true" ]]
    do
        if (( attempt_num == max_attempts ))
        then
            echo "rook-test infra container failed to start"
            return 1
        else
            sleep 1
            ((++attempt_num))

        fi
    done

    echo "rook-test container is running..."
}

#prepare the rook_infra container to run tests
rook_infra::init() {
    echo Creating results directory
    docker exec ${id} mkdir -p ${results_dir}
    rc=$?; if [[ $rc != 0 ]]; then set -e; fi

    echo Installing rook-test-framework dependencies...
    docker exec ${id} /bin/bash -c \
    "glide install"
    rc=$?; if [[ $rc != 0 ]]; then set -e; fi
    echo Success...
}

rook_infra::run_test() {
    local test_name_regex=$1
    local tag_name=$2
    local rook_platform=$3
    local k8s_version=$4

    docker exec -t ${id} /bin/bash -c \
        "go test -timeout 1200s -run ${test_name_regex} ${git_smoke_test_directory} --rook_platform=${rook_platform} --k8s_version=${k8s_version} --rook_version=${tag_name} -v 2>&1 | tee ${results_dir}/${results_filename}"

    rc=$?; if [[ $rc != 0 ]]; then set -e; fi
}

rook_infra::try_copy_docker_image_to_rook_infra() {
    local tag_name=$1

    echo Searching local docker registry for ${tag_name}
    export imageId=$(docker images -q ${tag_name})

    if [ -z "$imageId" ]; then
        echo Image not found
    else
        echo Image found...

        echo Archiving the docker image
        docker save -o ${temp_archive} ${tag_name}
        rc=$?; if [[ $rc != 0 ]]; then set -e; fi
        echo success...

        echo Copying archived image to rook-infra
        docker cp forRookImage.tar ${id}:/${temp_archive}
        rc=$?; if [[ $rc != 0 ]]; then set -e; fi
        echo success...

        echo Importing image into rook-infra image registry
        docker exec ${id} /bin/bash -c "docker load -i /${temp_archive}"
        rc=$?; if [[ $rc != 0 ]]; then set -e; fi
        echo success...
    fi
}

rook_infra::gather_results() {
    echo Gathering results...

    #install junit parser && create junit results
    docker exec ${id} /bin/bash -c \
        "go get -u -f github.com/jstemmer/go-junit-report && cat ${results_dir}/test.log | go-junit-report > ${results_dir}/junit.xml"

    rc=$?; if [[ $rc != 0 ]]; then set -e; fi
}

rook_infra::cleanup() {
    local tag_name=$1
    local rook_platform=$2
    local k8s_version=$3

    echo Removing rook-test-framework container and images...
    docker kill ${id} || true
    docker rm ${id} || true
    docker images|grep 'rook-test\|kubeadm-dind-cluster\|ceph/base'|xargs docker rmi > /dev/null 2>&1 || true
    docker volume ls -qf dangling=true | xargs -r docker volume rm > /dev/null 2>&1|| true

}

 tag_name=${1:-"master-latest"}
 rook_platform=${2:-"Kubernetes"}
 k8s_version=${3:-"v1.6"}

{ #try

    rook_infra::create
    rook_infra::try_copy_docker_image_to_rook_infra quay.io/rook/rookd:${tag_name}
    rook_infra::try_copy_docker_image_to_rook_infra quay.io/rook/toolbox:${tag_name}
    rook_infra::init

    rook_infra::run_test SmokeSuite ${tag_name} ${rook_platform} ${k8s_version}

    rook_infra::gather_results

} || { #catch
    rook_infra::gather_results
}

rook_infra::cleanup ${tag_name} ${rook_platform} ${k8s_version}