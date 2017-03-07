// Rook build for Jenkins Pipelines

try {
    // if you are building this in a forked repo please change/remove the node specification to suit your Jenkins setup
    node("ec2-stateful") {
        // this count is meant to be optimal for the AWS EC2 c4.4xlarge instance type, adjust accordingly for the build host
        def parallel = '12'
        // a ccache location is not required, but if is used it will accelerate successive builds
        def CCACHE_DIR = '~/.ccache'
        // this parameter is used only as an override to build specific commits when debugging
        def sha1

        stage('Preparation') {
            sh "mkdir -p ${CCACHE_DIR}"
            writeFile file: "${CCACHE_DIR}/ccache.conf", text: "max_size = 20G"
            checkout scm
            sh "git submodule sync --recursive"
            sh "git submodule update --init --recursive"
            sh "external/ceph-submodule-check"
        }

        stage('Build') {
            sh "CCACHE_DIR=${CCACHE_DIR} build/run ccache -s"
            sh "CCACHE_DIR=${CCACHE_DIR} build/run make -j${parallel} release"
            sh "CCACHE_DIR=${CCACHE_DIR} build/run ccache -s"
        }

        stage('Unit Tests') {
            sh "CCACHE_DIR=${CCACHE_DIR} build/run make -j${parallel} check"
        }

        stage('Results') {
            // not yet handling artifacts
            //junit '**/target/reports/TEST-*.xml'
            //archive 'target/*.tgz'
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