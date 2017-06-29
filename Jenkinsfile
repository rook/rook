// Rook build for Jenkins Pipelines

pipeline {
    agent { label 'ec2-stateful' }

    options {
        disableConcurrentBuilds()
        timeout(time: 1, unit: 'HOURS')
        timestamps()
    }

    stages {
        stage('Build') {
            steps {
                sh 'build/run make -j\$(nproc) build.all'
            }
        }
        stage('Unit Tests') {
            steps {
                sh 'build/run make -j\$(nproc) test'
            }
        }
        stage('Integration Tests') {
            environment {
                KUBE_VERSION = "v1.6"
            }
            steps {
                sh 'tests/scripts/kubeadm-dind.sh up'
                sh 'build/run make -j\$(nproc) test-integration'
            }
        }
        stage('Publish') {
            when {
                anyOf {
                    branch 'master'
                    branch 'release-*'
                }
            }
            environment {
                DOCKER = credentials('rook-docker-hub')
                QUAY = credentials('rook-quay-io')
                AWS = credentials('rook-jenkins-aws')
            }
            steps {
                sh 'docker login -u="${DOCKER_USR}" -p="${DOCKER_PSW}"'
                sh 'docker login -u="${QUAY_USR}" -p="${QUAY_PSW}" quay.io'
                sh 'build/run make -j\$(nproc) -C build/release build'
                sh 'build/run make -j\$(nproc) -C build/release publish'
            }
        }
    }

    post {
        always {
            archive '_output/tests/*'
            junit allowEmptyResults: true, keepLongStdio: true, testResults: '_output/tests/*.xml'
            sh 'make clean'
            sh 'make prune PRUNE_HOURS=48 PRUNE_KEEP=48'
            deleteDir()
        }
    }
}
