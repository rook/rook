multiJob('ci-rook') {
    description('Rook Continuous Integration builds')

    parameters {
        stringParam('branch', 'master')
        stringParam('parallel', '12', "parallel make jobs")
        stringParam('VERSION', 'dev')
        stringParam('CCACHE_DIR', '/home/ubuntu/ccache')
        stringParam('WORKSPACE', 'workspace/ci-rook')

    }

    wrappers {
        buildName('${GIT_REVISION} on ${ENV,var="branch"}')
        credentialsBinding {
            file('dockerconfig', 'dockerconfig')
            file('gitconfig', 'gitconfig')
            usernamePassword('GITHUB_USERNAME', 'GITHUB_TOKEN','quantumbuild')

        }
    }

    publishers {
        postBuildScripts {
            githubCommitNotifier()
        }
    }

    customWorkspace('${WORKSPACE}')

    throttleConcurrentBuilds {
        maxPerNode(1)
        maxTotal(1)
    }

    scm {
        git {
            remote {
                github('rook/rook', 'https')
                credentials('quantumbuild')
            }
            branch '${branch}'
            extensions {
                wipeOutWorkspace()
            }
            
        }      
    }

    triggers {
        githubPush()

    }

    steps {
        phase('Build') {
            phaseJob('build') {

            }
        }
        phase('Test') {
            phaseJob('unit-test-runner') {

            }

        }

    }
    label 'ec2-stateful'
}