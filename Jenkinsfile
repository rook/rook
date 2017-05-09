// Rook build for Jenkins Pipelines

try {
    node("ec2-stateful") {

        stage('Checkout') {
            checkout scm
            sh 'git submodule sync --recursive'
            sh 'git submodule update --init --recursive'
        }

        stage('Validation') {
            sh 'external/ceph-submodule-check'
        }

        withEnv(["DOWNLOADDIR=${env.HOME}/.download", "ALWAYS_BUILD=0", "CHANNEL=${env.BRANCH_NAME}"]) {

            stage('Build') {
                sh 'build/run make -j\$(nproc) release'
            }

            stage('Tests') {
                sh 'build/run make -j\$(nproc) check'
            }

           stage('E2E') {
                def exists = fileExists 'release/version'

                if (!exists) {
                    error('The file release/version does not exist')
                }

                def rook_tag = readFile (file: 'release/version', encoding : 'utf-8').trim()

                if (rook_tag == '') {
                    error('Failed to get rook_tag from version file')
                }

                echo 'Rook Tag is ' + rook_tag

                sh "e2e/scripts/smoke_test.sh ${rook_tag} Kubernetes v1.6"

                junit 'e2e/results/*.xml'
            }

            stage('Publish') {
                withCredentials([
                    [$class: 'UsernamePasswordMultiBinding', credentialsId: 'rook-quay-io', usernameVariable: 'DOCKER_USER', passwordVariable: 'DOCKER_PASSWORD'],
                    [$class: 'UsernamePasswordMultiBinding', credentialsId: 'rook-jenkins-aws', usernameVariable: 'AWS_ACCESS_KEY_ID', passwordVariable: 'AWS_SECRET_ACCESS_KEY'],
                    [$class: 'StringBinding', credentialsId: 'quantumbuild-token', variable: 'GITHUB_TOKEN']
                ]) {
                    sh 'docker login -u="${DOCKER_USER}" -p="${DOCKER_PASSWORD}" quay.io'
                    sh 'build/run make -j\$(nproc) publish'
                }
            }

            stage('Cleanup') {
                sh 'build/run make -j\$(nproc) publish.cleanup'
                sh 'docker images'
                deleteDir()
            }
        }
    }
}
catch (Exception e) {
    echo 'Failure encountered'

    node("ec2-stateful") {
        echo 'Cleaning up workspace'
        sh 'build/run make -j\$(nproc) publish.cleanup'
        sh 'docker images'
        deleteDir()
    }

    error "Build failure"
}
