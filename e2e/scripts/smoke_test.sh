#!/bin/bash -e

test_source_repo=$(pwd)/e2e
docker_test_repo=/workspace/go/src/github.com/dangula/rook/e2e
git_smoke_test_directory=github.com/dangula/rook/e2e/tests/integration/smokeTest
container_image=quay.io/quantum/rook-test
tmp_docker_sock_path=/tmp/docker.sock
test_name_setup_environment=TestSetupPlatform*
results_dir=results
set_go_env_var="export GOPATH=/workspace/go && export GOROOT=/usr/lib/go"

#create the rook infrastructure container
rook_infra::create() {
    export id=$(docker run \
        -d \
        --network=host \
        -e container=docker \
        --privileged \
        --security-opt=seccomp:unconfined \
        --cap-add=SYS_ADMIN \
        -v /dev:/dev \
        -v /sys:/sys \
        -v /sys/fs/cgroup:/sys/fs/cgroup \
        -v /sbin/modprobe:/sbin/modprobe \
        -v /lib/modules:/lib/modules:rw \
        -v /var/run/docker.sock:${tmp_docker_sock_path} \
        -v ${test_source_repo}:/${docker_test_repo} \
        -w ${docker_test_repo} \
        ${container_image} \
        /sbin/init)

        rc=$?; if [[ $rc != 0 ]]; then exit $rc; fi
}

#prepare the rook_infra container to run tests
rook_infra::init() {
    echo Mounting host docker.sock...

    docker exec -i ${id} rm -rfv /var/run/docker.sock
    rc=$?; if [[ $rc != 0 ]]; then exit $rc; fi

    docker exec -i ${id} ln -s ${tmp_docker_sock_path} /var/run/docker.sock
    rc=$?; if [[ $rc != 0 ]]; then exit $rc; fi
    echo Success...

    echo Installing rook-test-framework dependencies...
    docker exec -i ${id} /bin/bash -c \
    "${set_go_env_var} && glide install"
    rc=$?; if [[ $rc != 0 ]]; then exit $rc; fi
    echo Success...
}

rook_infra::run_test() {
    test_name_regex=$1

    #TODO:: pipe it to junit parser
    #TODO:: get exit code and fail if not 0
    docker exec ${id} /bin/bash -c \
    "${set_go_env_var} && go test -run ${test_name_regex} ${git_smoke_test_directory} -v --rook_platform=Kubernetes --k8s_version=v1.5"
}

rook_infra::gather_results() {
    echo Gathering results...

}

rook_infra::cleanup() {
    echo Removing rook-test-framework container...
    docker kill ${id}
    docker rm ${id}
}

{ #try
    rook_infra::create
    sleep 5
    rook_infra::init

    rook_infra::run_test TestFileStorage_SmokeTest

    read -n1 -r -p "Press space to continue..."
} || { #catch
    rook_infra::gather_results
}

#Delete the rook infrastructure container
rook_infra::cleanup

