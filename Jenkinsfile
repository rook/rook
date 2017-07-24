// Rook build for Jenkins Pipelines

pipeline {
    agent { label 'ec2-stateful' }
    stages {
        stage('Publish Documentation') {
            when {
                anyOf {
                    branch 'master'
                    branch 'PR-*' // TODO remove this
                }
            }
            steps {
                withCredentials([[$class: 'UsernamePasswordMultiBinding', credentialsId: 'quantumbuild', usernameVariable: 'GIT_API_USER', passwordVariable: 'GIT_API_TOKEN']]) {
                    sh "make -C build/release build FLAVORS=docs BRANCH_NAME=release-0.4"
                    sh "make -C build/release publish FLAVORS=docs BRANCH_NAME=release-0.4"
                }
            }
        }
    }
}
