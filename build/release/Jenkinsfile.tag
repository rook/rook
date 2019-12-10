// Rook build for Jenkins Pipelines

pipeline {
    agent { label 'ec2-stateful' }

    options {
        disableConcurrentBuilds()
        timestamps()
    }

    parameters {
        string(name: 'version', defaultValue: '', description: 'The version you are releasing, for example, v1.1.0 or v1.1.0-alpha.0 or v1.1.0-beta.0 or v1.1.0-rc.0')
        string(name: 'commit', defaultValue: '', description: 'Optional commit hash for this release, for example, 56b65dba917e50132b0a540ae6ff4c5bbfda2db6. If empty the latest commit hash will be used.')
    }

    stages {
        stage('Tag Release') {
            environment {
                GITHUB = credentials('rook-github')
            }
            steps {
                // github credentials are not setup to push over https in jenkins. add the github token to the url
                sh "git config remote.origin.url https://${GITHUB_USR}:${GITHUB_PSW}@\$(git config --get remote.origin.url | sed -e 's/https:\\/\\///')"
                sh "make -j\$(nproc) -C build/release tag VERSION=${params.version} COMMIT_HASH=${params.commit} TAG_WITH_SUFFIX=true"
            }
        }
    }

    post {
        always {
            deleteDir()
        }
    }
}
