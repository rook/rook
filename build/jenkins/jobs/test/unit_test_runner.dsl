job('unit-test-runner') {

    configure { project ->
        project / 'properties' / 'hudson.plugins.copyartifact.CopyArtifactPermissionProperty' / 'projectNameList' {
            'string' "*rook*"
        }
    }

    parameters {
            stringParam('WORKSPACE', '')

    }

    customWorkspace('${WORKSPACE}')

    throttleConcurrentBuilds {
        maxPerNode(1)
        maxTotal(1)
    }

    //// not yet handling test artifacts
    // publishers {
    //     archiveArtifacts {
    //         pattern('results/*')
    //         onlyIfSuccessful()
    //     } 
    // }

    steps {

        // execute test 
        shell('(build/run make check -j${parallel})')

    }

    concurrentBuild false
    label 'ec2-stateful'
}
