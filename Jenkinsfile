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
                sh 'tests/scripts/makeTestImages.sh save amd64'
                stash name: 'repo-amd64',includes: 'rook-amd64.tar,build/common.sh,_output/tests/linux_amd64/,tests/scripts/'
                script{

                    def data = [
                        "aws_ci": "v1.6",
                        "gce_ci": "v1.7"
                    ]
                    testruns = [:]
                    for (kv in mapToList(data)) {
                        testruns[kv[0]] = RunIntegrationTest(kv[0], kv[1])
                    }
                    parallel testruns

                    for (kv in mapToList(data)) {
                        unstash "${kv[0]}_${kv[1]}_result"
                        sh "build/run go get github.com/jstemmer/go-junit-report && cat _output/tests/${kv[0]}_${kv[1]}_integrationTests.log | go-junit-report > ${kv[0]}_${kv[1]}_integrationTests.xml"
                    }
                }
            }
        }
        stage('Publish') {
            environment {
                DOCKER = credentials('rook-docker-hub')
                QUAY = credentials('rook-quay-io')
                AWS = credentials('rook-jenkins-aws')
            }
            steps {
                sh 'docker login -u="${DOCKER_USR}" -p="${DOCKER_PSW}"'
                sh 'docker login -u="${QUAY_USR}" -p="${QUAY_PSW}" quay.io'
                sh 'build/run make -j\$(nproc) -C build/release build BRANCH_NAME=${BRANCH_NAME}'
                sh 'build/run make -j\$(nproc) -C build/release publish BRANCH_NAME=${BRANCH_NAME} AWS_ACCESS_KEY_ID=${AWS_USR} AWS_SECRET_ACCESS_KEY=${AWS_PSW}'
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
                    sh "tests/scripts/kubeadm-dind.sh up"
                    sh "chmod +x .cache/kubeadm-dind/kubectl"
                    sh "sudo cp .cache/kubeadm-dind/kubectl /usr/local/bin/"
                    try{
                        sh '''#!/bin/bash
                        set -o pipefail
                        _output/tests/linux_amd64/smoke -test.v -test.timeout 1200s 2>&1 | tee _output/tests/integrationTests.log'''
                    }
                    finally{
                        sh "mv _output/tests/integrationTests.log _output/tests/${k}_${v}_integrationTests.log"
                    }
                    stash name: "${k}_${v}_result",includes : "_output/tests/${k}_${v}_integrationTests.log"
                }
            }
            finally{
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
