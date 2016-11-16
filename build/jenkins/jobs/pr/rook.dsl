multiJob('pr-rook') {
    description('Rook Pull Request builds')

    parameters {
        stringParam('branch', '')
        stringParam('WORKSPACE', 'workspace/pr-rook')
        stringParam('VERSION', 'dev')
        stringParam('CCACHE_DIR', '/home/ubuntu/ccache')
        stringParam('parallel', '12', "parallel make jobs")

    }

    wrappers {
        buildName('${sha1}')
        credentialsBinding {
            file('dockerconfig', 'dockerconfig')
            file('gitconfig', 'gitconfig')
            usernamePassword('GITHUB_USERNAME', 'GITHUB_TOKEN','quantumbuild')

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
                refspec('+refs/pull/*:refs/remotes/origin/pr/*')
            }
            branch '${sha1}'
            extensions {
                wipeOutWorkspace()
            }
            
        }      
    }

    triggers {
        githubPullRequest {
            admin('bassamtabbara')
            admins(['bassamtabbara', 'doubledensity'])
            userWhitelist(['travisn', 'jbw976'])
            orgWhitelist('quantum')
            orgWhitelist(['rook'])
            cron('H/5 * * * *')
            triggerPhrase('OK to test')
            onlyTriggerPhrase(false)
            useGitHubHooks()
            permitAll(true)
            autoCloseFailedPullRequests(false)
            allowMembersOfWhitelistedOrgsAsAdmin(true)
            extensions {
                commitStatus {
                    context('ci-rook')
                    triggeredStatus('in progress...')
                    startedStatus('in progress...')
                    completedStatus('SUCCESS', 'All is well')
                    completedStatus('FAILURE', 'Something went wrong. Investigate!')
                    completedStatus('PENDING', 'still in progress...')
                    completedStatus('ERROR', 'Something went really wrong. Investigate!')
                }
            }
        }

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