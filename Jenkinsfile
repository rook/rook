// Rook build for Jenkins Pipelines

try {
    node("ec2-stateful") {

        def DOWNLOADDIR='~/.download'

        stage('Checkout') {
            checkout scm
            sh "git submodule sync --recursive"
            sh "git submodule update --init --recursive"
        }

        stage('Validation') {
            sh "external/ceph-submodule-check"
        }

        stage('Build') {
            sh "mkdir -p ${DOWNLOADDIR}"
            sh "DOWNLOADDIR=${DOWNLOADDIR} build/run make -j\$(nproc) release"
        }

        stage('Tests') {
            sh "DOWNLOADDIR=${DOWNLOADDIR} build/run make -j\$(nproc) check"
        }

        stage('Cleanup') {
            deleteDir()
        }
    }
}
catch (Exception e) {
    echo 'Failure encountered'

    node("ec2-stateful") {
        echo 'Cleaning up workspace'
        deleteDir()
    }

    exit 1
}
