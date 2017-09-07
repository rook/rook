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

       stage('Integration Tests') {
            steps{
                sh 'cat _output/version | xargs tests/scripts/makeTestImages.sh  save amd64'
                stash name: 'repo-amd64',includes: 'rook-amd64.tar,build/common.sh,_output/tests/linux_amd64/,_output/charts/,tests/scripts/'
                script{

                    def data = [
                        "aws_ci": "v1.6.7",
                        "gce_ci": "v1.7.4"
                    ]
                    testruns = [:]
                    for (kv in mapToList(data)) {
                        testruns[kv[0]] = RunIntegrationTest(kv[0], kv[1])
                    }
                    try{
                        parallel testruns
                    }

                    finally{
                        sh "build/run go get -u -f  github.com/jstemmer/go-junit-report"
                        for (kv in mapToList(data)) {
                            unstash "${kv[0]}_${kv[1]}_result"
                            sh "cat _output/tests/${kv[0]}_${kv[1]}_integrationTests.log | _output/go-junit-report > _output/tests/${kv[0]}_${kv[1]}_integrationTests.xml"
                        }
                    }
                }
            }
        }
        stage('Publish') {
            environment {
                DOCKER = credentials('rook-docker-hub')
                QUAY = credentials('rook-quay-io')
                AWS = credentials('rook-jenkins-aws')
                GIT = credentials('rook-github')
            }
            steps {
                sh 'docker login -u="${DOCKER_USR}" -p="${DOCKER_PSW}"'
                sh 'docker login -u="${QUAY_USR}" -p="${QUAY_PSW}" quay.io'
                sh 'build/run make -j\$(nproc) -C build/release build BRANCH_NAME=${BRANCH_NAME} GIT_API_TOKEN=${GIT_PSW}'
                sh 'build/run make -j\$(nproc) -C build/release publish BRANCH_NAME=${BRANCH_NAME} AWS_ACCESS_KEY_ID=${AWS_USR} AWS_SECRET_ACCESS_KEY=${AWS_PSW} GIT_API_TOKEN=${GIT_PSW}'
                sh '[ "${BRANCH_NAME}" != "master" ] || build/run make -j\$(nproc) -C build/release promote BRANCH_NAME=master CHANNEL=master AWS_ACCESS_KEY_ID=${AWS_USR} AWS_SECRET_ACCESS_KEY=${AWS_PSW}'
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
def RunIntegrationTest(k, v) {
  return {
      node("${k}") {
        script{
            try{
                withEnv(["KUBE_VERSION=${v}"]){
                    unstash 'repo-amd64'
                    echo "running tests on k8s version ${v}"
                    sh 'tests/scripts/makeTestImages.sh load amd64'
                    sh "tests/scripts/kubeadm.sh up"
                    sh '''#!/bin/bash
                          export KUBECONFIG=$HOME/admin.conf
                          tests/scripts/helm.sh up'''
                    try{
                        sh '''#!/bin/bash
                        set -o pipefail
                        export KUBECONFIG=$HOME/admin.conf
                        kubectl config view
                        _output/tests/linux_amd64/smoke -test.v -test.timeout 1200s 2>&1 | tee _output/tests/integrationTests.log'''
                    }
                    finally{
                        sh '''#!/bin/bash
                              export KUBECONFIG=$HOME/admin.conf
                              tests/scripts/helm.sh clean || true'''
                        sh "mv _output/tests/integrationTests.log _output/tests/${k}_${v}_integrationTests.log"
                        stash name: "${k}_${v}_result",includes : "_output/tests/${k}_${v}_integrationTests.log"
                    }
                }
            }
            finally{
                archive '_output/tests/*.log'
                deleteDir()
            }
        }
      }
  }
}

// Required due to JENKINS-27421
@NonCPS
List<List<?>> mapToList(Map map) {
  return map.collect { it ->[it.key, it.value]}
}