job('build') {

    configure { project ->
        project / 'properties' / 'hudson.plugins.copyartifact.CopyArtifactPermissionProperty' / 'projectNameList' {
            'string' "*rook*"
        }
    }

    parameters {
        stringParam('branch', 'master')
        stringParam('parallel', '')
        stringParam('CCACHE_DIR', '')
        stringParam('WORKSPACE', '')
    }
    
    customWorkspace('${WORKSPACE}')

    wrappers {
        credentialsBinding {
            usernamePassword('GITHUB_USERNAME', 'GITHUB_TOKEN','quantumbuild')

        }
    }

    throttleConcurrentBuilds {
        maxPerNode(1)
        maxTotal(1)
    }

    //// not yet handling build artifacts
    // publishers {
    //     archiveArtifacts {
    //         pattern('results/*')
    //         onlyIfSuccessful()
    //     }
    // }

    steps {

        shell('git submodule sync --recursive')
        shell('git submodule update --init --recursive')
        shell('mkdir -p ${CCACHE_DIR}')
        shell('(CCACHE_DIR=${CCACHE_DIR} build/run make release -j${parallel})')
        
    }

    label 'ec2-stateful'
    concurrentBuild false
}