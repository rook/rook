// Rook build for Jenkins Pipelines

pipeline {
    agent { label 'ec2-stateful' }

    options {
        disableConcurrentBuilds()
        timeout(time: 2, unit: 'HOURS')
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

        stage('Smoke Test - k8s 1.6'){
            environment {
                KUBE_VERSION = "v1.6"
            }
            steps {
                sh 'tests/scripts/kubeadm-dind.sh up'
                sh 'build/run make -j\$(nproc) test-integration'
            }
        }

        stage('Integration Tests SetUp') {
            steps {
                sh 'tests/scripts/saveImages.sh'
                stash name: 'repo',excludes: '_output/,vendor/*,.cache/,.work/' ,useDefaultExcludes: true
            }
        }

        stage('Integration Tests - parallel runs') {
            environment {
                KUBE_VERSION = "v1.6"
            }
            steps {
                parallel(
                    "GCE_K8s_v1.7" : {
                        node("gce_vm"){
                            script {
                                try{
                                    withEnv(["KUBE_VERSION=v1.7"]){
                                        deleteDir()
                                        unstash 'repo'
                                        sh "sleep 120"
                                        sh "whoami"
                                        sh "docker load -i rookamd64.tar"
                                        sh "docker load -i toolboxamd64.tar"
                                        sh 'build/run make -j\$(nproc) build.common'
                                        sh 'tests/scripts/kubeadm-dind.sh up'
                                        sh 'build/run make -j\$(nproc) test-integration'
                                    }
                                }
                                finally{
                                    archive '_output/tests/*'
                                    junit allowEmptyResults: true, keepLongStdio: true, testResults: '_output/tests/*.xml'
                                    sh 'make -j\$(nproc) clean'
                                    sh 'make -j\$(nproc) prune PRUNE_HOURS=48 PRUNE_KEEP=48'
                                    sh 'make -j\$(nproc) -C build/release clean'
                                    deleteDir()
                                }
                            }
                        }
                    },
                    "Azure k8s_v1.6" : {
                        node("azure_vm"){
                            script{
                                try{
                                    withEnv(["KUBE_VERSION=v1.6"]){
                                       deleteDir()
                                       unstash 'repo'
                                       sh "docker load -i rookamd64.tar"
                                       sh "docker load -i toolboxamd64.tar"
                                       sh 'build/run make -j\$(nproc) build.common'
                                       sh 'tests/scripts/kubeadm-dind.sh up'
                                       sh 'build/run make -j\$(nproc) test-integration'
                                    }
                                }
                                finally{
                                    archive '_output/tests/*'
                                    junit allowEmptyResults: true, keepLongStdio: true, testResults: '_output/tests/*.xml'
                                    sh 'make -j\$(nproc) clean'
                                    sh 'make -j\$(nproc) prune PRUNE_HOURS=48 PRUNE_KEEP=48'
                                    sh 'make -j\$(nproc) -C build/release clean'
                                    deleteDir()
                                }
                            }
                        }
                    }
                )
            }
        }
        stage('Publish') {
            when {
                anyOf {
                    branch 'master'
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
                sh 'build/run make -j\$(nproc) -C build/release publish AWS_ACCESS_KEY_ID=${AWS_USR} AWS_SECRET_ACCESS_KEY=${AWS_PSW}'
                sh 'build/run make -j\$(nproc) -C build/release promote CHANNEL=master AWS_ACCESS_KEY_ID=${AWS_USR} AWS_SECRET_ACCESS_KEY=${AWS_PSW}'
            }
        }
    }

    post {
        always {
            archive '_output/tests/*'
            junit allowEmptyResults: true, keepLongStdio: true, testResults: '_output/tests/*.xml'
            sh 'make -j\$(nproc) clean'
            sh 'make -j\$(nproc) prune PRUNE_HOURS=48 PRUNE_KEEP=48'
            sh 'make -j\$(nproc) -C build/release clean'
            deleteDir()
        }
    }
}
