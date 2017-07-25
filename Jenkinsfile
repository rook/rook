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
                sh 'tests/scripts/makeTestImages.sh  save amd64'
                stash name: 'repo',excludes: '_output/,vendor/,.cache/,.work/'
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
                }
            }
        }
        stage('Publish') {
            environment {
                DOCKER = credentials('rook-docker-hub')
                QUAY = credentials('rook-quay-io')
                AWS = credentials('rook-jenkins-aws')
                GIT = credentials('quantumbuild')
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
                    unstash 'repo'
                    echo "running tests on k8s version ${v}"
                    sh 'tests/scripts/makeTestImages.sh load amd64'
                    sh 'build/run make -j\$(nproc) build.common'
                    sh "tests/scripts/kubeadm-dind.sh up"
                    sh 'build/run make -j\$(nproc) test-integration SUITE=Smoke'
                }
            }
            finally{
                archive '_output/tests/*'
                junit allowEmptyResults: true, keepLongStdio: true, testResults: '_output/tests/*.xml'
                sh 'make -j\$(nproc) clean'
                sh 'make -j\$(nproc) prune PRUNE_HOURS=48 PRUNE_KEEP=48'
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
