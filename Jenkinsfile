// Rook build for Jenkins Pipelines

pipeline {
   parameters {
         booleanParam(defaultValue: false, description: 'Skip Integration Tests?', name: 'skipIntegrationTests')
      }
    agent { label 'ec2-stateful' }

    options {
        disableConcurrentBuilds()
        buildDiscarder(logRotator(numToKeepStr: '200'))
        timeout(time: 2, unit: 'HOURS')
        timestamps()
    }

    stages {
        stage('Pre Build check'){
            when { branch "PR-*" }
            steps {
                script {
                    // When running in a PR we assuming it's not an official build
                    env.isOfficialBuild = "false"
                    env.shouldBuild = "true"
                    env.testProvider = "ceph"
                    env.testArgs = "min-test-matrix"
                }
            }
        }
        stage('Build') {
            when {
                expression {
                    return env.shouldBuild != "false"
                }
            }
            steps {
                // run the build
                script {
                    if (env.isOfficialBuild == "false") {
                        sh (script: "build/run make -j\$(nproc) build", returnStdout: true)
                    } else {
                        sh (script: "build/run make -j\$(nproc) build.all", returnStdout: true)
                    }
                }
                sh 'git status'
            }
        }

        stage('Integration Tests') {
            when {
                expression {
                    return env.shouldBuild != "false" && !params.skipIntegrationTests
                }
            }
            steps {
                sh 'cat _output/version | xargs tests/scripts/makeTestImages.sh  save amd64'
                stash name: 'repo-amd64',includes: 'ceph-amd64.tar,cassandra-amd64.tar,nfs-amd64.tar,build/common.sh,_output/tests/linux_amd64/,_output/charts/,tests/scripts/,cluster/'
                script {
                    def data = [
                        "aws_1.11.x": "v1.11.10"
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
                            unstash "${kv[1]}_result"
                            sh "cat _output/tests/${kv[1]}_integrationTests.log | _output/go-junit-report > _output/tests/${kv[1]}_integrationTests.xml"
                        }
                    }
                }
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
                    echo "running tests on k8s version ${v}"
                    unstash 'repo-amd64'
                    sh "tests/scripts/kubeadm.sh clean || 1"
                    sh 'tests/scripts/makeTestImages.sh load amd64'
                    sh "tests/scripts/kubeadm.sh up"
                    sh '''#!/bin/bash
                          export KUBECONFIG=$HOME/admin.conf
                          tests/scripts/helm.sh up'''
                    try{
                        echo "Running full regression"
                        sh '''#!/bin/bash
                              set -o pipefail
                              export KUBECONFIG=$HOME/admin.conf \
                                  SKIP_TEST_CLEANUP=false \
                                  SKIP_CLEANUP_POLICY=false \
                                  SKIP_CASSANDRA_TESTS=true \
                                  TEST_ENV_NAME='''+"${k}"+''' \
                                  TEST_BASE_DIR="WORKING_DIR" \
                                  TEST_LOG_COLLECTION_LEVEL='''+"${env.getLogs}"+''' \
                                  STORAGE_PROVIDER_TESTS='''+"${env.testProvider}"+''' \
                                  TEST_ARGUMENTS='''+"${env.testArgs}"+''' \
                                  TEST_IS_OFFICIAL_BUILD='''+"${env.isOfficialBuild}"+''' \
                                  TEST_SCRATCH_DEVICE=/dev/nvme0n1
                              kubectl config view
                              _output/tests/linux_amd64/integration -test.v -test.timeout 7200s 2>&1 | tee _output/tests/integrationTests.log'''
                    }
                    finally{
                        sh "journalctl -u kubelet > _output/tests/kubelet_${v}.log"
                        sh "journalctl > _output/tests/system_journalctl_${v}.log"
                        sh "dmesg > _output/tests/system_dmesg_${v}.log"
                        sh '''#!/bin/bash
                              export KUBECONFIG=$HOME/admin.conf
                              tests/scripts/helm.sh clean || true'''
                        sh "mv _output/tests/integrationTests.log _output/tests/${v}_integrationTests.log"
                        stash name: "${v}_result",includes : "_output/tests/${v}_integrationTests.log"
                    }
                }
            }
            finally{
                archive '_output/tests/*.log'
                sh 'sudo rm -rf ${PWD}/rook-test'
                sh 'sudo ls -l ${PWD}'
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

@NonCPS
def evaluateJson(String json, String gpath){
    //parse json
    def ojson = new groovy.json.JsonSlurper().parseText(json)
    //evaluate gpath as a gstring template where $json is a parsed json parameter
    return new groovy.text.GStringTemplateEngine().createTemplate(gpath).make(json:ojson).toString()
}
