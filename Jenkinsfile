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
                    def json = sh (script: "curl -s https://api.github.com/repos/rook/rook/pulls/${env.CHANGE_ID}", returnStdout: true).trim()
                    def draft = evaluateJson(json,'${json.draft}')
                    if (draft.contains("true")) {
                        echo ("This is a draft PR. Aborting")
                        env.shouldBuild = "false"
                    }
                    def body = evaluateJson(json,'${json.body}')
                    if (body.contains("[skip ci]")) {
                        echo ("'[skip ci]' spotted in PR body text. Aborting.")
                        env.shouldBuild = "false"
                    }
                    if (body.contains("[skip tests]")) {
                        env.shouldTest = "false"
                    }
                    if (body.contains("[all logs]")) {
                        env.getLogs = "all"
                    }

                    // When running in a PR we assuming it's not an official build
                    env.isOfficialBuild = "false"

                    if (!body.contains("[test full]")) {
                        // By default run the min test matrix (all tests run, but will be distributed on different versions of K8s).
                        // This only affects the PR builds. The master and release builds will always run the full matrix.
                        env.testArgs = "min-test-matrix"
                    }

                    // Get changed files
                    def json_changed_files = sh (script: "curl -s https://api.github.com/repos/rook/rook/pulls/${env.CHANGE_ID}/files", returnStdout: true).trim()
                    def list = new groovy.json.JsonSlurper().parseText(json_changed_files)
                    def changed_files = list.filename.join(",")
                     echo ("changed files are: ${changed_files}")

                    // Get PR title
                    def title = evaluateJson(json,'${json.title}')

                    // extract which specific storage provider to test
                    if (body.contains("[test cassandra]") || title.contains("cassandra:")) {
                        env.testProvider = "cassandra"
                    } else if (body.contains("[test ceph]")) {
                        // Run the Ceph tests in Jenkins if specifically requested
                        env.testProvider = "ceph"
                    } else if (title.contains("ceph:")) {
                        // Don't run Jenkins for ceph PRs by default since they are covered by github actions
                        env.shouldBuild = "false"
                    } else if (body.contains("[test nfs]") || title.contains("nfs:")) {
                        env.testProvider = "nfs"
                    } else if (body.contains("[test]")) {
                        env.shouldBuild = "true"
                    } else if (!changed_files.contains(".go")) {
                        echo ("No golang changes detected! Looking for .md, .yaml and .txt now.")
                        if (changed_files.contains(".md")) {
                            echo ("Documentation changes detected! Aborting.")
                            env.shouldBuild = "false"
                        } else if (changed_files.contains(".yaml")) {
                            echo ("YAML changes detected! Aborting.")
                            env.shouldBuild = "false"
                        } else if (changed_files.contains(".txt")) {
                            echo ("Text changes detected! Aborting.")
                            env.shouldBuild = "false"
                        }
                    } else if (!changed_files.contains(".go")) {
                        echo ("No code changes detected! Just building.")
                        env.shouldTest = "false"
                    }

                    echo ("integration test provider: ${env.testProvider}")
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
        stage('Unit Tests for Release Builds') {
            when {
                expression {
                    return env.shouldBuild != "false" && env.isOfficialBuild != "false"
                }
            }
            steps {
                sh 'build/run make test'
            }
        }

        stage('Publish if Master') {
            when {
                expression {
                    return env.BRANCH_NAME == "master"
                }
            }
            environment {
                DOCKER = credentials('rook-docker-hub')
                AWS = credentials('rook-jenkins-aws')
                GIT = credentials('rook-github')
            }
            steps {
                sh 'docker login -u="${DOCKER_USR}" -p="${DOCKER_PSW}"'
                // quick check that go modules are tidied
                sh 'build/run make -j\$(nproc) mod.check'
                sh 'build/run make -j\$(nproc) -C build/release build BRANCH_NAME=${BRANCH_NAME} GIT_API_TOKEN=${GIT_PSW}'
                sh 'git status'
                sh 'git diff'
                sh 'build/run make -j\$(nproc) -C build/release publish BRANCH_NAME=${BRANCH_NAME} AWS_ACCESS_KEY_ID=${AWS_USR} AWS_SECRET_ACCESS_KEY=${AWS_PSW} GIT_API_TOKEN=${GIT_PSW}'
                // automatically promote the master builds
                sh 'build/run make -j\$(nproc) -C build/release promote BRANCH_NAME=master CHANNEL=master AWS_ACCESS_KEY_ID=${AWS_USR} AWS_SECRET_ACCESS_KEY=${AWS_PSW}'
            }
        }

        stage('Integration Tests') {
            when {
                expression {
                    return env.shouldBuild != "false" && env.shouldTest != "false" && !params.skipIntegrationTests
                }
            }
            steps {
                sh 'cat _output/version | xargs tests/scripts/makeTestImages.sh  save amd64'
                stash name: 'repo-amd64',includes: 'ceph-amd64.tar,cassandra-amd64.tar,nfs-amd64.tar,build/common.sh,_output/tests/linux_amd64/,_output/charts/,tests/scripts/,cluster/'
                script {
                    def data = [
                        "aws_1.11.x": "v1.11.10",
                        "aws_1.13.x": "v1.13.12",
                        "aws_1.15.x": "v1.15.12",
                        "aws_1.17.x": "v1.17.13",
                        "aws_1.19.x": "v1.19.9",
                        "aws_1.20.x": "v1.20.5"
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

        stage('Publish if Release') {
            when {
                expression {
                    return env.BRANCH_NAME.contains("release-")
                }
            }
            environment {
                DOCKER = credentials('rook-docker-hub')
                AWS = credentials('rook-jenkins-aws')
                GIT = credentials('rook-github')
            }
            steps {
                sh 'docker login -u="${DOCKER_USR}" -p="${DOCKER_PSW}"'
                // quick check that go modules are tidied
                sh 'build/run make -j\$(nproc) mod.check'
                sh 'build/run make -j\$(nproc) -C build/release build BRANCH_NAME=${BRANCH_NAME} TAG_WITH_SUFFIX=true GIT_API_TOKEN=${GIT_PSW}'
                sh 'git status'
                sh 'git diff'
                sh 'build/run make -j\$(nproc) -C build/release publish BRANCH_NAME=${BRANCH_NAME} TAG_WITH_SUFFIX=true AWS_ACCESS_KEY_ID=${AWS_USR} AWS_SECRET_ACCESS_KEY=${AWS_PSW} GIT_API_TOKEN=${GIT_PSW}'
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

def notifySlack(String buildStatus) {

    // build status of null means successful
    buildStatus =  buildStatus ?: 'SUCCESS'

    // Default values
    def colorCode = '#FF0000'
    def summary = "@channel  ${buildStatus}: $JOB_NAME: \n<$BUILD_URL|Build #$BUILD_NUMBER> - $currentBuild.displayName"

    // Override default values based on build status
    if (buildStatus != 'SUCCESS') {
        // Send notifications to channel on non success builds only
        if (env.BRANCH_NAME == "master" || env.BRANCH_NAME.contains("release-")){
            slackSend (color: colorCode, message: summary)
        }
    }
}
