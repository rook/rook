// Rook build for Jenkins Pipelines

pipeline {
    agent { label 'ec2-stateful' }

    options {
        timeout(time: 1, unit: 'HOURS')
        timestamps()
    }

    environment {
        PRUNE_HOURS = 48
        PRUNE_KEEP = 48
    }

    stages {
        stage('Build') {
            steps {
                sh 'build/run make -j\$(nproc) build.all'
            }
        }
        stage('Unit Tests') {
            steps {
                sh 'build/run make -j\$(nproc) check'
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
                sh 'build/run make -C build/release -j\$(nproc)'
                sh 'build/run make -C build/release -j\$(nproc) publish'
            }
        }

    }

    post {
        always {
            sh 'make clean'
            sh 'make prune'
            deleteDir()
        }
    }
}
