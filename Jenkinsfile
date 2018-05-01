// Rook build for Jenkins Pipelines

pipeline {
   parameters {
         booleanParam(defaultValue: true, description: 'Execute pipeline?', name: 'shouldBuild')
         booleanParam(defaultValue: false, description: 'Run Only Smoke Test', name: 'smokeOnly')

      }
    agent { label 'ec2-stateful' }

    options {
        disableConcurrentBuilds()
        timeout(time: 2, unit: 'HOURS')
        timestamps()
    }

    stages {
        stage('Pre Build check'){
            when { branch "PR-*" }
            steps {
                script {
                    pr_number = sh (script: "echo ${env.BRANCH_NAME} | grep -o -E '[0-9]+' ",returnStdout: true)
                    def json = sh (script: "curl -s https://api.github.com/repos/rook/rook/pulls/${pr_number}", returnStdout: true).trim()
                    def body = evaluateJson(json,'${json.body}')
                    if (body.contains("[skip ci]")) {
                         echo ("'[skip ci]' spotted in PR body text. Aborting.")
                         env.shouldBuild = "false"
                    }
                    if (body.contains("[skip tests]")) {
                         env.shouldTest = "false"
                    }
                    if (body.contains("[smoke only]")) {
                          env.smokeOnly = "true"
                    }
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
                sh 'build/run make -j\$(nproc) build.all'
            }
        }
        stage('Unit Tests') {
            when {
                expression {
                    return env.shouldBuild != "false"
                }
            }
            steps {
                sh 'build/run make -j\$(nproc) test'
            }
        }

       stage('Integration Tests') {
            when {
                expression {
                    return env.shouldBuild != "false" && env.shouldTest != "false"
                }
            }
            steps{
                sh 'cat _output/version | xargs tests/scripts/makeTestImages.sh  save amd64'
                stash name: 'repo-amd64',includes: 'ceph-amd64.tar,cockroachdb-amd64.tar,build/common.sh,_output/tests/linux_amd64/,_output/charts/,tests/scripts/'
                script{
                    def data = [
                        "aws_1.7.x": "v1.7.11",
                        "aws_1.8.x": "v1.8.5",
                        "gce_1.9.x": "v1.9.6",
                        "aws_1.10.x": "v1.10.1"
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
            when {
                expression {
                    return env.shouldBuild != "false" && env.shouldTest != "false"
                }
            }
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
            notifySlack(currentBuild.result)
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
                        if ("${env.smokeOnly}" == "true") {
                            echo "Running Smoke Tests"
                            sh '''#!/bin/bash
                                  set -o pipefail
                                  export KUBECONFIG=$HOME/admin.conf
                                  kubectl config view
                                  _output/tests/linux_amd64/integration -test.v -test.timeout 600s -test.run SmokeSuite --host_type '''+"${k}"+''' 2>&1 | tee _output/tests/integrationTests.log'''
                        }
                        else {
                        echo "Running full regression"
                        sh '''#!/bin/bash
                              set -o pipefail
                              export KUBECONFIG=$HOME/admin.conf
                              kubectl config view
                              _output/tests/linux_amd64/integration -test.v -test.timeout 2400s --host_type '''+"${k}"+''' 2>&1 | tee _output/tests/integrationTests.log'''
                         }
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

def notifySlack(String buildStatus) {

    // build status of null means successful
    buildStatus =  buildStatus ?: 'SUCCESS'

    // Default values
    def colorCode = '#FF0000'
    def summary = "@channel  ${buildStatus}: $JOB_NAME: \n<$BUILD_URL|Build #$BUILD_NUMBER> - $currentBuild.displayName"

    // Override default values based on build status
    if (buildStatus != 'SUCCESS') {
        // Send notifications to channel on non success builds only
        if (env.BRANCH_NAME == "master" || env.BRANCH_NAME.contains("release")){
            slackSend (color: colorCode, message: summary)
        }
    }
}
